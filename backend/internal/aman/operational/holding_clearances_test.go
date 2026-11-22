package operational

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/holdingclearance"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

func TestEuroScopeClearancePublishesArrivalCFLWithoutReconciliation(t *testing.T) {
	for _, hold := range []string{"", "TESPI"} {
		name := "without a hold"
		if hold != "" {
			name = "with a hold"
		}
		t.Run(name, func(t *testing.T) {
			_, state, now := holdingClearanceRefreshFixture(t)
			state.GeneratedAt = now
			flight := &state.Flights[0]
			oldAltitude := 9000
			flight.LatestObservation.ClearedAltitude = &oldAltitude
			strip := &models.Strip{Callsign: flight.Callsign, Destination: state.Airport, Hold: hold, HoldType: "enroute"}
			repository := &memoryRepository{state: state, has: true}
			publisher := &recordingPublisher{}
			clearances, err := holdingclearance.New(holdingclearance.Dependencies{Repository: repository, Publisher: publisher})
			require.NoError(t, err)
			observer, err := NewEuroScopeHoldingClearanceObserver(EuroScopeHoldingClearanceObserverDependencies{
				Sink: clearances, Now: func() time.Time { return now },
			})
			require.NoError(t, err)
			health := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveAuthoritative}
			legacy, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
			require.NoError(t, err)
			require.Equal(t, &oldAltitude, legacy.Data.Flights[0].ClearedAltitude)

			for index, altitude := range []int32{7000, 1, 2, 0} {
				strip.ClearedAltitude = &altitude
				now = now.Add(time.Second)
				require.NoError(t, observer.ObserveClearedAltitude(t.Context(), strip))
				require.Len(t, publisher.states, index+1)
				published := publisher.states[index]
				event, err := frontendEvents.NewAMANStateEvent(published, health.EffectiveMode, health)
				require.NoError(t, err)
				var expected *int
				if altitude > 0 {
					level := int(altitude)
					expected = &level
				}
				require.Equal(t, expected, event.Data.Flights[0].ClearedAltitude)
				require.Equal(t, &oldAltitude, published.Flights[0].LatestObservation.ClearedAltitude)
				if hold != "" {
					require.Len(t, event.Data.HoldingInformation, 1)
					require.Equal(t, &altitude, event.Data.HoldingInformation[0].ClearedAltitude)
				} else {
					require.Empty(t, event.Data.HoldingInformation)
				}
			}
		})
	}
}

func TestDedicatedClearanceReassertsRetainedCFLAfterPositionRecovery(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		retained int32
		hold     string
	}{
		{name: "restore retained level", retained: 9000, hold: "TESPI"},
		{name: "clear again"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, state, now := holdingClearanceRefreshFixture(t)
			holdingAt := now
			positionAt := now.Add(time.Second)
			state.GeneratedAt = positionAt
			flight := &state.Flights[0]
			strip := &models.Strip{
				ID: 42, Version: 1,
				Callsign: flight.Callsign, Destination: state.Airport, Hold: scenario.hold,
				HoldType: "enroute", ClearedAltitude: &scenario.retained,
			}
			flight.HoldingClearance = normalizedStripHoldingClearance(strip, holdingAt)
			recovered := 7000
			flight.LatestObservation.Provider = aman.ObservationProviderEuroScope
			flight.LatestObservation.FlightPlan.ObservedAt = &positionAt
			flight.LatestObservation.ClearedAltitude = &recovered
			flight.LatestObservation.ClearedAltitudeStripRevision = aman.StripRevision{ID: strip.ID, Version: 2}

			repository := &holdingSnapshotRepository{memoryRepository: memoryRepository{state: state, has: true}}
			publisher := &recordingPublisher{}
			clearances, err := holdingclearance.New(holdingclearance.Dependencies{Repository: repository, Publisher: publisher})
			require.NoError(t, err)
			observer, err := NewEuroScopeHoldingClearanceObserver(EuroScopeHoldingClearanceObserverDependencies{
				Sink: clearances, Now: func() time.Time { return now },
			})
			require.NoError(t, err)
			health := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveAuthoritative}
			before, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
			require.NoError(t, err)
			require.Equal(t, &recovered, before.Data.Flights[0].ClearedAltitude)

			now = positionAt.Add(time.Second)
			require.NoError(t, observer.ObserveHoldingClearance(t.Context(), strip))
			require.Empty(t, publisher.states, "a replay of the same strip revision is still deduplicated")
			strip.Version = 3
			require.NoError(t, observer.ObserveClearedAltitude(t.Context(), strip))
			require.Len(t, publisher.states, 1)
			published := publisher.states[0]
			require.Equal(t, state.Revision+1, published.Revision)
			clearance := published.Flights[0].HoldingClearance
			require.Equal(t, holdingAt, clearance.ObservedAt, "reasserting the CFL preserves the holding observation time")
			require.Equal(t, now, clearance.LatestStripClearanceTime())
			updated, err := frontendEvents.NewAMANStateEvent(published, health.EffectiveMode, health)
			require.NoError(t, err)
			if scenario.retained == 0 {
				require.Nil(t, updated.Data.Flights[0].ClearedAltitude)
			} else {
				require.EqualValues(t, scenario.retained, *updated.Data.Flights[0].ClearedAltitude)
			}

			require.NoError(t, observer.ObserveClearedAltitude(t.Context(), strip))
			now = now.Add(time.Second)
			strip.Version = 2
			oldAltitude := int32(recovered)
			strip.ClearedAltitude = &oldAltitude
			require.NoError(t, observer.ObserveHoldingClearance(t.Context(), strip))
			require.Len(t, publisher.states, 1, "replays and stale strip snapshots cannot undo the dedicated clearance")
		})
	}
}

