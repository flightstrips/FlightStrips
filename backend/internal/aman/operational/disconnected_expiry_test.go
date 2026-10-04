package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestDisconnectedExpiryReleasesProtectedSlotsAndPromotesStableTraffic(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			base := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
			now := base
			group := aman.RunwayGroupID("ARRIVAL-22")
			repository, publisher := &memoryRepository{}, &recordingPublisher{}
			deps := disconnectedExpiryDependencies(repository, publisher, &now)
			service, err := New(deps)
			require.NoError(t, err)
			lost := gapCommandFlight("LOST", group, base.Add(15*time.Minute), 1, aman.StateStable, freeze)
			lost.UpdatedAt = base
			lost.Prediction.GeneratedAt, lost.Prediction.InputObservedAt = base, base
			if freeze != aman.FreezeNone {
				lost.FrozenAt = &base
			}
			lost.DataStatus = aman.DataDisconnected
			lost.HoldingClearance = &aman.HoldingClearance{Hold: "MONAK", HoldType: aman.HoldingClearanceEnroute, ObservedAt: base}
			lost.HoldingStack = &aman.HoldingStackState{HoldingID: "MONAK", FirstObservedAt: base, CandidateObservedAt: base, ConsecutiveObservations: 2, Confirmed: true}
			stable := gapCommandFlight("STABLE", group, base.Add(18*time.Minute), 2, aman.StateStable, aman.FreezeNone)
			stable.UpdatedAt = base
			stable.Prediction.GeneratedAt, stable.Prediction.InputObservedAt = base, base
			stable.Prediction.RawTETA = base.Add(15 * time.Minute)
			stable.Prediction.OperationalTETA = stable.Prediction.RawTETA
			repository.state, repository.has = service.initialState("EKCH", base), true
			repository.state.Revision, repository.state.Flights = 7, []aman.AMANFlight{lost, stable}
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			first := stateFlight(t, repository.state, "LOST")
			require.Equal(t, base.Add(5*time.Minute), *first.Lifecycle.Absence.RemovalDueAt)
			require.Equal(t, lost.Slot.Time, first.Slot.Time)
			before := stateFlight(t, repository.state, "STABLE").Slot.Time

			// Restart from persisted state, without restarting the grace period.
			repository.state = cloneGapState(t, repository.state)
			service, err = New(deps)
			require.NoError(t, err)
			now = base.Add(5*time.Minute - time.Nanosecond)
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			require.Equal(t, lost.Slot.Time, stateFlight(t, repository.state, "LOST").Slot.Time)
			require.Equal(t, before, stateFlight(t, repository.state, "STABLE").Slot.Time)
			now = now.Add(time.Nanosecond)
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			removed := stateFlight(t, repository.state, "LOST")
			require.Equal(t, aman.StateRemoved, removed.State)
			require.Nil(t, removed.Slot)
			require.Nil(t, removed.FrozenSlot)
			require.Nil(t, removed.HoldingStack)
			require.Equal(t, aman.FreezeNone, removed.FreezeReason)
			promoted := stateFlight(t, repository.state, "STABLE")
			require.True(t, promoted.Slot.Time.Before(before), "stable traffic must be able to use the released capacity")
			require.Equal(t, repository.state.Revision, promoted.Slot.Revision)
			require.Equal(t, repository.state, publisher.states[len(publisher.states)-1])
			var removals []aman.AuditRecord
			for _, record := range repository.commits[len(repository.commits)-1].AuditRecords {
				if record.Category == "aman.disconnected_flight_removed" {
					removals = append(removals, record)
				}
			}
			require.Len(t, removals, 1)
			require.Equal(t, repository.state.Revision, removals[0].Revision)
			now = now.Add(time.Minute)
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			for _, record := range repository.commits[len(repository.commits)-1].AuditRecords {
				require.NotEqual(t, "aman.disconnected_flight_removed", record.Category)
			}
		})
	}
}

