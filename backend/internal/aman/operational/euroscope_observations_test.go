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
		Sink: sink, Identities: euroScopeIdentityBinder{}, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	cid, aircraft, route := "123456", "A320", "MONAK OLPIB"
	strip := &models.Strip{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", VatsimCID: &cid, AircraftType: &aircraft, Route: &route}

	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.0, 12.0, 9000))
	require.Empty(t, sink.observations, "the first report establishes the speed/track reference")
	now = now.Add(30 * time.Second)
	require.NoError(t, observer.ObserveEuroScopePosition(context.Background(), 1, strip, 55.01, 12.0, 8800))
	require.Len(t, sink.observations, 1)
	observation := sink.observations[0]
	require.Equal(t, aman.SurveillanceSourceEuroScope, observation.SurveillanceSource)
	require.Equal(t, aman.FlightID("aman-123456"), observation.FlightID)
	require.Equal(t, 8800, *observation.Surveillance.AltitudeFeet)
	require.InDelta(t, 72, *observation.Surveillance.GroundspeedKnots, 1)
	require.InDelta(t, 0, *observation.Surveillance.TrackTrueDegrees, .1)
}

func TestEuroScopeSurveillanceOverlaysVATSIMUntilItExpires(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 9000, 250.0
	previous := aman.FlightObservation{
		FlightID: "F", VATSIMCID: "123", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", FiledRoute: stringPointer("OLD ROUTE"),
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

func TestServiceWaitsForVATSIMBeforeAdmittingEuroScopeOverlay(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 9000, 250.0
	euroScope := aman.FlightObservation{
		FlightID: "F", VATSIMCID: "123", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		Surveillance:       &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &now},
		SurveillanceSource: aman.SurveillanceSourceEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	service := &Service{observed: map[string]map[aman.FlightID]aman.FlightObservation{}}

	require.NoError(t, service.Observe(context.Background(), euroScope))
	require.Empty(t, service.observations("EKCH"))

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
	merged := service.observations("EKCH")["F"]
	require.Equal(t, aman.SurveillanceSourceEuroScope, merged.SurveillanceSource)
	require.NotNil(t, merged.PlannedTiming)
	require.Equal(t, eet, *merged.PlannedTiming.EstimatedEnrouteTime)
}

type euroScopeObservationSink struct{ observations []aman.FlightObservation }

func (s *euroScopeObservationSink) Observe(_ context.Context, observation aman.FlightObservation) error {
	s.observations = append(s.observations, observation)
	return nil
}

type euroScopeIdentityBinder struct{}

func (euroScopeIdentityBinder) BindVATSIMFlight(_ context.Context, identity aman.VATSIMFlightIdentity) (aman.FlightID, error) {
	return aman.FlightID("aman-" + identity.VATSIMCID), nil
}

func TestEuroScopePositionUsesReceiptTimeDespiteQueueDelay(t *testing.T) {
	received := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	sink := &euroScopeObservationSink{}
	observer, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, Identities: euroScopeIdentityBinder{}, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return received.Add(time.Minute) }})
	require.NoError(t, err)
	cid, route := "123456", "MONAK OLPIB"
	strip := &models.Strip{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", VatsimCID: &cid, Route: &route}
	require.NoError(t, observer.ObserveEuroScopePosition(shared.WithReceiptTime(context.Background(), received), 1, strip, 55, 12, 9000))
	require.NoError(t, observer.ObserveEuroScopePosition(shared.WithReceiptTime(context.Background(), received.Add(30*time.Second)), 1, strip, 55.01, 12, 8800))
	require.Len(t, sink.observations, 1)
	require.InDelta(t, 72, *sink.observations[0].Surveillance.GroundspeedKnots, 1)
	require.Equal(t, received.Add(30*time.Second), *sink.observations[0].Surveillance.ObservedAt)
}