func TestFlightCreationAndRecoveryDiscardConflictingHoldingClearance(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		stripID     int32
		positionCFL int
		wantHold    bool
	}{
		{name: "different CFL", stripID: 42, positionCFL: 7000},
		{name: "replacement strip with the same CFL", stripID: 43, positionCFL: 9000},
		{name: "same strip confirms the CFL", stripID: 42, positionCFL: 9000, wantHold: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			previous := state.Flights[0]
			level := int32(9000)
			observation := *previous.LatestObservation
			observation.Provider = aman.ObservationProviderEuroScope
			observation.ReconciledAt = now.Add(time.Second)
			observation.ClearedAltitude = &scenario.positionCFL
			observation.ClearedAltitudeStripRevision = aman.StripRevision{ID: scenario.stripID, Version: 19}
			observation.HoldingClearance = normalizedStripHoldingClearance(&models.Strip{
				ID: 42, Version: 18, Hold: "TESPI", HoldType: "enroute", ClearedAltitude: &level,
			}, now)
			admitted := newFlight(observation, observation.ReconciledAt)

			previous.State = aman.StateRemoved
			previous.Lifecycle = &aman.LifecycleState{Reason: aman.LifecycleReasonSourceDisappearance, EnteredAt: now}
			restored, err := service.reconcileFlight(t.Context(), state, previous, observation, observation.ReconciledAt)
			require.NoError(t, err)
			require.NotEqual(t, aman.StateRemoved, restored.State)
			for _, flight := range []aman.AMANFlight{admitted, restored} {
				if scenario.wantHold {
					require.Equal(t, observation.HoldingClearance, flight.HoldingClearance)
				} else {
					require.Nil(t, flight.HoldingClearance)
					require.Nil(t, flight.LatestObservation.HoldingClearance)
				}
				require.Equal(t, &scenario.positionCFL, flight.LatestObservation.ClearedAltitude)
			}
		})
	}
}

