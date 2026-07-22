package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAMANRepositoryRoundTripIdempotencyAndRollback(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()

	first := amanState(1, "CID-1", "SAS123")
	command := aman.CommandOutcome{
		CommandID: "command-1", Airport: first.Airport, Revision: first.Revision,
		Payload: []byte(`{"result":"accepted"}`), RecordedAt: amanTestTime.Add(time.Minute),
	}
	result, err := repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: 0, State: first, CommandOutcome: &command,
		AuditRecords:       []aman.AuditRecord{{Airport: first.Airport, Revision: first.Revision, Category: "slot_changed", Payload: []byte(`{"reason":"test"}`), RecordedAt: amanTestTime.Add(time.Minute)}},
		ValidationEvidence: []aman.ValidationEvidence{{ID: "evidence-1", Airport: first.Airport, Kind: "shadow-comparison", Payload: []byte(`{"passed":true}`), RecordedAt: amanTestTime.Add(time.Minute)}},
	})
	require.NoError(t, err)
	require.False(t, result.DuplicateCommand)
	require.Equal(t, first, result.State)

	loaded, err := NewAMANRepository(pool).LoadAirportState(ctx, first.Airport)
	require.NoError(t, err)
	require.Equal(t, first, loaded, "a reconstructed repository must restore the operational aggregate")
	audits, err := repo.ListAuditRecords(ctx, first.Airport)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, "slot_changed", audits[0].Category)
	evidence, err := repo.ListValidationEvidence(ctx, first.Airport)
	require.NoError(t, err)
	require.Len(t, evidence, 1)
	require.Equal(t, "evidence-1", evidence[0].ID)

	duplicateState := amanState(2, "CID-1", "SHOULD-NOT-PERSIST")
	duplicate, err := repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 1, State: duplicateState, CommandOutcome: &command})
	require.NoError(t, err)
	require.True(t, duplicate.DuplicateCommand)
	require.Equal(t, first, duplicate.State)
	require.Equal(t, command.CommandID, duplicate.CommandOutcome.CommandID)
	require.Equal(t, command.Airport, duplicate.CommandOutcome.Airport)
	require.Equal(t, command.Revision, duplicate.CommandOutcome.Revision)
	require.Equal(t, command.RecordedAt, duplicate.CommandOutcome.RecordedAt)
	require.JSONEq(t, string(command.Payload), string(duplicate.CommandOutcome.Payload))

	duplicateAfterRestart, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 1, State: duplicateState, CommandOutcome: &command})
	require.NoError(t, err)
	require.True(t, duplicateAfterRestart.DuplicateCommand)
	require.Equal(t, first, duplicateAfterRestart.State)

	corrected := amanState(2, "CID-1", "SAS456")
	correctedCommand := aman.CommandOutcome{CommandID: "command-2", Airport: corrected.Airport, Revision: corrected.Revision, Payload: []byte(`{"result":"corrected"}`), RecordedAt: amanTestTime.Add(2 * time.Minute)}
	_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 1, State: corrected, CommandOutcome: &correctedCommand})
	require.NoError(t, err)
	loaded, err = repo.LoadAirportState(ctx, first.Airport)
	require.NoError(t, err)
	require.Equal(t, aman.FlightID("flight-1"), loaded.Flights[0].ID, "callsign corrections must not rekey FlightID")
	require.Equal(t, "SAS456", loaded.Flights[0].CurrentCallsign)

	conflicting := amanState(3, "CID-1", "SAS456")
	second := conflicting.Flights[0]
	second.ID = "flight-2"
	second.CurrentCallsign = "SAS789"
	conflicting.Flights = append(conflicting.Flights, second)
	_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 2, State: conflicting})
	requireDomainErrorClass(t, err, aman.ErrorActiveFlightConflict)
	loaded, err = repo.LoadAirportState(ctx, first.Airport)
	require.NoError(t, err)
	require.Equal(t, corrected, loaded, "a failed transaction must leave the complete prior aggregate")
}

