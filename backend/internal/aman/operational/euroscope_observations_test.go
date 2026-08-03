package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
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

type euroScopeObservationSink struct{ observations []aman.FlightObservation }

func (s *euroScopeObservationSink) Observe(_ context.Context, observation aman.FlightObservation) error {
	s.observations = append(s.observations, observation)
	return nil
}

type euroScopeIdentityBinder struct{}

func (euroScopeIdentityBinder) BindVATSIMFlight(_ context.Context, identity aman.VATSIMFlightIdentity) (aman.FlightID, error) {
	return aman.FlightID("aman-" + identity.VATSIMCID), nil
}