func TestDedicatedClearancePublicationFollowsPersistedStripVersion(t *testing.T) {
	for _, scenario := range []struct {
		name           string
		older, newer   int32
		commitConflict bool
	}{
		{name: "late level cannot undo a newer clear", older: 9000},
		{name: "late clear cannot undo a newer level", newer: 7000},
		{name: "retry after another handler commits", older: 9000, newer: 7000, commitConflict: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, state, now := holdingClearanceRefreshFixture(t)
			repository := &holdingSnapshotRepository{memoryRepository: memoryRepository{state: state, has: true}}
			publisher := &recordingPublisher{}
			clearances, err := holdingclearance.New(holdingclearance.Dependencies{Repository: repository, Publisher: publisher})
			require.NoError(t, err)
			observer, err := NewEuroScopeHoldingClearanceObserver(EuroScopeHoldingClearanceObserverDependencies{
				Sink: clearances, Now: func() time.Time { return now },
			})
			require.NoError(t, err)
			older := models.Strip{ID: 42, Version: 17, Callsign: state.Flights[0].Callsign, Destination: state.Airport, ClearedAltitude: &scenario.older}
			newer := older
			newer.Version++
			newer.ClearedAltitude = &scenario.newer
			now = now.Add(2 * time.Second)
			if scenario.commitConflict {
				repository.beforeCommit = func() {
					now = now.Add(-time.Second)
					require.NoError(t, observer.ObserveClearedAltitude(t.Context(), &newer))
				}
			} else {
				require.NoError(t, observer.ObserveClearedAltitude(t.Context(), &newer))
				now = now.Add(time.Second)
			}
			require.NoError(t, observer.ObserveClearedAltitude(t.Context(), &older))
			require.NoError(t, observer.ObserveClearedAltitude(t.Context(), &newer))
			require.Len(t, publisher.states, 1)
			published := publisher.states[0]
			require.Equal(t, state.Revision+1, published.Revision)
			require.Equal(t, stripRevision(&newer), published.Flights[0].HoldingClearance.StripRevision)
			health := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveAuthoritative}
			event, err := frontendEvents.NewAMANStateEvent(published, health.EffectiveMode, health)
			require.NoError(t, err)
			if scenario.newer == 0 {
				require.Nil(t, event.Data.Flights[0].ClearedAltitude)
			} else {
				require.EqualValues(t, scenario.newer, *event.Data.Flights[0].ClearedAltitude)
			}
		})
	}
}

func TestDelayedFullSyncCannotRestoreClearedAltitude(t *testing.T) {
	service, state, now := holdingClearanceRefreshFixture(t)
	flight := &state.Flights[0]
	assigned := int32(9000)
	cleared := int32(0)
	current := &models.Strip{
		ID: 42, Version: 18, Callsign: flight.Callsign, Origin: "ESSA", Destination: state.Airport, ClearedAltitude: &cleared,
	}
	flight.HoldingClearance = normalizedStripHoldingClearance(current, now)

	delayed := *current
	delayed.Version--
	delayed.ClearedAltitude = &assigned
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink})
	require.NoError(t, err)
	state.GeneratedAt = now.Add(time.Second)
	require.NoError(t, observer.ObserveEuroScopeStrips(t.Context(), 0, []shared.EuroScopeStripObservation{
		{Strip: &delayed, ObservedAt: state.GeneratedAt},
	}))
	require.Len(t, sink.observations, 1)
	updated, err := service.reconcileFlight(t.Context(), state, *flight, sink.observations[0], state.GeneratedAt)
	require.NoError(t, err)
	*flight = updated
	require.Equal(t, stripRevision(current), flight.HoldingClearance.StripRevision)
	health := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveAuthoritative}
	event, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
	require.NoError(t, err)
	require.Nil(t, event.Data.Flights[0].ClearedAltitude)
}

func TestSourceMergeKeepsNewerHoldingCancellation(t *testing.T) {
	service, state, now := holdingClearanceRefreshFixture(t)
	level := int32(9000)
	old := *state.Flights[0].LatestObservation
	old.Provider = aman.ObservationProviderEuroScope
	old.ClearedAltitude = new(int)
	*old.ClearedAltitude = int(level)
	old.ClearedAltitudeStripRevision = aman.StripRevision{ID: 42, Version: 18}
	old.HoldingClearance = normalizedStripHoldingClearance(&models.Strip{
		ID: 42, Version: 18, Hold: "TESPI", HoldType: "enroute", ClearedAltitude: &level,
	}, now)
	current := old
	current.ClearedAltitudeStripRevision.Version = 20
	current.HoldingClearance = normalizedStripHoldingClearance(&models.Strip{ID: 42, Version: 20, ClearedAltitude: &level}, now.Add(time.Second))
	delayed := old
	delayed.ReconciledAt = now.Add(2 * time.Second)
	delayed.FlightPlan.ObservedAt = &delayed.ReconciledAt
	merged := mergeSurveillanceObservation(current, delayed)
	require.Equal(t, current.HoldingClearance, merged.HoldingClearance)
	require.Equal(t, current.ClearedAltitudeStripRevision, merged.ClearedAltitudeStripRevision)
	admitted := newFlight(merged, delayed.ReconciledAt)
	require.Empty(t, admitted.HoldingClearance.Hold)
	restored := state.Flights[0]
	restored.State = aman.StateRemoved
	restored.Lifecycle = &aman.LifecycleState{Reason: aman.LifecycleReasonSourceDisappearance, EnteredAt: now}
	updated, err := service.reconcileFlight(t.Context(), state, restored, merged, delayed.ReconciledAt)
	require.NoError(t, err)
	require.Empty(t, updated.HoldingClearance.Hold)
	require.Equal(t, current.HoldingClearance.StripRevision, updated.HoldingClearance.StripRevision)
}

