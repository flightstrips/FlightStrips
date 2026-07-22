package frontend

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"FlightStrips/internal/aman"

	"github.com/stretchr/testify/require"
)

func TestAMANStateEventMatchesSharedV1Golden(t *testing.T) {
	event, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, goldenAMANHealth())
	require.NoError(t, err)

	encoded, err := event.Marshal()
	require.NoError(t, err)
	expected, err := os.ReadFile("testdata/aman-state-v1.json")
	require.NoError(t, err)

	var actualJSON, expectedJSON any
	require.NoError(t, json.Unmarshal(encoded, &actualJSON))
	require.NoError(t, json.Unmarshal(expected, &expectedJSON))
	require.Equal(t, expectedJSON, actualJSON)
}

func TestAMANStateEventRejectsEffectiveModeHealthFromAnotherState(t *testing.T) {
	health := goldenAMANHealth()
	health.EffectiveMode = aman.EffectiveBlocked

	_, err := NewAMANStateEvent(goldenAMANState(), aman.EffectiveAuthoritative, health)
	require.EqualError(t, err, "map AMAN state event: effective mode and health must belong to the same state")
}

func TestAMANStateEventNormalizesDisabledComponentHealth(t *testing.T) {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	state := aman.AirportState{
		Airport: "EKCH", GeneratedAt: now, PolicyVersion: "ekch-aman-v1", Mode: aman.ModeDisabled,
		Flights: []aman.AMANFlight{}, RunwayGroups: []aman.RunwayGroupPolicy{},
	}
	health := aman.EvaluateTechnicalHealth(aman.ModeDisabled, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{})

	event, err := NewAMANStateEvent(state, aman.EffectiveDisabled, health)
	require.NoError(t, err)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.VATSIM.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Navigation.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Weather.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Repository.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.Predictor.Status)
	require.Equal(t, "disabled", event.Data.TechnicalHealth.ReplayValidation.Status)
}

func TestAMANCommandRejectionCarriesStableCorrelation(t *testing.T) {
	event, err := NewAMANCommandRejectedEvent("command-7", 9, &aman.DomainError{
		Class: aman.ErrorRevisionConflict, Message: "revision changed",
	}, true)
	require.NoError(t, err)
	require.Equal(t, AMANCommandRejectedEvent{Version: 1, Data: AMANCommandRejection{
		CommandID: "command-7", Code: "revision_conflict", Message: "revision changed", CurrentRevision: 9, Retryable: true,
	}}, event)
}

func goldenAMANState() aman.AirportState {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	runwayGroup := aman.RunwayGroupID("ARRIVAL-22")
	feeder, holding := "SOK", "SOK-HF"
	dtg := 87.5
	return aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: "ekch-aman-v1",
		Mode: aman.ModeAuthoritative, Authoritative: true,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: runwayGroup}},
		Flights: []aman.AMANFlight{{
			ID: "flight-123", VATSIMCID: "1234567", CurrentCallsign: "SAS123",
			State: aman.StateStable, DataStatus: aman.DataFresh, SelectedRunwayGroup: &runwayGroup,
			SelectedFeeder: &feeder, SelectedHolding: &holding,
			ActiveRouteFact: &aman.RouteFact{ID: "route-fact-1", Fix: "SOK", ObservedAt: now.Add(-2 * time.Minute), State: aman.RouteFactActive},
			Prediction: &aman.Prediction{
				RawTETA: now.Add(20 * time.Minute), OperationalTETA: now.Add(19 * time.Minute), OperationalReason: aman.OperationalReasonSmoothed,
				GeneratedAt: now, InputObservedAt: now.Add(-time.Minute), Confidence: aman.ConfidenceHigh, Publishable: true,
				DatasetVersion: "2607", GeometryDigest: "geometry-sha256", DistanceToGoNM: &dtg,
				HoldingFixETA: timePointer(now.Add(10 * time.Minute)), ModelVersion: "performance-wind-v1", ConfigVersion: "ekch-v1", Sources: []string{"vatsim", "airacnet"},
			},
			FreezeReason: aman.FreezeNone,
			Slot:         &aman.Slot{Time: now.Add(18 * time.Minute), RunwayGroupID: runwayGroup, Sequence: 3, Revision: 7, Reason: "rate_wtc"},
			Order:        intPointer(3), QueueOffers: []aman.QueueOffer{}, UpdatedAt: now,
		}},
	}
}

func goldenAMANHealth() aman.TechnicalHealth {
	now := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	age := float64(0)
	ready := aman.ComponentHealth{Status: aman.HealthReady, UpdatedAt: &now, AgeSeconds: &age}
	return aman.TechnicalHealth{
		Enabled: true, Mode: aman.ModeAuthoritative, DesiredMode: aman.ModeAuthoritative,
		EffectiveMode: aman.EffectiveAuthoritative, AuthorityAllowed: true, Ready: true, Status: aman.HealthReady,
		BlockedReasons: []string{}, VATSIM: ready, Navigation: ready, Weather: ready,
		Repository: ready, Predictor: ready, ReplayValidation: ready,
	}
}

func timePointer(value time.Time) *time.Time { return &value }
func intPointer(value int) *int              { return &value }
