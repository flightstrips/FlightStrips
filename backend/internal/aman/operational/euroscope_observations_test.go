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

func TestEuroScopePositionObserverPublishesDerivedSurveillance(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{
		Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	aircraft, route := "A320", "MONAK OLPIB"
	strip := &models.Strip{ID: 42, Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AircraftType: &aircraft, Route: &route, Star: stringPointer("TESPI1A")}

	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.0, 12.0, 9000))
	require.Len(t, sink.observations, 1, "the first report establishes the flight and speed/track reference")
	require.Equal(t, "TESPI1A", *sink.observations[0].AssignedSTAR)
	require.Nil(t, sink.observations[0].Surveillance.GroundspeedKnots)
	now = now.Add(30 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.01, 12.0, 8800))
	require.Len(t, sink.observations, 2)
	observation := sink.observations[1]
	require.Equal(t, aman.SurveillanceSourceEuroScope, observation.SurveillanceSource)
	require.Equal(t, "SAS123", observation.Callsign)
	require.NotNil(t, observation.TakeoffDetected)
	require.Equal(t, 8800, *observation.Surveillance.AltitudeFeet)
	require.InDelta(t, 72, *observation.Surveillance.GroundspeedKnots, 1)
	require.InDelta(t, 0, *observation.Surveillance.TrackTrueDegrees, .1)
}

func TestEuroScopeSurveillanceOverlaysVATSIMUntilItExpires(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 9000, 250.0
	previous := aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", FiledRoute: stringPointer("OLD ROUTE"),
		Surveillance:       &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12.0, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &now},
		SurveillanceSource: aman.SurveillanceSourceEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	newRoute := "NEW ROUTE"
	incoming := previous
	incoming.FiledRoute, incoming.SurveillanceSource, incoming.ReconciledAt = &newRoute, aman.SurveillanceSourceVATSIM, now.Add(20*time.Second)
	incoming.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: 55.6, LongitudeDegrees: 12.1, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &incoming.ReconciledAt}

	merged := mergeSurveillanceObservation(previous, incoming)
	require.Equal(t, aman.SurveillanceSourceEuroScope, merged.SurveillanceSource)
	require.Equal(t, 55.5, merged.Surveillance.LatitudeDegrees)
	require.Equal(t, "OLD ROUTE", *merged.FiledRoute, "EuroScope remains authoritative for the filed route")

	incoming.ReconciledAt = now.Add(euroScopeSurveillanceFresh + time.Second)
	*incoming.Surveillance.ObservedAt = incoming.ReconciledAt
	fallback := mergeSurveillanceObservation(previous, incoming)
	require.Equal(t, aman.SurveillanceSourceVATSIM, fallback.SurveillanceSource)
	require.Equal(t, 55.6, fallback.Surveillance.LatitudeDegrees)
	require.Equal(t, "OLD ROUTE", *fallback.FiledRoute, "surveillance fallback cannot replace EuroScope flight-plan facts")
}

func TestServiceAdmitsEuroScopeWithoutVATSIMAndStillMergesLiveMetadata(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 9000, 250.0
	euroScope := aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		Surveillance:       &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &now},
		SurveillanceSource: aman.SurveillanceSourceEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	service := &Service{observed: map[string]map[aman.Callsign]aman.FlightObservation{}}

	require.NoError(t, service.Observe(context.Background(), euroScope))
	require.Len(t, service.observations("EKCH"), 1)

	eet := time.Hour
	vatsim := euroScope
	vatsim.SurveillanceSource = aman.SurveillanceSourceVATSIM
	vatsim.PlannedTiming = &aman.PlannedTiming{EstimatedEnrouteTime: &eet}
	require.NoError(t, service.Observe(context.Background(), vatsim))
	require.Len(t, service.observations("EKCH"), 1)

	now = now.Add(10 * time.Second)
	euroScope.ReconciledAt = now
	euroScope.Surveillance.ObservedAt = &now
	require.NoError(t, service.Observe(context.Background(), euroScope))
	merged := service.observations("EKCH")["SAS123"]
	require.Equal(t, aman.SurveillanceSourceEuroScope, merged.SurveillanceSource)
	require.NotNil(t, merged.PlannedTiming)
	require.Equal(t, eet, *merged.PlannedTiming.EstimatedEnrouteTime)
}

func TestServiceRetractionRemovesOnlyOwningProvider(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	service := &Service{observed: map[string]map[aman.Callsign]aman.FlightObservation{}}
	vatsim := aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderVATSIM,
		SurveillanceSource: aman.SurveillanceSourceVATSIM, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	euroScope := vatsim
	euroScope.Provider, euroScope.SurveillanceSource = aman.ObservationProviderEuroScope, ""
	require.NoError(t, service.Observe(context.Background(), vatsim))
	require.NoError(t, service.Observe(context.Background(), euroScope))

	euroScope.Missing = true
	euroScope.ReconciledAt = now.Add(time.Second)
	require.NoError(t, service.Observe(context.Background(), euroScope))
	remaining := service.observations("EKCH")["SAS123"]
	require.False(t, remaining.Missing)
	require.Equal(t, aman.ObservationProviderVATSIM, remaining.Provider)

	vatsim.Missing = true
	vatsim.ReconciledAt = now.Add(2 * time.Second)
	require.NoError(t, service.Observe(context.Background(), vatsim))
	require.True(t, service.observations("EKCH")["SAS123"].Missing)
}

type euroScopeObservationSink struct{ observations []aman.FlightObservation }