func TestDelayedStripSnapshotAppliesHoldingAfterNewerPositionReceipt(t *testing.T) {
	for _, revision := range []int32{19, 20} {
		t.Run(fmt.Sprintf("strip revision %d", revision), func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			service.observed = make(map[string]map[aman.Callsign]aman.FlightObservation)
			previous := *state.Flights[0].LatestObservation
			previous.Provider = aman.ObservationProviderEuroScope
			previous.ClearedAltitudeStripRevision = aman.StripRevision{ID: 42, Version: 19}
			positionAt := now.Add(time.Second)
			previous.ReconciledAt = positionAt
			previous.FlightPlan.ObservedAt = &positionAt
			previous.HoldingClearance = nil
			state.Flights[0].LatestObservation = &previous
			state.Flights[0].HoldingClearance = nil
			incoming := previous
			incoming.ClearedAltitudeStripRevision.Version = revision
			incoming.ReconciledAt = now
			incoming.FlightPlan.ObservedAt = &now
			incoming.Surveillance = nil
			incoming.HoldingClearance = normalizedStripHoldingClearance(&models.Strip{
				ID: 42, Version: revision, Hold: "TESPI", HoldType: "enroute",
			}, now)
			require.NoError(t, service.Observe(t.Context(), previous))
			require.NoError(t, service.Observe(t.Context(), incoming))
			captured := service.observed[state.Airport][previous.Callsign]
			require.Equal(t, incoming.HoldingClearance, captured.HoldingClearance)
			require.Equal(t, previous.Surveillance, captured.Surveillance)
			updated, err := service.reconcileFlight(t.Context(), state, state.Flights[0], incoming, positionAt)
			require.NoError(t, err)
			require.Equal(t, incoming.HoldingClearance, updated.HoldingClearance)
			require.Equal(t, previous.Surveillance, updated.LatestObservation.Surveillance)
			legacy := incoming
			legacy.ClearedAltitudeStripRevision = aman.StripRevision{}
			require.True(t, observationPrecedes(legacy, previous))
		})
	}
}

func TestDelayedFullSyncHoldingUsesNewerPositionCFL(t *testing.T) {
	for _, scenario := range []struct {
		name        string
		positionID  int32
		positionCFL int
		wantVersion int32
	}{
		{name: "newer position disagrees", positionID: 42, positionCFL: 7000, wantVersion: 17},
		{name: "newer position confirms level", positionID: 42, positionCFL: 9000, wantVersion: 18},
		{name: "position belongs to replacement strip", positionID: 43, positionCFL: 9000, wantVersion: 17},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			flight := &state.Flights[0]
			retained := int32(11000)
			strip := &models.Strip{
				ID: 42, Version: 17, Callsign: flight.Callsign, Origin: "ESSA", Destination: state.Airport,
				Hold: "TESPI", HoldType: "enroute", ClearedAltitude: &retained,
			}
			flight.HoldingClearance = normalizedStripHoldingClearance(strip, now)
			positionAt := now.Add(time.Second)
			flight.LatestObservation.Provider = aman.ObservationProviderEuroScope
			flight.LatestObservation.FlightPlan.ObservedAt = &positionAt
			flight.LatestObservation.ClearedAltitude = &scenario.positionCFL
			flight.LatestObservation.ClearedAltitudeStripRevision = aman.StripRevision{ID: scenario.positionID, Version: 19}

			syncCFL := int32(9000)
			strip.Version, strip.ClearedAltitude = 18, &syncCFL
			sink := &euroScopeObservationSink{}
			observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink})
			require.NoError(t, err)
			state.GeneratedAt = positionAt.Add(time.Second)
			require.NoError(t, observer.ObserveEuroScopeStrips(t.Context(), 0, []shared.EuroScopeStripObservation{
				{Strip: strip, ObservedAt: state.GeneratedAt},
			}))
			require.Len(t, sink.observations, 1)
			updated, err := service.reconcileFlight(t.Context(), state, *flight, sink.observations[0], state.GeneratedAt)
			require.NoError(t, err)
			require.Equal(t, scenario.wantVersion, updated.HoldingClearance.StripRevision.Version)
			require.Equal(t, &scenario.positionCFL, updated.LatestObservation.ClearedAltitude)
		})
	}
}

