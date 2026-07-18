package postgres

import (
	"FlightStrips/internal/aman"
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
			Order:           intPtr(1), ETAReview: &aman.ETAReview{Status: "accepted"}, GoAroundDetection: &aman.GoAroundDetectionState{},
		}},
	}
}

func intPtr(value int) *int { return &value }

func requireDomainErrorClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domainErr *aman.DomainError
	require.True(t, errors.As(err, &domainErr), "expected domain error, got %v", err)
	require.Equal(t, class, domainErr.Class)
}
