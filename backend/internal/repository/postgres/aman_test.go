package postgres

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/pdc/testdata"
	"context"
	"encoding/json"
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

func TestAMANRepositoryRestartsWithActiveRunwaySetAndDecodesLegacySelection(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-ACTIVE", "SAS101")
	state.RunwayGroups[0].Selected = true
	state.RunwayGroups = append(state.RunwayGroups, aman.RunwayGroupPolicy{ID: "south"})
	state.ActiveRunwayGroups = []aman.RunwayGroupID{"north", "south"}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	var stored []byte
	require.NoError(t, pool.QueryRow(ctx, "SELECT runway_groups FROM aman_airport_states WHERE airport = $1", state.Airport).Scan(&stored))
	require.JSONEq(t, `[{"ID":"north","Active":true,"Selected":true,"SelectionSchedule":null,"SelectionConflict":null,"ActiveRatePerHour":0,"RateEffectiveAt":null,"RateSchedule":null,"SameSTARSpacing":null,"SequenceWarnings":null,"Gaps":null,"Closures":null,"CapacityReservations":null},{"ID":"south","Active":true,"Selected":false,"SelectionSchedule":null,"SelectionConflict":null,"ActiveRatePerHour":0,"RateEffectiveAt":null,"RateSchedule":null,"SameSTARSpacing":null,"SequenceWarnings":null,"Gaps":null,"Closures":null,"CapacityReservations":null}]`, string(stored))
	var legacyDecoder []struct {
		ID       aman.RunwayGroupID
		Selected bool
	}
	require.NoError(t, json.Unmarshal(stored, &legacyDecoder), "legacy decoders must ignore the additive marker")
	require.Equal(t, "north", string(legacyDecoder[0].ID))
	require.True(t, legacyDecoder[0].Selected)

	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored, "a reconstructed repository must preserve the complete active set")

	_, err = pool.Exec(ctx, `UPDATE aman_airport_states SET runway_groups = '[{"ID":"north","Selected":true}]' WHERE airport = $1`, state.Airport)
	require.NoError(t, err)
	legacy, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, []aman.RunwayGroupID{"north"}, legacy.ActiveRunwayGroups)
	require.True(t, legacy.RunwayGroups[0].Selected, "legacy Selected remains the deterministic compatibility source")
}

func TestAMANRepositoryRestartsWithProtectedSameSTARWarning(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-WARNING", "SAS101")
	state.RunwayGroups[0].SequenceWarnings = []aman.RunwayGroupSequenceWarning{{
		Code: "protected_same_star_spacing", FlightID: "flight-2", RelatedFlightID: "flight-1", STARFamily: "MONAK",
	}}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state.RunwayGroups[0].SequenceWarnings, restored.RunwayGroups[0].SequenceWarnings)
}

func TestAMANRepositoryRestartsWithRunwayGaps(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-GAP", "SAS101")
	state.RunwayGroups[0].Gaps = []aman.RunwayGap{
		{ID: "gap-1", Start: amanTestTime.Add(time.Hour), End: amanTestTime.Add(70 * time.Minute), Label: "approach stop", CreatedAt: amanTestTime, CreatedBy: "controller-1"},
		{ID: "gap-2", Start: amanTestTime.Add(2 * time.Hour), End: amanTestTime.Add(130 * time.Minute), Label: "runway inspection", CreatedAt: amanTestTime.Add(time.Minute), CreatedBy: "controller-2"},
	}

	committed, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	require.Equal(t, state.RunwayGroups[0].Gaps, committed.State.RunwayGroups[0].Gaps)

	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored, "a reconstructed repository must preserve runway-owned GAP intervals")
}

func TestAMANRepositoryRestartsWithRunwayClosures(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-CLOSURE", "SAS101")
	end := amanTestTime.Add(2 * time.Hour)
	state.RunwayGroups[0].Closures = []aman.RunwayClosure{
		{ID: "finite", Start: amanTestTime.Add(time.Hour), End: &end, Reason: "runway inspection", CreatedAt: amanTestTime, CreatedBy: "controller-1"},
		{ID: "indefinite", Start: amanTestTime.Add(3 * time.Hour), Reason: "surface damage", CreatedAt: amanTestTime.Add(time.Minute), CreatedBy: "controller-2"},
	}

	committed, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	require.Equal(t, state.RunwayGroups[0].Closures, committed.State.RunwayGroups[0].Closures)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored, "a reconstructed repository must preserve finite and indefinite runway closures")
}

