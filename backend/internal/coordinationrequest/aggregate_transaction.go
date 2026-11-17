package coordinationrequest

import (
	"context"
	"errors"
	"sync"

	"FlightStrips/internal/aman"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type aggregateClearanceKey struct{}
type aggregateClearance struct {
	mu              sync.Mutex
	session         int32
	fact            ClearanceFact
	pool            *pgxpool.Pool
	result          CommitResult
	done, delivered bool
}

// WithAggregateClearance lets the synchronous prediction commit correlate the
// already durable fact in its own transaction. If prediction fails or makes no
// commit, ObserveClearance continues through its normal transaction instead.
func WithAggregateClearance(ctx context.Context, fact ClearanceFact) context.Context {
	return context.WithValue(ctx, aggregateClearanceKey{}, &aggregateClearance{session: aman.SessionID(ctx), fact: fact})
}
func aggregateClearanceFrom(ctx context.Context) *aggregateClearance {
	scope, _ := ctx.Value(aggregateClearanceKey{}).(*aggregateClearance)
	if scope != nil && scope.session == aman.SessionID(ctx) {
		return scope
	}
	return nil
}
func HasAggregateClearance(ctx context.Context) bool { return aggregateClearanceFrom(ctx) != nil }

func CompleteAggregateClearance(ctx context.Context, pool *pgxpool.Pool, result CommitResult) {
	if scope := aggregateClearanceFrom(ctx); scope != nil {
		scope.mu.Lock()
		defer scope.mu.Unlock()
		if scope.done {
			return
		}
		scope.pool, scope.result, scope.done = pool, result, true
	}
}
func committedAggregateClearance(ctx context.Context, pool *pgxpool.Pool, fact ClearanceFact) (CommitResult, bool) {
	scope := aggregateClearanceFrom(ctx)
	if scope == nil {
		return CommitResult{}, false
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if !scope.done || scope.pool != pool || scope.fact != fact {
		return CommitResult{}, false
	}
	result := scope.result
	if scope.delivered {
		result.Duplicate = true
	}
	scope.delivered = true
	return result, true
}

// PrepareAggregateTransaction uses the same coordination lock and row locks as
// standalone decisions, submissions and correlation. Callers must atomically
// persist the returned replacements with the aggregate, then complete the
// clearance receipt only after COMMIT. No locks span prediction computation.
func PrepareAggregateTransaction(ctx context.Context, tx pgx.Tx, airport string, expiries []ExpiryFact) ([]Request, CommitResult, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, aman.SessionAirportKey(ctx, airport)); err != nil {
		return nil, CommitResult{}, err
	}
	current, err := lockedRequests(ctx, tx, airport)
	if err != nil {
		return nil, CommitResult{}, err
	}
	var fact *ClearanceFact
	if scope := aggregateClearanceFrom(ctx); scope != nil {
		value := scope.fact
		fact = &value
	}
	return prepareAggregateFacts(current, expiries, fact)
}

func prepareAggregateFacts(current []Request, expiries []ExpiryFact, fact *ClearanceFact) ([]Request, CommitResult, error) {
	current = append([]Request(nil), current...)
	changed := make(map[RequestID]bool)
	for _, expiry := range expiries {
		if expiry.Airport == "" || expiry.Callsign == "" || !present(expiry.FactID) || expiry.Revision == 0 || !expiry.Reason.valid() || !utc(expiry.OccurredAt) {
			return nil, CommitResult{}, errors.New("authoritative coordination expiry fact is invalid")
		}
		duplicate := false
		for _, request := range current {
			if request.Expiry != nil && request.Expiry.FactID == expiry.FactID && request.Expiry.FactRevision == expiry.Revision {
				if request.Expiry.Reason != expiry.Reason || !request.Expiry.ExpiredAt.Equal(expiry.OccurredAt) {
					return nil, CommitResult{}, ErrCommandConflict
				}
				duplicate = true
			}
		}
		if duplicate {
			continue
		}
		for i, request := range current {
			if request.Airport != expiry.Airport || request.Callsign != expiry.Callsign || request.State != StatePending {
				continue
			}
			updated, err := request.Expire(Expiry{FactID: expiry.FactID, FactRevision: expiry.Revision, Reason: expiry.Reason, ExpiredAt: expiry.OccurredAt})
			if err != nil {
				return nil, CommitResult{}, err
			}
			current[i] = updated
			changed[updated.ID] = true
		}
	}
	result := CommitResult{}
	if fact != nil {
		if fact.Airport == "" || fact.Callsign == "" || !present(fact.FactID) || !present(fact.Issuer) || !present(fact.Value) || (fact.Kind != KindRouteDirect && fact.Kind != KindSpeed) || !utc(fact.ObservedAt) {
			return nil, result, errors.New("authoritative clearance fact is invalid")
		}
		for i := len(current) - 1; i >= 0; i-- {
			if current[i].Clearance != nil && current[i].Clearance.FactID == fact.FactID {
				result.Request = current[i]
				result.Duplicate = true
				break
			}
		}
		if !result.Duplicate {
			for i := len(current) - 1; i >= 0; i-- {
				request := current[i]
				if request.Airport != fact.Airport || request.Callsign != fact.Callsign || request.Kind != fact.Kind || request.State != StateAccepted || request.Clearance != nil || request.ResolvedAt.After(fact.ObservedAt) || !matchesClearance(request, fact.Value) {
					continue
				}
				updated, err := request.Correlate(ClearanceAudit{FactID: fact.FactID, Kind: fact.Kind, Value: fact.Value, Issuer: fact.Issuer, ObservedAt: fact.ObservedAt})
				if err != nil {
					return nil, result, err
				}
				current[i] = updated
				changed[updated.ID] = true
				result.Request = updated
				break
			}
		}
	}
	result.Revision = coordinationRevision(current)
	updates := make([]Request, 0, len(changed))
	for _, request := range current {
		if changed[request.ID] {
			updates = append(updates, request)
		}
	}
	return updates, result, nil
}
