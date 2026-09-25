package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEuroScopeStripObserverCreatesFlightWithoutCID(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{
		Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	route := "MONAK OLPIB"
	strip := &models.Strip{ID: 77, Session: 12, Callsign: "sas123", Origin: "essa", Destination: "ekch", Route: &route, Hold: "olpib", HoldType: "enroute", HoldEat: "1015"}

	require.NoError(t, observer.ObserveEuroScopeStrip(context.Background(), strip))
	require.Len(t, sink.observations, 1)
	observation := sink.observations[0]
	require.Equal(t, "SAS123", observation.Callsign)
	require.Equal(t, "ESSA", observation.Origin)
	require.Equal(t, "EKCH", observation.Destination)
	require.Equal(t, "OLPIB", observation.HoldingClearance.Hold)
	require.Nil(t, observation.Surveillance)
	require.Empty(t, observation.SurveillanceSource, "strip facts are not surveillance")
	require.Equal(t, aman.ObservationProviderEuroScope, observation.Provider)
}

func TestEuroScopeStripObserverRetractionReachesOperationalService(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	service := &Service{
		observed: map[string]map[aman.Callsign]aman.FlightObservation{},
		health:   serviceHealth{euroScope: aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "source_not_observed"}},
	}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{
		Sink: service, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	strip := &models.Strip{Session: 12, Callsign: "OLD123", Origin: "ESSA", Destination: "EKCH"}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 12, []shared.EuroScopeStripObservation{{Strip: strip, ObservedAt: now}}))
	require.False(t, service.observations("EKCH")["OLD123"].Missing)
	require.Equal(t, aman.HealthUnavailable, service.health.euroScope.Status, "strip facts alone are not surveillance readiness")

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 12, nil))
	require.True(t, service.observations("EKCH")["OLD123"].Missing)
}

func TestEuroScopeStripObserverReplacementRetractsRemovedAndRenamedCallsigns(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{
		Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	oldStrip := &models.Strip{Session: 12, Callsign: "OLD123", Origin: "ESSA", Destination: "EKCH"}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 12, []shared.EuroScopeStripObservation{{Strip: oldStrip, ObservedAt: now}}))

	now = now.Add(time.Minute)
	newStrip := &models.Strip{Session: 12, Callsign: "NEW123", Origin: "ESSA", Destination: "EKCH"}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 12, []shared.EuroScopeStripObservation{{Strip: newStrip, ObservedAt: now}}))
	require.Len(t, sink.observations, 3)
	require.Equal(t, "NEW123", sink.observations[1].Callsign)
	require.Equal(t, "OLD123", sink.observations[2].Callsign)
	require.True(t, sink.observations[2].Missing)
	require.Equal(t, now, sink.observations[2].ReconciledAt)

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 12, nil))
	require.Equal(t, "NEW123", sink.observations[3].Callsign)
	require.True(t, sink.observations[3].Missing)
}

func TestEuroScopeStripObserverKeepsReplacementSetsPerSession(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := func(session int32) *models.Strip {
		return &models.Strip{Session: session, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}
	}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: strip(1), ObservedAt: now}}))
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, []shared.EuroScopeStripObservation{{Strip: strip(2), ObservedAt: now}}))

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, nil))
	require.Len(t, sink.observations, 4, "the removed owner must be retracted before the surviving session is restored")
	require.True(t, sink.observations[2].Missing)
	require.False(t, sink.observations[3].Missing)
	require.Len(t, observer.known[2], 1, "replacing one session must not retract another session")

	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, nil))
	require.Len(t, sink.observations, 5)
	require.Equal(t, aman.Callsign("SAS123"), sink.observations[4].Callsign)
	require.True(t, sink.observations[4].Missing)
}

func TestEuroScopeStripObserverRestoresSurvivingSessionFacts(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, Now: func() time.Time { return now }})
	require.NoError(t, err)
	survivingRoute, removedRoute := "MONAK OLPIB", "NEXEN TIDVU"
	surviving := &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Route: &survivingRoute, Hold: "OLPIB"}
	removed := &models.Strip{Session: 2, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Route: &removedRoute, Hold: "TIDVU"}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: surviving, ObservedAt: now}}))
	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, []shared.EuroScopeStripObservation{{Strip: removed, ObservedAt: now}}))

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, nil))
	require.Len(t, sink.observations, 4)
	require.True(t, sink.observations[2].Missing)
	require.Equal(t, survivingRoute, *sink.observations[3].FiledRoute)
	require.Equal(t, "OLPIB", sink.observations[3].HoldingClearance.Hold)
	require.False(t, sink.observations[3].Missing)
}