type holdingSnapshotRepository struct {
	memoryRepository
	beforeCommit func()
}

func (r *holdingSnapshotRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if beforeCommit := r.beforeCommit; beforeCommit != nil {
		r.beforeCommit = nil
		beforeCommit()
	}
	if commit.ExpectedRevision != r.state.Revision {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "airport revision changed before commit"}
	}
	return r.memoryRepository.Commit(ctx, commit)
}

func (r *holdingSnapshotRepository) LoadHoldingFactSnapshots(context.Context, string, []aman.Callsign) ([]aman.HoldingFactSnapshot, error) {
	flight := r.state.Flights[0]
	return []aman.HoldingFactSnapshot{{Callsign: flight.Callsign, Clearance: flight.HoldingClearance}}, nil
}

func TestEuroScopePositionRecoversArrivalCFLAfterMissedClearancePublication(t *testing.T) {
	service, state, now := holdingClearanceRefreshFixture(t)
	flight := &state.Flights[0]
	clearedAt := now.Add(time.Second)
	retained := int32(9000)
	flight.HoldingClearance = &aman.HoldingClearance{
		ObservedAt: clearedAt, ArrivalClearedAltitude: &retained, StripRevision: aman.StripRevision{ID: 42, Version: 8},
	}
	strip := &models.Strip{ID: 42, Callsign: flight.Callsign, Origin: flight.LatestObservation.Origin, Destination: state.Airport}
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{state.Airport}})
	require.NoError(t, err)
	health := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveAuthoritative}

	for _, test := range []struct {
		name     string
		received time.Time
		version  int32
		altitude int32
		want     int
	}{
		{name: "queued position sampled before dedicated clearance", received: now, version: 7, altitude: 11000, want: 9000},
		{name: "position reads a newer persisted clearance", received: clearedAt.Add(2 * time.Second), version: 9, altitude: 7000, want: 7000},
		{name: "position reads the persisted clearance removal", received: clearedAt.Add(4 * time.Second), version: 10},
		{name: "late report cannot undo a recovered clearance removal", received: clearedAt.Add(6 * time.Second), version: 9, altitude: 7000},
	} {
		// The steps run in order against one fixture: each report depends on the
		// clearance the previous step left, so they are not separate subtests
		// that `go test -run` could select on their own.
		strip.Version = test.version
		strip.ClearedAltitude = &test.altitude
		ctx := shared.WithReceiptTime(t.Context(), test.received)
		require.NoError(t, observer.ObserveEuroScopePosition(ctx, 0, strip, 55.14, 12.14, 10000), test.name)
		observed := sink.observations[len(sink.observations)-1]
		// A later network report must not make an old strip snapshot win.
		state.GeneratedAt = test.received.Add(2 * time.Second)
		observed = mergeSurveillanceObservation(observed, aman.FlightObservation{
			Callsign: strip.Callsign, Origin: strip.Origin, Destination: strip.Destination,
			Provider: aman.ObservationProviderVATSIM, ReconciledAt: state.GeneratedAt, SourceStatus: aman.DataFresh,
		})
		updated, err := service.reconcileFlight(ctx, state, *flight, observed, state.GeneratedAt)
		require.NoError(t, err, test.name)
		*flight = updated
		require.Equal(t, &test.received, flight.LatestObservation.FlightPlan.ObservedAt, test.name)
		require.Equal(t, &retained, flight.HoldingClearance.ArrivalClearedAltitude, "%s: the dedicated update did not reach AMAN", test.name)
		event, err := frontendEvents.NewAMANStateEvent(state, health.EffectiveMode, health)
		require.NoError(t, err, test.name)
		if test.want == 0 {
			require.Nil(t, event.Data.Flights[0].ClearedAltitude, test.name)
		} else {
			require.Equal(t, &test.want, event.Data.Flights[0].ClearedAltitude, test.name)
		}
	}
}