func TestAMANRepositoryRestartsWithMergedRunwayGapUnion(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-GAP-UNION", "SAS102")
	state.RunwayGroups[0].Gaps = []aman.RunwayGap{
		{ID: "first", Start: amanTestTime.Add(time.Hour), End: amanTestTime.Add(70 * time.Minute), Label: "first reason", CreatedAt: amanTestTime, CreatedBy: "controller-1"},
		{ID: "second", Start: amanTestTime.Add(80 * time.Minute), End: amanTestTime.Add(90 * time.Minute), Label: "second reason", CreatedAt: amanTestTime, CreatedBy: "controller-2"},
	}
	end := amanTestTime.Add(80 * time.Minute)
	interval, err := aman.NormalizeRunwayGapInterval(aman.RunwayGapIntervalInput{Start: amanTestTime.Add(70 * time.Minute), End: &end}, 20)
	require.NoError(t, err)
	merged, err := aman.MergeRunwayGap(state.RunwayGroups, aman.RunwayGapMergeInput{
		RunwayGroupID: "north", CommandID: "merge-command", Interval: interval,
		Label: "combined stop", CreatedAt: amanTestTime.Add(time.Minute), CreatedBy: "controller-3",
	})
	require.NoError(t, err)
	require.Equal(t, []aman.RunwayGapID{"first", "second"}, merged.ReplacedIDs)
	state.RunwayGroups = merged.RunwayGroups

	_, err = NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored)
	require.Equal(t, []aman.RunwayGap{merged.Union}, restored.RunwayGroups[0].Gaps, "restart must not restore replaced fragments")
}

func TestAMANRepositoryPersistsNoOpCommandWithoutAdvancingState(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	repo := NewAMANRepository(pool)
	state := amanState(1, "CID-NOOP", "SAS100")
	_, err := repo.Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)

	outcome := aman.CommandOutcome{
		CommandID: "no-op-command", Airport: state.Airport, Revision: state.Revision,
		Payload: []byte(`{"result":"unchanged"}`), RecordedAt: amanTestTime.Add(2 * time.Minute),
	}
	audit := aman.AuditRecord{
		Airport: state.Airport, Revision: state.Revision, Category: "command_unchanged",
		Payload: []byte(`{"result":"unchanged"}`), RecordedAt: outcome.RecordedAt,
	}
	result, err := repo.Commit(ctx, aman.StateCommit{
		ExpectedRevision: state.Revision, State: state, CommandOutcome: &outcome, AuditRecords: []aman.AuditRecord{audit},
	})
	require.NoError(t, err)
	require.False(t, result.DuplicateCommand)
	require.Equal(t, state, result.State)

	restarted := NewAMANRepository(pool)
	loaded, err := restarted.LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, loaded)
	storedOutcome, err := restarted.LoadCommandOutcome(ctx, outcome.CommandID)
	require.NoError(t, err)
	require.Equal(t, outcome.CommandID, storedOutcome.CommandID)
	require.Equal(t, outcome.Airport, storedOutcome.Airport)
	require.Equal(t, outcome.Revision, storedOutcome.Revision)
	require.Equal(t, outcome.RecordedAt, storedOutcome.RecordedAt)
	require.JSONEq(t, string(outcome.Payload), string(storedOutcome.Payload))
	audits, err := restarted.ListAuditRecords(ctx, state.Airport)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, state.Revision, audits[0].Revision)

	invalidNoOp := loaded
	invalidNoOp.Flights[0].CurrentCallsign = "SHOULD-NOT-PERSIST"
	_, err = restarted.Commit(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: invalidNoOp})
	requireDomainErrorClass(t, err, aman.ErrorInvalidArgument)
	loaded, err = restarted.LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, loaded)
}

func TestAMANRepositoryDoesNotPersistRemovedFlights(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-REMOVED", "SAS999")
	removed := state.Flights[0]
	removed.State = aman.StateRemoved
	removed.Slot, removed.Order, removed.ManualOrder, removed.QueueOffers = nil, nil, nil, nil
	removed.FreezeReason, removed.FrozenAt, removed.FrozenOperationalTETA, removed.FrozenSlot = aman.FreezeNone, nil, nil, nil
	state.Flights = []aman.AMANFlight{removed}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	var count int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM aman_flights WHERE state = 'removed'").Scan(&count))
	require.Zero(t, count)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Empty(t, restored.Flights)
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

func TestAMANRepositoryRestoresAuthoritativeHoldingClearance(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-HOLD", "SAS318")
	altitude := int32(12000)
	state.Flights[0].HoldingClearance = &aman.HoldingClearance{
		Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: "1422",
		ClearedAltitude: &altitude, ObservedAt: amanTestTime,
	}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state.Flights[0].HoldingClearance, restored.Flights[0].HoldingClearance)
}

func TestAMANRepositoryRestoresExplicitTerminalIdentitiesAfterRestart(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-IDENTITY", "SAS553")
	legacy, starFamily, feederFix := "TESPI", "TESPI", "TNO"
	state.Flights[0].SelectedFeeder = &legacy
	state.Flights[0].SelectedSTARFamily = &starFamily
	state.Flights[0].SelectedFeederFix = &feederFix

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, legacy, *restored.Flights[0].SelectedFeeder)
	require.Equal(t, starFamily, *restored.Flights[0].SelectedSTARFamily)
	require.Equal(t, feederFix, *restored.Flights[0].SelectedFeederFix)
}

