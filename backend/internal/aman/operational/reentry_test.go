package operational

import (
	"FlightStrips/internal/aman"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestAutomaticDisappearanceAcceptsNewArrivalButNotOldSamplesOrManualRemoval(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name    string
		reason  aman.LifecycleReason
		sample  time.Time
		missing bool
		reenter bool
	}{
		{"new surveillance", aman.LifecycleReasonSourceDisappearance, now, false, true},
		{"old surveillance", aman.LifecycleReasonSourceDisappearance, now.Add(-2 * time.Minute), false, false},
		{"missing source", aman.LifecycleReasonSourceDisappearance, now, true, false},
		{"manual removal", aman.LifecycleReasonManualRemoval, now, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			removed := aman.AMANFlight{Callsign: "SAS123", State: aman.StateRemoved, UpdatedAt: now.Add(-time.Minute), Lifecycle: &aman.LifecycleState{EnteredAt: now.Add(-time.Minute), Reason: tc.reason}}
			altitude, speed := 6000, 0.0
			observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ReconciledAt: now, SourceStatus: aman.DataFresh, Missing: tc.missing,
				Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 1, LongitudeDegrees: 1, AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &tc.sample}}
			updated, err := testTMAService(t).reconcileFlight(context.Background(), aman.AirportState{}, removed, observation, now)
			require.NoError(t, err)
			if !tc.reenter {
				require.Equal(t, removed, updated)
				return
			}
			require.NotEqual(t, aman.StateRemoved, updated.State)
			require.Equal(t, now, *updated.LatestObservation.Surveillance.ObservedAt)
			require.NotNil(t, updated.TMAEntry, "TMA containment does not require speed or a prediction")
			require.Equal(t, aman.TMAInside, updated.TMAEntry.LastContainment)
			require.Nil(t, updated.Prediction, "zero groundspeed does not invent flight times")
		})
	}
}