func TestHoldingClearanceRepairReplacesOffersBeforePublishingEAT(t *testing.T) {
	service, state, now := holdingOrderRepairFixture(aman.FreezeNone)
	state.PolicyVersion, state.Mode = "test", aman.ModeAuthoritative
	_, complete, _ := holdingClearanceRefreshFixture(t)
	for index := range state.Flights {
		flight := &state.Flights[index]
		prediction := *complete.Flights[0].Prediction
		prediction.RawTETA, prediction.OperationalTETA = flight.Prediction.RawTETA, flight.Prediction.OperationalTETA
		prediction.HoldingFixETA = flight.Prediction.HoldingFixETA
		prediction.GeneratedAt, prediction.InputObservedAt = now, now
		flight.Prediction = &prediction
		observation := *complete.Flights[0].LatestObservation
		observation.Callsign, observation.ReconciledAt, observation.Surveillance = flight.Callsign, now, nil
		flight.LatestObservation = &observation
		flight.DataStatus, flight.UpdatedAt = aman.DataFresh, now
		flight.HoldingStack.ConsecutiveObservations = 2
		if flight.FreezeReason != aman.FreezeNone {
			flight.FrozenAt, flight.FrozenOperationalTETA = &now, &flight.Prediction.OperationalTETA
		}
	}
	// The follower can move from third to second when the holding queue is
	// repaired. Its existing offer for second then becomes its assigned slot.
	follower := &state.Flights[1]
	follower.Slot.Sequence = 3
	follower.QueueOffers = []aman.QueueOffer{{
		Callsign: follower.Callsign, RunwayGroupID: follower.Slot.RunwayGroupID,
		CandidateSlot: aman.Slot{Time: now.Add(23 * time.Minute), RunwayGroupID: follower.Slot.RunwayGroupID, Sequence: 2, Revision: state.Revision, Reason: "rate_wtc"},
		QueuePosition: 1, ExpiresAt: now.Add(time.Minute), AirportRevision: state.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot,
	}}

	require.NoError(t, state.Validate(), "the original queue and offer must be valid")
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	clearances, err := holdingclearance.New(holdingclearance.Dependencies{Repository: repository, Publisher: publisher, Planner: service})
	require.NoError(t, err)
	altitude := int32(9000)
	require.NoError(t, clearances.ObserveHoldingClearance(context.Background(), aman.HoldingClearanceFact{
		Callsign: follower.Callsign, Destination: state.Airport, Hold: "ROSBI", HoldType: aman.HoldingClearanceEnroute,
		ClearedAltitude: &altitude, ObservedAt: now,
	}))
	require.Len(t, publisher.states, 1, "the repaired queue and EAT must be committed and published together")
	state = publisher.states[0]
	require.NoError(t, state.Validate())

	require.Equal(t, 2, state.Flights[1].Slot.Sequence)
	require.Equal(t, now.Add(28*time.Minute), state.Flights[1].Slot.Time)
	require.Equal(t, now.Add(11*time.Minute), state.Flights[1].Prediction.HoldingPlan.ApproachReleaseTime)
	require.Empty(t, state.Flights[1].QueueOffers, "the old second-place offer is now the assigned slot")
	model := holdingclearance.BuildReadModel(state)
	require.Len(t, model.Entries, 2)
	require.Empty(t, model.Warnings)
	for _, entry := range model.Entries {
		require.NotNil(t, entry.EAT, "queue repair must retain every holding EAT")
	}
	want, err := sequence.CalculateQueueOffers(service.sequenceInput(state), sequence.QueueOfferConfig{Validity: queueOfferValidity}, now)
	require.NoError(t, err)
	require.Empty(t, want)
}

