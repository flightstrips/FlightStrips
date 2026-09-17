package postgres

import (
	"context"

	"FlightStrips/internal/database"
	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/codes"
)

type positionIdentityRead struct {
	repo     *amanRepository
	callsign string
}

// Only the established-identity read participates. Creation, CID changes and
// retirement keep their existing transaction/locking semantics. A new report
// always reads PostgreSQL again; there is no cross-message identity cache.
func (r *amanRepository) activeObservationIdentity(ctx context.Context, callsign string) (database.AmanVatsimObservationIdentity, error) {
	if value, err, joined := joinPositionBatch(ctx, "aman_identity", positionIdentityRead{r, callsign}, batchPositionIdentities); joined {
		if err != nil {
			return database.AmanVatsimObservationIdentity{}, err
		}
		return value.(database.AmanVatsimObservationIdentity), nil
	}
	return r.queries.GetActiveAMANVATSIMObservationIdentity(ctx, callsign)
}

func batchPositionIdentities(calls []*batchCall) {
	groups := make(map[*amanRepository][]*batchCall)
	for _, call := range calls {
		if err := call.ctx.Err(); err != nil {
			call.result <- batchResult{err: err}
			continue
		}
		r := call.input.(positionIdentityRead).repo
		groups[r] = append(groups[r], call)
	}
	for repo, group := range groups {
		if len(group) == 1 {
			call := group[0]
			row, err := repo.queries.GetActiveAMANVATSIMObservationIdentity(call.ctx, call.input.(positionIdentityRead).callsign)
			call.result <- batchResult{value: row, err: err}
			continue
		}
		ctx, span := batchQueryContext(group, "aman_identity")
		callsigns := make([]string, len(group))
		for i, call := range group {
			callsigns[i] = call.input.(positionIdentityRead).callsign
		}
		rows, err := repo.pool.Query(ctx, `-- position batch AMAN identity
SELECT flight_id, vatsim_cid, current_callsign, retired_at, created_at, updated_at
FROM aman_vatsim_observation_identities
WHERE current_callsign = ANY($1::text[]) AND retired_at IS NULL`, callsigns)
		found := make(map[string]database.AmanVatsimObservationIdentity)
		if err == nil {
			var records []database.AmanVatsimObservationIdentity
			records, err = pgx.CollectRows(rows, pgx.RowToStructByName[database.AmanVatsimObservationIdentity])
			for _, row := range records {
				found[row.CurrentCallsign] = row
			}
		}
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
		for _, call := range group {
			row, ok := found[call.input.(positionIdentityRead).callsign]
			result := batchResult{value: row, err: err}
			if err == nil && !ok {
				result.err = pgx.ErrNoRows
			}
			call.result <- result
		}
	}
}
