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
	strip := &models.Strip{ID: 42, Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AircraftType: &aircraft, Route: &route}

	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.0, 12.0, 9000))
	require.Len(t, sink.observations, 1, "the first report establishes the flight and speed/track reference")
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
	require.Equal(t, newRoute, *merged.FiledRoute, "the VATSIM flight plan remains current")

	incoming.ReconciledAt = now.Add(euroScopeSurveillanceFresh + time.Second)
	*incoming.Surveillance.ObservedAt = incoming.ReconciledAt
	fallback := mergeSurveillanceObservation(previous, incoming)
	require.Equal(t, aman.SurveillanceSourceVATSIM, fallback.SurveillanceSource)
	require.Equal(t, 55.6, fallback.Surveillance.LatitudeDegrees)
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