func TestAMANRepositoryRollsBackInvalidAuditAndCommitsStructuredAudit(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()
	state := amanState(1, "CID-AUDIT", "SAS330")

	_, err := repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: 0, State: state,
		AuditRecords: []aman.AuditRecord{{Airport: state.Airport, Revision: state.Revision, Category: "", Payload: []byte(`{"event":"rejected"}`), RecordedAt: amanTestTime}},
	})
	require.Error(t, err)
	_, err = repo.LoadAirportState(ctx, state.Airport)
	require.Error(t, err, "a rejected audit record must roll back the accompanying state")

	_, err = repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: 0, State: state,
		AuditRecords: []aman.AuditRecord{{Airport: state.Airport, Revision: state.Revision, Category: "command_accepted", Payload: []byte(`{"command_type":"freeze","outcome":"accepted"}`), RecordedAt: amanTestTime}},
	})
	require.NoError(t, err)
	audits, err := repo.ListAuditRecords(ctx, state.Airport)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.JSONEq(t, `{"command_type":"freeze","outcome":"accepted"}`, string(audits[0].Payload))
}

func TestAMANRepositoryRestoresHeldAirborneBaselineForFreshPredictor(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	reducer, err := predictor.NewReducer(predictor.Config{MaxObservationAge: 2 * time.Minute})
	require.NoError(t, err)
	filedEET := 90 * time.Minute
	flightPlanObserved := amanTestTime.Add(-time.Minute)
	first := reducer.Reduce(predictor.Input{
		Now: amanTestTime, ExpectedDestination: "EKCH", Destination: "EKCH",
		Timing:               predictor.Timing{FiledEET: &filedEET},
		Airborne:             predictor.AirborneObservation{SensedAt: &flightPlanObserved, PreviouslyObserved: true},
		FlightPlanObservedAt: &flightPlanObserved,
	}, nil)
	require.NotNil(t, first.State)

	state := amanState(1, "CID-1", "SAS123")
	state.Flights[0].ArrivalBaseline = first.State
	_, err = NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, first.State, restored.Flights[0].ArrivalBaseline)

	// A newly constructed reducer receives the stored baseline and preserves it
	// instead of recalculating from a changed source duration after restart.
	freshReducer, err := predictor.NewReducer(predictor.Config{MaxObservationAge: 2 * time.Minute})
	require.NoError(t, err)
	changedEET := 3 * time.Hour
	held := freshReducer.Reduce(predictor.Input{
		Now: amanTestTime.Add(time.Minute), ExpectedDestination: "EKCH", Destination: "EKCH",
		Timing:               predictor.Timing{FiledEET: &changedEET},
		Airborne:             predictor.AirborneObservation{SensedAt: &amanTestTime, PreviouslyObserved: true},
		FlightPlanObservedAt: &flightPlanObserved,
	}, restored.Flights[0].ArrivalBaseline)
	require.Equal(t, predictor.ReasonHeldFirstAirborneBaseline, held.Reason)
	require.Equal(t, first.State, held.State)
}

