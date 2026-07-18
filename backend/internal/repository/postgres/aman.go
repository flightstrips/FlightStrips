package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/database"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// amanRepository maps private PostgreSQL records to the canonical AMAN domain
// values. It is deliberately not a generic application store.
type amanRepository struct {
	pool    *pgxpool.Pool
	queries *database.Queries
}

// NewAMANRepository creates the transactional persistence component used by
// AMAN command and restart consumers.
func NewAMANRepository(pool *pgxpool.Pool) *amanRepository {
	return &amanRepository{pool: pool, queries: database.New(pool)}
}

// Name makes this concrete component usable at AMAN's explicit runtime DI
// seam without claiming that the still-unimplemented policy components exist.
func (*amanRepository) Name() string { return "postgres AMAN repository" }

func (r *amanRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	return loadAMANAirportState(ctx, r.queries, airport)
}

func (r *amanRepository) LoadCommandOutcome(ctx context.Context, commandID string) (aman.CommandOutcome, error) {
	row, err := r.queries.GetAMANCommandOutcome(ctx, commandID)
	if err != nil {
		return aman.CommandOutcome{}, err
	}
	return commandOutcomeFromRow(row)
}

func (r *amanRepository) ListAuditRecords(ctx context.Context, airport string) ([]aman.AuditRecord, error) {
	rows, err := r.queries.ListAMANAuditRecords(ctx, airport)
	if err != nil {
		return nil, err
	}
	records := make([]aman.AuditRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, aman.AuditRecord{
			Airport:    row.Airport,
			Revision:   aman.SequenceRevision(row.Revision),
			Category:   row.Category,
			Payload:    cloneJSON(row.Payload),
			RecordedAt: timestampValue(row.RecordedAt).UTC(),
		})
	}
	return records, nil
}

func (r *amanRepository) ListValidationEvidence(ctx context.Context, airport string) ([]aman.ValidationEvidence, error) {
	rows, err := r.queries.ListAMANValidationEvidence(ctx, airport)
	if err != nil {
		return nil, err
	}
	evidence := make([]aman.ValidationEvidence, 0, len(rows))
	for _, row := range rows {
		evidence = append(evidence, aman.ValidationEvidence{
			ID:         row.EvidenceID,
			Airport:    row.Airport,
			Kind:       row.Kind,
			Payload:    cloneJSON(row.Payload),
			RecordedAt: timestampValue(row.RecordedAt).UTC(),
		})
	}
	return evidence, nil
}