func TestHoldingClearancePublishesEATWithoutWaitingForReconciliation(t *testing.T) {
	service, state, now := holdingClearanceRefreshFixture(t)
	flight := state.Flights[0]
	slot := *flight.Slot
	positionAt := *flight.LatestObservation.Surveillance.ObservedAt
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	clearances, err := holdingclearance.New(holdingclearance.Dependencies{
		Repository: repository, Publisher: publisher, Planner: service,
	})
	require.NoError(t, err)
	cfl := int32(10000)
	require.NoError(t, clearances.ObserveHoldingClearance(context.Background(), aman.HoldingClearanceFact{
		Callsign: flight.Callsign, Destination: state.Airport, Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute,
		ClearedAltitude: &cfl, ObservedAt: now,
	}))
	require.Len(t, publisher.states, 1, "the clearance and its EAT must be published together")
	model := holdingclearance.BuildReadModel(publisher.states[0])
	require.Len(t, model.Entries, 1)
	require.NotNil(t, model.Entries[0].EAT, "the first clearance publication must already contain the EAT: warnings=%+v", model.Warnings)
	require.Empty(t, model.Warnings)
	updated := repository.state.Flights[0]
	require.NoError(t, updated.Prediction.HoldingPlan.Validate())
	require.Equal(t, slot.Time, updated.Slot.Time)
	require.Equal(t, state.Revision+1, repository.state.Revision)
	require.Equal(t, positionAt, *updated.LatestObservation.Surveillance.ObservedAt, "replanning must not invent a newer position")
}

func TestHoldingClearanceSurvivesUnavailableImmediateCalculation(t *testing.T) {
	for _, scenario := range []string{"stale position", "missing observation", "navigation unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			switch scenario {
			case "stale position":
				stale := now.Add(-3 * time.Minute)
				state.Flights[0].LatestObservation.Surveillance.ObservedAt = &stale
			case "missing observation":
				state.Flights[0].LatestObservation = nil
			case "navigation unavailable":
				service.deps.Geometry = unavailableHoldingGeometry{service.deps.Geometry}
			}
			repository := &memoryRepository{state: state, has: true}
			publisher := &recordingPublisher{}
			clearances, err := holdingclearance.New(holdingclearance.Dependencies{
				Repository: repository, Publisher: publisher, Planner: service,
			})
			require.NoError(t, err)
			require.NoError(t, clearances.ObserveHoldingClearance(context.Background(), aman.HoldingClearanceFact{
				Callsign: state.Flights[0].Callsign, Destination: state.Airport, Hold: "TESPI",
				HoldType: aman.HoldingClearanceEnroute, ObservedAt: now,
			}))
			require.Len(t, publisher.states, 1)
			require.Equal(t, "TESPI", repository.state.Flights[0].HoldingClearance.Hold)
			require.Nil(t, repository.state.Flights[0].Prediction.HoldingPlan, "unavailable inputs must not invent an EAT")
			require.Equal(t, state.Flights[0].Slot.Time, repository.state.Flights[0].Slot.Time)
		})
	}
}

type unavailableHoldingGeometry struct{ GeometryCache }

func (g unavailableHoldingGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return navdata.ActiveGeometrySnapshot{}, errors.New("navigation unavailable")
}

func holdingClearanceRefreshFixture(t *testing.T) (*Service, aman.AirportState, time.Time) {
	t.Helper()
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	hold := navdata.HoldingID("TESPI-HOLD")
	geometry.path.HoldingIDs = []navdata.HoldingID{hold}
	service.deps.Geometry = holdingReleaseGeometry{terminalIdentityGeometry: geometry,
		holding: navdata.HoldingPattern{ID: hold, Fix: "TESPI"}}
	service.deps.Terminal.Paths[0].SelectedHolding = hold
	service.deps.Now = func() time.Time { return now }

	flight := state.Flights[0]
	flight.Slot.Time = now.Add(30 * time.Minute)
	observation := *flight.LatestObservation
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55.14, 12.14
	positionAt := now
	position.ObservedAt = &positionAt
	observation.Surveillance, observation.ReconciledAt = &position, now
	var err error
	flight, err = service.reconcileFlight(context.Background(), state, flight, observation, now)
	require.NoError(t, err)
	require.Nil(t, flight.Prediction.HoldingFixETA, "the uncleared route has already passed the holding fix")
	require.Nil(t, flight.Prediction.HoldingPlan)
	now = now.Add(time.Second)
	state.Flights = []aman.AMANFlight{flight}
	return service, state, now
}
