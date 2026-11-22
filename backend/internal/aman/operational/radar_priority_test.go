package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestLiveSessionPrefersEuroScopeRadarOverNewerNetworkSample(t *testing.T) {
	for _, networkFirst := range []bool{false, true} {
		name := "EuroScope received first"
		if networkFirst {
			name = "network received first"
		}
		t.Run(name, func(t *testing.T) {
			at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
			networkAt := at.Add(20 * time.Second)
			altitude, radarSpeed, networkSpeed := 9000, 250.0, 300.0
			local := aman.FlightObservation{
				Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
				Provider: aman.ObservationProviderEuroScope, SurveillanceSource: aman.SurveillanceSourceEuroScope,
				ReconciledAt: at, SourceStatus: aman.DataFresh,
				Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12,
					AltitudeFeet: &altitude, GroundspeedKnots: &radarSpeed, ObservedAt: &at},
			}
			network := local
			network.Provider, network.SurveillanceSource = aman.ObservationProviderVATSIM, aman.SurveillanceSourceVATSIM
			network.ReconciledAt = networkAt
			network.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: 55.6, LongitudeDegrees: 12.1,
				AltitudeFeet: &altitude, GroundspeedKnots: &networkSpeed, ObservedAt: &networkAt}
			service := &Service{
				observed:     make(map[string]map[aman.Callsign]aman.FlightObservation),
				liveSessions: map[int32]bool{1: true},
			}
			ctx := aman.WithSession(t.Context(), 1)
			if networkFirst {
				require.NoError(t, service.Observe(t.Context(), network))
			}
			require.NoError(t, service.Observe(ctx, local))
			if !networkFirst {
				require.NoError(t, service.Observe(t.Context(), network))
			}

			merged := service.sessionObservations(ctx, "EKCH")[local.Callsign]
			require.Equal(t, aman.SurveillanceSourceEuroScope, merged.SurveillanceSource)
			require.Equal(t, local.Surveillance, merged.Surveillance)
		})
	}
}

func TestDelayedEuroScopeSnapshotRetainsNewerRadar(t *testing.T) {
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	delayedAt := at.Add(-time.Second)
	altitude, speed := 9000, 250.0
	current := aman.FlightObservation{
		Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		Provider: aman.ObservationProviderEuroScope, SurveillanceSource: aman.SurveillanceSourceEuroScope,
		ClearedAltitudeStripRevision: aman.StripRevision{ID: 42, Version: 19},
		ReconciledAt:                 at, SourceStatus: aman.DataFresh,
		Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.5, LongitudeDegrees: 12,
			AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &at},
	}
	delayed := current
	delayed.ReconciledAt = delayedAt
	delayed.ClearedAltitudeStripRevision.Version++
	delayed.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: 55.4, LongitudeDegrees: 11.9,
		AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &delayedAt}
	service := &Service{observed: make(map[string]map[aman.Callsign]aman.FlightObservation)}
	ctx := aman.WithSession(t.Context(), 1)
	require.NoError(t, service.Observe(ctx, current))
	require.NoError(t, service.Observe(ctx, delayed))

	merged := service.sessionObservations(ctx, "EKCH")[current.Callsign]
	require.Equal(t, delayed.ClearedAltitudeStripRevision, merged.ClearedAltitudeStripRevision)
	require.Equal(t, aman.SurveillanceSourceEuroScope, merged.SurveillanceSource)
	require.Equal(t, current.Surveillance, merged.Surveillance)
}