func TestAMANRepositoryRestoresETAReviewAndKeepsResolutionAtomicAndIdempotent(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repo := NewAMANRepository(pool)

	pendingState := amanState(1, "CID-REVIEW", "SAS317")
	createdAt := pendingState.Flights[0].UpdatedAt
	deadlineAt := createdAt.Add(5 * time.Minute)
	pendingState.Flights[0].ETAReview = &aman.ETAReview{
		Status: aman.ReviewPending, CreatedAt: createdAt, DeadlineAt: deadlineAt,
		InitialBaselineTETA:       createdAt.Add(20 * time.Minute),
		CalculatedOperationalTETA: pendingState.Flights[0].Prediction.OperationalTETA,
		SelectedTETA:              pendingState.Flights[0].Prediction.OperationalTETA,
	}
	pendingState.Flights[0].ArrivalBaseline = reviewBaseline(createdAt)
	openedAudit := aman.AuditRecord{
		Airport: pendingState.Airport, Revision: pendingState.Revision, Category: "eta_review_opened",
		Payload: []byte(`{"status":"pending"}`), RecordedAt: createdAt,
	}
	_, err := repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: pendingState, AuditRecords: []aman.AuditRecord{openedAudit}})
	require.NoError(t, err)
	restoredPending, err := NewAMANRepository(pool).LoadAirportState(ctx, pendingState.Airport)
	require.NoError(t, err)
	require.Equal(t, pendingState, restoredPending)

	manualState := amanState(2, "CID-REVIEW", "SAS317")
	resolvedAt := createdAt.Add(time.Minute)
	manualTETA := createdAt.Add(26 * time.Minute)
	actor := "1345678"
	manualState.GeneratedAt = resolvedAt
	manualState.Flights[0].UpdatedAt = resolvedAt
	manualState.Flights[0].Prediction.OperationalTETA = manualTETA
	manualState.Flights[0].Prediction.OperationalReason = aman.OperationalReasonManualOverride
	manualState.Flights[0].FreezeReason = aman.FreezeManual
	manualState.Flights[0].FrozenAt = &resolvedAt
	manualState.Flights[0].FrozenOperationalTETA = &manualTETA
	manualState.Flights[0].ETAReview = &aman.ETAReview{
		Status: aman.ReviewManualETA, CreatedAt: createdAt, DeadlineAt: deadlineAt, ResolvedAt: &resolvedAt, Actor: &actor,
		InitialBaselineTETA:       createdAt.Add(20 * time.Minute),
		CalculatedOperationalTETA: pendingState.Flights[0].Prediction.OperationalTETA,
		SelectedTETA:              manualTETA, ManualTETA: &manualTETA,
	}
	manualState.Flights[0].ArrivalBaseline = reviewBaseline(createdAt)
	command := aman.CommandOutcome{
		CommandID: "review-command-1", Airport: manualState.Airport, Revision: manualState.Revision,
		Payload: []byte(`{"status":"manual_eta"}`), RecordedAt: resolvedAt,
	}
	audit := aman.AuditRecord{
		Airport: manualState.Airport, Revision: manualState.Revision, Category: "eta_review_resolved",
		Payload: []byte(`{"status":"manual_eta"}`), RecordedAt: resolvedAt,
	}
	_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 1, State: manualState, CommandOutcome: &command, AuditRecords: []aman.AuditRecord{audit}})
	require.NoError(t, err)
	restoredManual, err := NewAMANRepository(pool).LoadAirportState(ctx, manualState.Airport)
	require.NoError(t, err)
	require.Equal(t, manualState, restoredManual)
	audits, err := repo.ListAuditRecords(ctx, manualState.Airport)
	require.NoError(t, err)
	require.Len(t, audits, 2)
	require.Equal(t, "eta_review_opened", audits[0].Category)
	require.Equal(t, "eta_review_resolved", audits[1].Category)

	duplicateProposal := amanState(3, "CID-REVIEW", "SHOULD-NOT-PERSIST")
	duplicate, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 2, State: duplicateProposal, CommandOutcome: &command})
	require.NoError(t, err)
	require.True(t, duplicate.DuplicateCommand)
	require.Equal(t, manualState, duplicate.State)

	failedReset := amanState(3, "CID-REVIEW", "SAS317")
	_, err = repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: 2, State: failedReset,
		AuditRecords: []aman.AuditRecord{{Airport: failedReset.Airport, Revision: failedReset.Revision, Category: "", Payload: []byte(`{}`), RecordedAt: createdAt.Add(2 * time.Minute)}},
	})
	require.Error(t, err)
	afterFailure, err := repo.LoadAirportState(ctx, manualState.Airport)
	require.NoError(t, err)
	require.Equal(t, manualState, afterFailure, "failed reset must expose the complete state from before the transaction")
}

func TestAMANRepositoryCompareAndSwapAllocatesOneRevision(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	repo := NewAMANRepository(pool)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, callsign := range []string{"SAS111", "SAS222"} {
		wg.Add(1)
		go func(callsign string) {
			defer wg.Done()
			_, err := repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: amanState(1, "CID-"+callsign, callsign)})
			results <- err
		}(callsign)
	}
	wg.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var domainErr *aman.DomainError
		require.True(t, errors.As(err, &domainErr), "unexpected race error: %v", err)
		require.Equal(t, aman.ErrorRevisionConflict, domainErr.Class)
		conflicts++
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	state, err := repo.LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(1), state.Revision)
}

func TestAMANVATSIMObservationIdentitySurvivesRestartCorrectsCallsignAndRetires(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	firstRepository := NewAMANRepository(pool)
	first, err := firstRepository.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123456", CurrentCallsign: "SAS123"})
	require.NoError(t, err)
	require.NotEmpty(t, first)

	// A reconstructed repository must find the same active flight and update
	// only its mutable callsign.
	secondRepository := NewAMANRepository(pool)
	corrected, err := secondRepository.BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123456", CurrentCallsign: "SAS456"})
	require.NoError(t, err)
	require.Equal(t, first, corrected)
	var callsign string
	require.NoError(t, pool.QueryRow(ctx, "SELECT current_callsign FROM aman_vatsim_observation_identities WHERE flight_id = $1", string(first)).Scan(&callsign))
	require.Equal(t, "SAS456", callsign)

	require.NoError(t, secondRepository.RetireVATSIMFlight(ctx, first))
	next, err := NewAMANRepository(pool).BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123456", CurrentCallsign: "SAS789"})
	require.NoError(t, err)
	require.NotEqual(t, first, next, "a later flight from the same VATSIM user receives a new FlightID")
	requireDomainErrorClass(t, secondRepository.RetireVATSIMFlight(ctx, first), aman.ErrorNotFound)
}