// Commit provides the repository's one atomic transition. A returned result
// may be published by the caller because the database transaction has already
// committed. This repository deliberately performs no transport delivery.
func (r *amanRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return aman.CommitResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	queries := r.queries.WithTx(tx)

	if commit.CommandOutcome != nil {
		if err := queries.LockAMANCommand(ctx, commit.CommandOutcome.CommandID); err != nil {
			return aman.CommitResult{}, err
		}
		outcome, err := queries.GetAMANCommandOutcome(ctx, commit.CommandOutcome.CommandID)
		if err == nil {
			stored, err := commandOutcomeFromRow(outcome)
			if err != nil {
				return aman.CommitResult{}, err
			}
			state, err := loadAMANAirportState(ctx, queries, stored.Airport)
			if err != nil {
				return aman.CommitResult{}, err
			}
			return aman.CommitResult{State: state, CommandOutcome: &stored, DuplicateCommand: true}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return aman.CommitResult{}, err
		}
	}
	// A retry with a known command is returned above before revalidating its
	// proposed state. This guarantees idempotency even when the caller rebuilt
	// a stale command payload after the original commit.
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}

	current, err := queries.GetAMANAirportState(ctx, commit.State.Airport)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return aman.CommitResult{}, err
	}
	if errors.Is(err, pgx.ErrNoRows) && commit.ExpectedRevision != 0 {
		return aman.CommitResult{}, revisionConflict()
	}
	if err == nil && aman.SequenceRevision(current.Revision) != commit.ExpectedRevision {
		return aman.CommitResult{}, revisionConflict()
	}

	runwayGroups, err := json.Marshal(commit.State.RunwayGroups)
	if err != nil {
		return aman.CommitResult{}, fmt.Errorf("encode AMAN runway groups: %w", err)
	}
	_, err = queries.UpsertAMANAirportState(ctx, database.UpsertAMANAirportStateParams{
		Airport:       commit.State.Airport,
		Revision:      int64(commit.State.Revision),
		GeneratedAt:   requiredTimestamp(commit.State.GeneratedAt),
		PolicyVersion: commit.State.PolicyVersion,
		Mode:          string(commit.State.Mode),
		Authoritative: commit.State.Authoritative,
		RunwayGroups:  runwayGroups,
		Revision_2:    int64(commit.ExpectedRevision),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return aman.CommitResult{}, revisionConflict()
	}
	if err != nil {
		return aman.CommitResult{}, mapAMANWriteError(err)
	}

	if err := queries.DeleteAMANFlightsForAirport(ctx, commit.State.Airport); err != nil {
		return aman.CommitResult{}, err
	}
	for _, flight := range commit.State.Flights {
		payload, err := json.Marshal(flight)
		if err != nil {
			return aman.CommitResult{}, fmt.Errorf("encode AMAN flight %q: %w", flight.ID, err)
		}
		if err := queries.UpsertAMANFlight(ctx, database.UpsertAMANFlightParams{
			FlightID:        string(flight.ID),
			Airport:         commit.State.Airport,
			VatsimCid:       flight.VATSIMCID,
			CurrentCallsign: flight.CurrentCallsign,
			State:           string(flight.State),
			DataStatus:      string(flight.DataStatus),
			UpdatedAt:       requiredTimestamp(flight.UpdatedAt),
			Payload:         payload,
		}); err != nil {
			return aman.CommitResult{}, mapAMANWriteError(err)
		}
	}

	if commit.CommandOutcome != nil {
		if err := queries.CreateAMANCommandOutcome(ctx, database.CreateAMANCommandOutcomeParams{
			CommandID:  commit.CommandOutcome.CommandID,
			Airport:    commit.CommandOutcome.Airport,
			Revision:   int64(commit.CommandOutcome.Revision),
			Payload:    commit.CommandOutcome.Payload,
			RecordedAt: requiredTimestamp(commit.CommandOutcome.RecordedAt),
		}); err != nil {
			return aman.CommitResult{}, mapAMANWriteError(err)
		}
	}
	for _, record := range commit.AuditRecords {
		if err := queries.CreateAMANAuditRecord(ctx, database.CreateAMANAuditRecordParams{
			Airport: record.Airport, Revision: int64(record.Revision), Category: record.Category,
			Payload: record.Payload, RecordedAt: requiredTimestamp(record.RecordedAt),
		}); err != nil {
			return aman.CommitResult{}, err
		}
	}
	for _, evidence := range commit.ValidationEvidence {
		if err := queries.CreateAMANValidationEvidence(ctx, database.CreateAMANValidationEvidenceParams{
			EvidenceID: evidence.ID, Airport: evidence.Airport, Kind: evidence.Kind,
			Payload: evidence.Payload, RecordedAt: requiredTimestamp(evidence.RecordedAt),
		}); err != nil {
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

func loadAMANAirportState(ctx context.Context, queries *database.Queries, airport string) (aman.AirportState, error) {
	stateRow, err := queries.GetAMANAirportState(ctx, airport)
	if err != nil {
		return aman.AirportState{}, err
	}
	var runwayGroups []aman.RunwayGroupPolicy
	if err := json.Unmarshal(stateRow.RunwayGroups, &runwayGroups); err != nil {
		return aman.AirportState{}, corruptAMANData("decode runway groups", err)
	}
	flights, err := queries.ListAMANFlights(ctx, airport)
	if err != nil {
		return aman.AirportState{}, err
	}
	state := aman.AirportState{
		Airport: stateRow.Airport, Revision: aman.SequenceRevision(stateRow.Revision),
		GeneratedAt: timestampValue(stateRow.GeneratedAt).UTC(), PolicyVersion: stateRow.PolicyVersion,
		Mode: aman.RolloutMode(stateRow.Mode), Authoritative: stateRow.Authoritative, RunwayGroups: runwayGroups,
		Flights: make([]aman.AMANFlight, 0, len(flights)),
	}
	for _, row := range flights {
		var flight aman.AMANFlight
		if err := json.Unmarshal(row.Payload, &flight); err != nil {
			return aman.AirportState{}, corruptAMANData("decode flight", err)
		}
		// Identity and lifecycle columns support database constraints; prefer them
		// on load so a malformed private payload cannot bypass those guarantees.
		flight.ID = aman.FlightID(row.FlightID)
		flight.VATSIMCID = row.VatsimCid
		flight.CurrentCallsign = row.CurrentCallsign
		flight.State = aman.FlightState(row.State)
		flight.DataStatus = aman.DataStatus(row.DataStatus)
		flight.UpdatedAt = timestampValue(row.UpdatedAt).UTC()
		state.Flights = append(state.Flights, flight)
	}
	if err := state.Validate(); err != nil {
		return aman.AirportState{}, corruptAMANData("validate stored airport state", err)
	}
	return state, nil
}

func commandOutcomeFromRow(row database.AmanCommandOutcome) (aman.CommandOutcome, error) {
	outcome := aman.CommandOutcome{
		CommandID: row.CommandID, Airport: row.Airport, Revision: aman.SequenceRevision(row.Revision),
		Payload: cloneJSON(row.Payload), RecordedAt: timestampValue(row.RecordedAt).UTC(),
	}
	if !json.Valid(outcome.Payload) {
		return aman.CommandOutcome{}, corruptAMANData("decode command outcome", errors.New("payload is not JSON"))
	}
	return outcome, nil
}

func requiredTimestamp(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func cloneJSON(value []byte) []byte {
	return append([]byte(nil), value...)
}

func cloneCommandOutcome(value aman.CommandOutcome) aman.CommandOutcome {
	value.Payload = cloneJSON(value.Payload)
	return value
}

func cloneAirportState(value aman.AirportState) aman.AirportState {
	value.Flights = append([]aman.AMANFlight(nil), value.Flights...)
	value.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), value.RunwayGroups...)
	return value
}

func revisionConflict() error {
	return &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "airport revision changed before commit"}
}

func corruptAMANData(action string, err error) error {
	return &aman.DomainError{Class: aman.ErrorCorruptData, Message: fmt.Sprintf("%s: %v", action, err)}
}

func mapAMANWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "ux_aman_flights_active_vatsim_cid" {
		return &aman.DomainError{Class: aman.ErrorActiveFlightConflict, Message: "active VATSIM CID already belongs to another AMAN flight"}
	}
	return err
}

var (
	_ aman.AirportStateReader       = (*amanRepository)(nil)
	_ aman.CommandOutcomeReader     = (*amanRepository)(nil)
	_ aman.StateCommitter           = (*amanRepository)(nil)
	_ aman.AuditReader              = (*amanRepository)(nil)
	_ aman.ValidationEvidenceReader = (*amanRepository)(nil)
	_ aman.Component                = (*amanRepository)(nil)
)
