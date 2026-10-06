package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/coordinationrequest"
	"github.com/jackc/pgx/v5"
)

type amanSessionQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadAMANSessionState(ctx context.Context, db amanSessionQuerier, airport string) (aman.AirportState, error) {
	var raw []byte
	err := db.QueryRow(ctx, `SELECT payload FROM aman_session_states WHERE session_id=$1 AND airport=$2`, aman.SessionID(ctx), airport).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN session state was not found"}
	}
	if err != nil {
		return aman.AirportState{}, err
	}
	var state aman.AirportState
	if err := json.Unmarshal(raw, &state); err != nil {
		return aman.AirportState{}, corruptAMANData("decode session state", err)
	}
	if state.SessionID != aman.SessionID(ctx) || state.Airport != airport {
		return aman.AirportState{}, corruptAMANData("session state identity", errors.New("session/airport mismatch"))
	}
	if err := state.Validate(); err != nil {
		return aman.AirportState{}, corruptAMANData("validate session state", err)
	}
	return state, nil
}

func loadAMANSessionOutcome(ctx context.Context, db amanSessionQuerier, id string) (aman.CommandOutcome, error) {
	var raw []byte
	err := db.QueryRow(ctx, `SELECT payload FROM aman_session_command_outcomes WHERE session_id=$1 AND command_id=$2`, aman.SessionID(ctx), id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return aman.CommandOutcome{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN session command outcome was not found"}
	}
	var outcome aman.CommandOutcome
	if err == nil {
		err = json.Unmarshal(raw, &outcome)
	}
	return outcome, err
}

func (r *amanRepository) commitSession(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	id := aman.SessionID(ctx)
	if commit.State.SessionID != id {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "AMAN commit belongs to another session"}
	}
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return aman.CommitResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize creation as well as updates, including concurrent first reports.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("aman-session/%d/%s", id, commit.State.Airport)); err != nil {
		return aman.CommitResult{}, err
	}
	var sessionAirport string
	if err := tx.QueryRow(ctx, `SELECT airport FROM sessions WHERE id=$1 FOR KEY SHARE`, id).Scan(&sessionAirport); err != nil {
		return aman.CommitResult{}, err
	}
	if sessionAirport != commit.State.Airport {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "AMAN airport does not belong to session"}
	}
	if commit.CommandOutcome != nil {
		outcome, err := loadAMANSessionOutcome(ctx, tx, commit.CommandOutcome.CommandID)
		if err == nil {
			state, err := loadAMANSessionState(ctx, tx, outcome.Airport)
			return aman.CommitResult{State: state, CommandOutcome: &outcome, DuplicateCommand: true}, err
		}
		var domain *aman.DomainError
		if !errors.As(err, &domain) || domain.Class != aman.ErrorNotFound {
			return aman.CommitResult{}, err
		}
	}
	previous, err := loadAMANSessionState(ctx, tx, commit.State.Airport)
	var domain *aman.DomainError
	missing := errors.As(err, &domain) && domain.Class == aman.ErrorNotFound
	if err != nil && !missing {
		return aman.CommitResult{}, err
	}
	if previous.Revision != commit.ExpectedRevision || missing && commit.ExpectedRevision != 0 {
		return aman.CommitResult{}, revisionConflict()
	}
	changed := commit.State.Revision == commit.ExpectedRevision+1
	if !changed && (missing || !airportStatesEqual(previous, commit.State)) {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "unchanged session state differs from persisted state"}
	}
	if changed {
		stored := cloneAirportState(commit.State)
		stored.Flights = []aman.AMANFlight{}
		for _, flight := range commit.State.Flights {
			if flight.State != aman.StateRemoved {
				stored.Flights = append(stored.Flights, flight)
			}
		}
		raw, err := json.Marshal(stored)
		if err != nil {
			return aman.CommitResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO aman_session_states(session_id,airport,revision,payload) VALUES($1,$2,$3,$4)
ON CONFLICT(session_id,airport) DO UPDATE SET revision=EXCLUDED.revision,payload=EXCLUDED.payload`, id, stored.Airport, stored.Revision, raw); err != nil {
			return aman.CommitResult{}, err
		}
		for _, fact := range coordinationExpiryFacts(previous, commit.State) {
			if _, err := coordinationrequest.ExpirePendingTx(ctx, tx, fact); err != nil {
				return aman.CommitResult{}, err
			}
		}
	}
	if commit.CommandOutcome != nil {
		raw, err := json.Marshal(commit.CommandOutcome)
		if err != nil {
			return aman.CommitResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO aman_session_command_outcomes(session_id,airport,command_id,payload) VALUES($1,$2,$3,$4)`, id, commit.State.Airport, commit.CommandOutcome.CommandID, raw); err != nil {
			return aman.CommitResult{}, err
		}
	}
	for _, record := range commit.AuditRecords {
		raw, err := json.Marshal(record)
		if err != nil {
			return aman.CommitResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO aman_session_audit_records(session_id,airport,revision,payload) VALUES($1,$2,$3,$4)`, id, record.Airport, record.Revision, raw); err != nil {
			return aman.CommitResult{}, err
		}
	}
	for _, evidence := range commit.ValidationEvidence {
		raw, err := json.Marshal(evidence)
		if err != nil {
			return aman.CommitResult{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO aman_session_validation_evidence(session_id,airport,evidence_id,payload) VALUES($1,$2,$3,$4)`, id, evidence.Airport, evidence.ID, raw); err != nil {
			return aman.CommitResult{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return aman.CommitResult{}, err
	}
	result := aman.CommitResult{State: cloneAirportState(commit.State)}
	if commit.CommandOutcome != nil {
		outcome := cloneCommandOutcome(*commit.CommandOutcome)
		result.CommandOutcome = &outcome
	}
	return result, nil
}

func sessionAMANRecords[T any](ctx context.Context, r *amanRepository, query, airport string) ([]T, error) {
	rows, err := r.pool.Query(ctx, query, aman.SessionID(ctx), airport)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (T, error) {
		var result T
		var raw []byte
		err := row.Scan(&raw)
		if err == nil {
			err = json.Unmarshal(raw, &result)
		}
		return result, err
	})
}