func TestEuroScopeStripObserverClearsRemovedSessionSurveillance(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	service := &Service{
		observed: map[string]map[aman.Callsign]aman.FlightObservation{},
		health:   serviceHealth{euroScope: aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "source_not_observed"}},
	}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: service, Now: func() time.Time { return now }})
	require.NoError(t, err)
	survivingRoute, removedRoute := "MONAK OLPIB", "NEXEN TIDVU"
	surviving := &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Route: &survivingRoute, Hold: "OLPIB"}
	removed := &models.Strip{Session: 2, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Route: &removedRoute, Hold: "TIDVU"}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: surviving, ObservedAt: now}}))
	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, []shared.EuroScopeStripObservation{{Strip: removed, ObservedAt: now}}))
	altitude := 7000
	require.NoError(t, service.Observe(context.Background(), aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderEuroScope,
		SurveillanceSource: aman.SurveillanceSourceEuroScope,
		Surveillance:       &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12.5, AltitudeFeet: &altitude, ObservedAt: &now},
		ReconciledAt:       now, SourceStatus: aman.DataFresh,
	}))
	require.NotNil(t, service.observations("EKCH")["SAS123"].Surveillance)

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, nil))
	restored := service.observations("EKCH")["SAS123"]
	require.Nil(t, restored.Surveillance, "surveillance from the removed session must not leak into the survivor")
	require.Equal(t, survivingRoute, *restored.FiledRoute)
	require.Equal(t, "OLPIB", restored.HoldingClearance.Hold)
}

func TestEuroScopeStripObserverScopesSameCallsignOwnershipByAirport(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := func(session int32, destination string) *models.Strip {
		return &models.Strip{Session: session, Callsign: "SAS123", Origin: "ESSA", Destination: destination}
	}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: strip(1, "EKCH"), ObservedAt: now}}))
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, []shared.EuroScopeStripObservation{{Strip: strip(2, "EKBI"), ObservedAt: now}}))

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, nil))
	require.Len(t, sink.observations, 3, "a same-callsign flight at another airport must not retain this airport's flight")
	require.Equal(t, "EKCH", sink.observations[2].Destination)
	require.True(t, sink.observations[2].Missing)

	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: strip(1, "EKCH"), ObservedAt: now}}))
	require.NoError(t, observer.RemoveEuroScopeStrip(context.Background(), 1, "SAS123"))
	require.Len(t, sink.observations, 5)
	require.Equal(t, "EKCH", sink.observations[4].Destination)
	require.True(t, sink.observations[4].Missing)
	require.False(t, observer.known[2]["SAS123"].Missing)
}

func TestEuroScopeStripObserverRetractsPreviousAirportOnDestinationChange(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{
		Strip: &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}, ObservedAt: now,
	}}))

	now = now.Add(time.Minute)
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{
		Strip: &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKBI"}, ObservedAt: now,
	}}))
	require.Len(t, sink.observations, 3)
	require.Equal(t, "EKBI", sink.observations[1].Destination)
	require.False(t, sink.observations[1].Missing)
	require.Equal(t, "EKCH", sink.observations[2].Destination)
	require.True(t, sink.observations[2].Missing)
}

func TestEuroScopeStripObserverRemovesIndividualDisconnectPerSession(t *testing.T) {
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := func(session int32) *models.Strip {
		return &models.Strip{Session: session, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}
	}
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 1, []shared.EuroScopeStripObservation{{Strip: strip(1), ObservedAt: now}}))
	require.NoError(t, observer.ObserveEuroScopeStrips(context.Background(), 2, []shared.EuroScopeStripObservation{{Strip: strip(2), ObservedAt: now}}))

	require.NoError(t, observer.RemoveEuroScopeStrip(context.Background(), 1, "sas123"))
	require.Len(t, sink.observations, 4, "the other session is republished as the active owner")
	require.True(t, sink.observations[2].Missing)
	require.False(t, sink.observations[3].Missing)
	now = now.Add(time.Minute)
	require.NoError(t, observer.RemoveEuroScopeStrip(context.Background(), 2, "SAS123"))
	require.Len(t, sink.observations, 5)
	require.True(t, sink.observations[4].Missing)
	require.Equal(t, now, sink.observations[4].ReconciledAt)
}