func TestDecodeLegacyAMANFlightRestoresSTARFamilyOnly(t *testing.T) {
	flight, err := decodeAMANFlightPayload([]byte(`{"SelectedFeeder":"TESPI"}`))
	require.NoError(t, err)
	require.Equal(t, aman.SequenceDispositionActive, flight.SequenceDisposition)
	require.Equal(t, "TESPI", *flight.SelectedFeeder)
	require.Equal(t, "TESPI", *flight.SelectedSTARFamily)
	require.Nil(t, flight.SelectedFeederFix)
}

func TestDecodeAMANFlightPayloadRejectsInvalidSequenceDisposition(t *testing.T) {
	for _, encoded := range []string{`{"SequenceDisposition":""}`, `{"SequenceDisposition":"removed"}`, `{"SequenceDisposition":null}`} {
		_, err := decodeAMANFlightPayload([]byte(encoded))
		require.Error(t, err, encoded)
	}
}

func TestAMANRepositoryRestoresSequenceDispositionAfterRestart(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-DISPOSITION", "SAS588")
	state.Flights[0].SequenceDisposition = aman.SequenceDispositionDesequenced

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restarted, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restarted)
	require.Equal(t, aman.SequenceDispositionDesequenced, restarted.Flights[0].SequenceDisposition)
}

func TestDecodeAMANFlightPayloadPreservesOptionalFeederETAState(t *testing.T) {
	legacy, err := decodeAMANFlightPayload([]byte(`{"SelectedFeeder":"TESPI"}`))
	require.NoError(t, err)
	require.Nil(t, legacy.FeederETA, "old persisted payloads must not invent feeder timing")

	encoded := []byte(`{"SelectedFeeder":"TESPI","SelectedSTARFamily":"TESPI","SelectedFeederFix":"TNO","FeederETA":{"ETA":"2026-09-11T20:10:00Z","Source":"manual","Passed":false}}`)
	restored, err := decodeAMANFlightPayload(encoded)
	require.NoError(t, err)
	require.NotNil(t, restored.FeederETA)
	require.Equal(t, aman.FeederETASourceManual, restored.FeederETA.Source)
	require.Equal(t, time.Date(2026, time.September, 11, 20, 10, 0, 0, time.UTC), *restored.FeederETA.ETA)
}

func TestAMANRepositoryRestoresRevisionBoundQueueOffers(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-QUEUE-1", "SAS321")
	first := state.Flights[0]
	target := first
	target.ID = "flight-2"
	target.VATSIMCID = "CID-QUEUE-2"
	target.CurrentCallsign = "SAS322"
	target.Slot = &aman.Slot{
		Time: first.Slot.Time.Add(time.Minute), RunwayGroupID: first.Slot.RunwayGroupID,
		Sequence: 2, Revision: state.Revision, Reason: "rate_wtc",
	}
	target.Order = intPtr(2)
	target.QueueOffers = []aman.QueueOffer{{
		FlightID: target.ID, RunwayGroupID: first.Slot.RunwayGroupID, CandidateSlot: *first.Slot,
		QueuePosition: 1, ExpiresAt: first.Slot.Time, AirportRevision: state.Revision,
		Reason: aman.QueueOfferEarlierOccupiedSlot,
	}}
	state.Flights = append(state.Flights, target)

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state, restored)
}

func TestAMANRepositoryRestoresTMAEntryStateAndAcceptsLegacyFlightPayload(t *testing.T) {
	pool, _ := testdata.SetupTestDB(t)
	ctx := context.Background()
	state := amanState(1, "CID-TMA", "SAS321")
	state.Flights[0].TMAEntry = &aman.TMAEntryState{
		LastContainment: aman.TMAInside, LastObservedAt: state.GeneratedAt.Add(-time.Second), FreezeTriggered: true,
	}

	_, err := NewAMANRepository(pool).Commit(ctx, aman.StateCommit{ExpectedRevision: 0, State: state})
	require.NoError(t, err)
	restored, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Equal(t, state.Flights[0].TMAEntry, restored.Flights[0].TMAEntry)

	// A payload written before TMAEntry existed has no field and must continue
	// to load as an unobserved arrival episode.
	_, err = pool.Exec(ctx, `UPDATE aman_flights SET payload = payload - 'TMAEntry' WHERE flight_id = $1`, string(state.Flights[0].ID))
	require.NoError(t, err)
	legacy, err := NewAMANRepository(pool).LoadAirportState(ctx, state.Airport)
	require.NoError(t, err)
	require.Nil(t, legacy.Flights[0].TMAEntry)
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
			State: aman.StateStable, SequenceDisposition: aman.SequenceDispositionActive,
			DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone,
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