func TestEuroScopeReportedMotionSurvivesDispatchDelayAndDuplicates(t *testing.T) {
	received := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return received.Add(time.Minute) }})
	require.NoError(t, err)
	strip := &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}
	gs, age := 420.0, int64(5000)
	ctx := shared.WithPositionMotion(shared.WithReceiptTime(t.Context(), received), shared.PositionMotion{GroundspeedKnots: &gs, PositionAgeMS: &age})
	require.NoError(t, observer.ObserveEuroScopePosition(ctx, 1, strip, 55, 12, 30000))
	first := sink.observations[0].Surveillance
	require.Equal(t, gs, *first.GroundspeedKnots)
	require.Equal(t, "reported", first.GroundspeedSource)
	require.Equal(t, received.Add(-5*time.Second), *first.ObservedAt)
	require.Nil(t, first.TrackTrueDegrees, "reported heading must never become ground track")
	ctx = shared.WithPositionMotion(shared.WithReceiptTime(t.Context(), received.Add(10*time.Second)), shared.PositionMotion{GroundspeedKnots: &gs, PositionAgeMS: &age})
	require.NoError(t, observer.ObserveEuroScopePosition(ctx, 1, strip, 55, 12, 30000))
	require.Equal(t, *first.ObservedAt, *sink.observations[1].Surveillance.ObservedAt)
	// A second fresh report uses native speed even though the derived-speed
	// window has not accumulated thirty seconds yet.
	require.NoError(t, observer.ObserveEuroScopePosition(ctx, 1, strip, 55.01, 12, 30000))
	require.Equal(t, gs, *sink.observations[2].Surveillance.GroundspeedKnots)
}

func TestEuroScopeFallbackNeedsThirtySecondMotionBaseline(t *testing.T) {
	now := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := &models.Strip{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH"}
	for i := 0; i <= 6; i++ {
		if i > 0 {
			now = now.Add(5 * time.Second)
		}
		require.NoError(t, observer.ObserveEuroScopePosition(t.Context(), 1, strip, 55+float64(i)*0.01, 12, 10000))
		if i < 6 {
			require.Nil(t, sink.observations[i].Surveillance.GroundspeedKnots)
		}
	}
	require.InDelta(t, 432, *sink.observations[6].Surveillance.GroundspeedKnots, 2)
	invalid := 900.0
	now = now.Add(30 * time.Second)
	ctx := shared.WithPositionMotion(t.Context(), shared.PositionMotion{GroundspeedKnots: &invalid})
	require.NoError(t, observer.ObserveEuroScopePosition(ctx, 1, strip, 55.12, 12, 10000))
	require.Equal(t, "derived", sink.observations[7].Surveillance.GroundspeedSource)
}

func TestRepeatedEuroScopePositionsPreserveFreshMotionAndNextSpeedBaseline(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := &models.Strip{Session: 1, Callsign: "SAS71G", Origin: "ESSA", Destination: "EKCH"}
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55, 12, 9000))
	now = now.Add(30 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.01, 12, 9000))
	now = now.Add(20 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.01, 12, 9000))
	repeated := sink.observations[2]
	require.NotNil(t, repeated.Surveillance.GroundspeedKnots, "a duplicate must not remove speed and drop the AMAN slot")
	require.InDelta(t, 72, *repeated.Surveillance.GroundspeedKnots, 1)
	require.Equal(t, now.Add(-20*time.Second), *repeated.Surveillance.ObservedAt, "duplicates must not renew surveillance freshness")
	now = now.Add(10 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.02, 12, 9000))
	require.InDelta(t, 72, *sink.observations[3].Surveillance.GroundspeedKnots, 1, "duplicate timing must not inflate the next speed to 216 knots")

	// Repeated reports cannot keep an old motion estimate fresh indefinitely.
	now = now.Add(euroScopeSurveillanceFresh + time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.02, 12, 9000))
	require.Nil(t, sink.observations[4].Surveillance.GroundspeedKnots)
}

func TestEuroScopeDelayedPositionDoesNotReplaceMotionBaseline(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
	require.NoError(t, err)
	strip := &models.Strip{Session: 1, Callsign: "SAS71G", Origin: "ESSA", Destination: "EKCH"}
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55, 12, 9000))
	now = now.Add(30 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.01, 12, 9000))
	now = now.Add(-10 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 54, 12, 9000))
	require.Len(t, sink.observations, 2)
	now = now.Add(40 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.02, 12, 9000))
	require.InDelta(t, 72, *sink.observations[2].Surveillance.GroundspeedKnots, 1)
}

func (s *euroScopeObservationSink) Observe(_ context.Context, observation aman.FlightObservation) error {
	s.observations = append(s.observations, observation)
	return nil
}

func TestEuroScopePositionUsesReceiptTimeDespiteQueueDelay(t *testing.T) {
	received := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return received.Add(time.Minute) }})
	require.NoError(t, err)
	route := "MONAK OLPIB"
	strip := &models.Strip{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Route: &route}
	require.NoError(t, observer.ObserveEuroScopePosition(shared.WithReceiptTime(context.Background(), received), 1, strip, 55, 12, 9000))
	require.NoError(t, observer.ObserveEuroScopePosition(shared.WithReceiptTime(context.Background(), received.Add(30*time.Second)), 1, strip, 55.01, 12, 8800))
	require.Len(t, sink.observations, 2)
	require.InDelta(t, 72, *sink.observations[1].Surveillance.GroundspeedKnots, 1)
	require.Equal(t, received.Add(30*time.Second), *sink.observations[1].Surveillance.ObservedAt)
}
