package operational

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestResetClearsControllerSTARAndExistingTimingOverrideTogether(t *testing.T) {
	for _, scenario := range []string{"star_only", "star_and_manual_eta", "star_and_manual_review", "manual_eta_only"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
			state := runwayGapDisplacementState(now)
			flight := gapCommandFlight("TARGET", "north", now.Add(5*time.Minute), 1, aman.StateStable, aman.FreezeNone)
			flight.UpdatedAt, flight.Prediction.GeneratedAt, flight.Prediction.InputObservedAt = now, now, now
			flight.RawTETASamples = []aman.RawTETASample{{TETA: flight.Prediction.RawTETA, GeneratedAt: now, ObservedAt: now}}
			if scenario != "manual_eta_only" {
				flight.ControllerSTARFamily = stringPointer("MONAK")
			}
			if scenario == "star_and_manual_review" {
				flight.ArrivalBaseline = resetReviewBaseline(now)
				flight.ETAReview = &aman.ETAReview{Status: aman.ReviewPending, CreatedAt: now, DeadlineAt: now.Add(time.Minute), InitialBaselineTETA: now.Add(3 * time.Minute), CalculatedOperationalTETA: flight.Prediction.OperationalTETA, SelectedTETA: flight.Prediction.OperationalTETA}
			}
			state.Flights = []aman.AMANFlight{flight}
			repository := &memoryRepository{has: true, state: state}
			manualETA := now.Add(9 * time.Minute)
			if scenario != "star_only" {
				_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).SetManualETA(t.Context(), closureAuth(now.Add(time.Second)), aman.SetManualETACommand{Metadata: aman.CommandMetadata{CommandID: "manual", ExpectedRevision: repository.state.Revision}, Callsign: flight.Callsign, ManualETA: manualETA})
				require.NoError(t, err)
				require.Equal(t, aman.FreezeManual, repository.state.Flights[0].FreezeReason)
			}
			before := cloneGapState(t, repository.state)
			_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(2*time.Second)).ResetTETAOverride(t.Context(), closureAuth(now.Add(2*time.Second)), aman.ResetTETAOverrideCommand{Metadata: aman.CommandMetadata{CommandID: "reset", ExpectedRevision: repository.state.Revision}, Callsign: flight.Callsign})
			require.NoError(t, err)
			updated := repository.state.Flights[0]
			require.Nil(t, updated.ControllerSTARFamily)
			require.Nil(t, updated.ETAReview)
			require.Equal(t, aman.FreezeNone, updated.FreezeReason)
			require.Nil(t, updated.FrozenAt)
			require.Nil(t, updated.FrozenSlot)
			require.Nil(t, updated.FrozenOperationalTETA)
			require.NotEqual(t, manualETA, updated.Prediction.OperationalTETA)
			require.Equal(t, before.Flights[0].Prediction.RawTETA, updated.Prediction.RawTETA)
			require.Equal(t, before.Flights[0].RawTETASamples, updated.RawTETASamples)
			require.Equal(t, before.Revision+1, repository.state.Revision)
		})
	}
}

func TestRejectedETAResetDoesNotPartiallyClearControllerSTAR(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	flight := gapCommandFlight("TARGET", "north", now.Add(5*time.Minute), 1, aman.StateStable, aman.FreezeNone)
	flight.UpdatedAt = now
	flight.ControllerSTARFamily = stringPointer("MONAK")
	flight.ArrivalBaseline = resetReviewBaseline(now)
	flight.ETAReview = &aman.ETAReview{Status: aman.ReviewPending, CreatedAt: now, DeadlineAt: now.Add(time.Minute), InitialBaselineTETA: now.Add(3 * time.Minute), CalculatedOperationalTETA: flight.Prediction.OperationalTETA, SelectedTETA: flight.Prediction.OperationalTETA}
	state.Flights = []aman.AMANFlight{flight}
	repository := &memoryRepository{has: true, state: state}
	before := cloneGapState(t, state)
	_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).ResetTETAOverride(t.Context(), closureAuth(now.Add(time.Second)), aman.ResetTETAOverrideCommand{Metadata: aman.CommandMetadata{CommandID: "reset-pending", ExpectedRevision: state.Revision}, Callsign: flight.Callsign})
	requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
	require.Empty(t, repository.commits)
	require.Equal(t, before, repository.state)
}

func resetReviewBaseline(now time.Time) *aman.BaselineState {
	return &aman.BaselineState{ArrivalAt: now.Add(3 * time.Minute), AirborneSensedAt: now.Add(-time.Hour), Source: aman.BaselineSourceAirborneFiledEET, Confidence: aman.ConfidenceMedium, FlightPlanObservedAt: now.Add(-time.Hour), ModelVersion: "baseline-v1", ConfigVersion: "baseline-config-v1"}
}