func TestDisconnectedUpdatesAndReconnectControlExpiry(t *testing.T) {
	for _, reconnectStatus := range []aman.DataStatus{aman.DataStale, aman.DataFresh} {
		t.Run(string(reconnectStatus), func(t *testing.T) {
			base := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
			now := base
			repository, publisher := &memoryRepository{}, &recordingPublisher{}
			service, err := New(disconnectedExpiryDependencies(repository, publisher, &now))
			require.NoError(t, err)
			eobt, eet := base.Add(time.Hour), time.Hour
			observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ENGM", Destination: "EKCH", SourceStatus: aman.DataFresh, ReconciledAt: now,
				PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}}
			require.NoError(t, service.Observe(context.Background(), observation))
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			now = base.Add(time.Minute)
			observation.SourceStatus, observation.ReconciledAt = aman.DataDisconnected, now
			// Explicit disconnected status must start expiry even without Missing.
			require.NoError(t, service.Observe(context.Background(), observation))
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			missingAt := now
			now = now.Add(4 * time.Minute)
			observation.SourceStatus, observation.ReconciledAt = reconnectStatus, now
			require.NoError(t, service.Observe(context.Background(), observation))
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			if reconnectStatus == aman.DataFresh {
				require.Nil(t, repository.state.Flights[0].Lifecycle.Absence)
			} else {
				require.Equal(t, missingAt.Add(5*time.Minute), *repository.state.Flights[0].Lifecycle.Absence.RemovalDueAt)
			}
			now = missingAt.Add(5 * time.Minute)
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			if reconnectStatus == aman.DataFresh {
				require.NotEqual(t, aman.StateRemoved, repository.state.Flights[0].State)
				// A later disappearance gets its own full grace period.
				observation.SourceStatus, observation.ReconciledAt = aman.DataDisconnected, now
				require.NoError(t, service.Observe(context.Background(), observation))
				require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
				require.Equal(t, now.Add(5*time.Minute), *repository.state.Flights[0].Lifecycle.Absence.RemovalDueAt)
			} else {
				require.Equal(t, aman.StateRemoved, repository.state.Flights[0].State)
				// PostgreSQL drops completed removals from its active projection.
				// The cached stale observation must not recreate the aircraft.
				repository.state.Flights = nil
				require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
				require.Empty(t, repository.state.Flights)
			}
		})
	}
}

func TestAutomaticExpiryAcceptsNewObservationButRejectsReplayAndManualRemoval(t *testing.T) {
	base := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Minute)
	repository, publisher := &memoryRepository{}, &recordingPublisher{}
	service, err := New(disconnectedExpiryDependencies(repository, publisher, &now))
	require.NoError(t, err)
	removed := gapCommandFlight("SAS123", "ARRIVAL-22", base.Add(20*time.Minute), 1, aman.StateStable, aman.FreezeManual)
	markMissing(&removed, base)
	markMissing(&removed, now)
	eobt, eet := now.Add(time.Hour), time.Hour
	observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ENGM", Destination: "EKCH", SourceStatus: aman.DataFresh, ReconciledAt: base,
		PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}}
	state := service.initialState("EKCH", base)
	for _, at := range []time.Time{base, now} {
		observation.ReconciledAt = at
		got, err := service.reconcileFlight(context.Background(), state, removed, observation, now)
		require.NoError(t, err)
		require.Equal(t, removed, got)
	}
	observation.ReconciledAt = now.Add(time.Second)
	got, err := service.reconcileFlight(context.Background(), state, removed, observation, observation.ReconciledAt)
	require.NoError(t, err)
	require.NotEqual(t, aman.StateRemoved, got.State)
	require.Nil(t, got.Slot, "a reconnect after expiry must not reclaim the released slot")
	require.Equal(t, aman.FreezeNone, got.FreezeReason)
	removed.Lifecycle.Reason = aman.LifecycleReasonManualRemoval
	got, err = service.reconcileFlight(context.Background(), state, removed, observation, observation.ReconciledAt)
	require.NoError(t, err)
	require.Equal(t, removed, got)
}

func TestLegacyPausedDisappearanceStillExpiresAtOriginalDeadline(t *testing.T) {
	base := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	flight := gapCommandFlight("LOST", "ARRIVAL-22", base.Add(20*time.Minute), 1, aman.StateStable, aman.FreezeTMA)
	flight.Lifecycle = &aman.LifecycleState{Absence: &aman.AbsenceState{MissingSince: base, Remaining: time.Minute}, ReconciliationPending: true}
	markMissing(&flight, base.Add(5*time.Minute))
	require.Equal(t, aman.StateRemoved, flight.State)
	require.Nil(t, flight.Slot)
	require.Nil(t, flight.FrozenSlot)
}

func TestMissingAndExpiredObservationsCannotCreatePhantomAircraft(t *testing.T) {
	base := time.Date(2026, time.October, 4, 12, 0, 0, 0, time.UTC)
	now := base.Add(5 * time.Minute)
	for _, scenario := range []struct {
		name    string
		status  aman.DataStatus
		missing bool
		at      time.Time
	}{
		{"missing", aman.DataFresh, true, now},
		{"disconnected", aman.DataDisconnected, false, now},
		{"stale", aman.DataStale, false, now},
		{"old-fresh-replay", aman.DataFresh, false, base},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repository, publisher := &memoryRepository{}, &recordingPublisher{}
			service, err := New(disconnectedExpiryDependencies(repository, publisher, &now))
			require.NoError(t, err)
			repository.state, repository.has = service.initialState("EKCH", base), true
			require.NoError(t, service.Observe(context.Background(), aman.FlightObservation{
				Callsign: "EXPIRED", Origin: "ENGM", Destination: "EKCH", SourceStatus: scenario.status, Missing: scenario.missing, ReconciledAt: scenario.at,
			}))
			require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
			require.Empty(t, repository.state.Flights)
		})
	}
}

func disconnectedExpiryDependencies(repository *memoryRepository, publisher *recordingPublisher, now *time.Time) Dependencies {
	return Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: publisher,
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeAuthoritative, Now: func() time.Time { return *now },
	}
}