func TestAMANVATSIMObservationIdentityAllowsOnlyOneConcurrentActiveCID(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	start := make(chan struct{})
	ids := make(chan aman.FlightID, 2)
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, callsign := range []string{"SAS123", "SAS456"} {
		wait.Add(1)
		go func(callsign string) {
			defer wait.Done()
			<-start
			id, err := NewAMANRepository(pool).BindVATSIMFlight(ctx, aman.VATSIMFlightIdentity{VATSIMCID: "123456", CurrentCallsign: callsign})
			ids <- id
			errs <- err
		}(callsign)
	}
	close(start)
	wait.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var observed []aman.FlightID
	for id := range ids {
		observed = append(observed, id)
	}
	require.Len(t, observed, 2)
	require.Equal(t, observed[0], observed[1])
	var activeCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM aman_vatsim_observation_identities WHERE vatsim_cid = $1 AND retired_at IS NULL", "123456").Scan(&activeCount))
	require.Equal(t, 1, activeCount)
}

func TestAMANPersistenceDoesNotDependOnTransportOrCreateOutbox(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	base := filepath.Dir(sourceFile)
	repositorySource, err := os.ReadFile(filepath.Join(base, "aman.go"))
	require.NoError(t, err)
	for _, forbidden := range []string{
		"internal/frontend", "internal/websocket", "internal/euroscope", "internal/alb",
	} {
		require.NotContains(t, string(repositorySource), forbidden)
	}
	migration, err := os.ReadFile(filepath.Join(base, "..", "..", "..", "migrations", "0034-add-aman-persistence.sql"))
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(migration)), "outbox")
}

var amanTestTime = time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

func amanState(revision aman.SequenceRevision, vatsimCID, callsign string) aman.AirportState {
	flightTime := amanTestTime.Add(time.Duration(revision) * time.Minute)
	return aman.AirportState{
		Airport: "EKCH", Revision: revision, GeneratedAt: flightTime, PolicyVersion: "policy-v1",
		Mode: aman.ModeShadow, RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north"}},
		Flights: []aman.AMANFlight{{
			ID: aman.FlightID("flight-1"), VATSIMCID: vatsimCID, CurrentCallsign: callsign,
			State: aman.StateStable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone,
			UpdatedAt: flightTime,
			Prediction: &aman.Prediction{
				RawTETA: flightTime.Add(20 * time.Minute), OperationalTETA: flightTime.Add(21 * time.Minute), OperationalReason: "smoothed",
				GeneratedAt: flightTime, InputObservedAt: flightTime.Add(-time.Minute), Confidence: aman.ConfidenceHigh,
				Publishable: true, DatasetVersion: "2026-07", GeometryDigest: "geometry", ModelVersion: "model-v1", ConfigVersion: "config-v1", Sources: []string{"surveillance"},
			},
			ActiveRouteFact: &aman.RouteFact{ID: "route-1", Fix: "KAS", ObservedAt: flightTime},
			Slot:            &aman.Slot{Time: flightTime.Add(22 * time.Minute), RunwayGroupID: "north", Sequence: 1, Revision: revision, Reason: "spacing"},
			Order:           intPtr(1), GoAroundDetection: &aman.GoAroundDetectionState{},
		}},
	}
}

func intPtr(value int) *int { return &value }

func reviewBaseline(createdAt time.Time) *aman.BaselineState {
	return &aman.BaselineState{
		ArrivalAt: createdAt.Add(20 * time.Minute), AirborneSensedAt: createdAt.Add(-time.Hour),
		Source: aman.BaselineSourceAirborneFiledEET, Confidence: aman.ConfidenceMedium,
		FlightPlanObservedAt: createdAt.Add(-time.Hour), ModelVersion: "baseline-v1", ConfigVersion: "baseline-config-v1",
	}
}

func requireDomainErrorClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domainErr *aman.DomainError
	require.True(t, errors.As(err, &domainErr), "expected domain error, got %v", err)
	require.Equal(t, class, domainErr.Class)
}
