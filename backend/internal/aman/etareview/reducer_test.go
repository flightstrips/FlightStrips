package etareview_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/etareview"
	"FlightStrips/internal/aman/prediction"
	"github.com/stretchr/testify/require"
)

func TestOpenUsesFirstUnstableEligibilityAndExactThresholdBoundary(t *testing.T) {
	now := reviewTime()
	config := reviewConfig()

	for _, test := range []struct {
		name       string
		difference time.Duration
		wantOpen   bool
	}{
		{name: "below", difference: config.DiscrepancyThreshold - time.Second},
		{name: "equal", difference: config.DiscrepancyThreshold},
		{name: "above", difference: config.DiscrepancyThreshold + time.Second, wantOpen: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			flight := reviewFlight(now)
			flight.Prediction.OperationalTETA = flight.ArrivalBaseline.ArrivalAt.Add(test.difference)
			rawBefore := flight.Prediction.RawTETA
			result, err := etareview.Open(config, flight, now.Add(time.Minute))
			require.NoError(t, err)
			require.Equal(t, test.wantOpen, result.Changed)
			if !test.wantOpen {
				require.Nil(t, result.Flight.ETAReview)
				return
			}
			require.Equal(t, aman.ReviewPending, result.Flight.ETAReview.Status)
			require.Equal(t, now.Add(time.Minute), result.Flight.ETAReview.CreatedAt)
			require.Equal(t, now.Add(time.Minute).Add(config.ReviewDeadline), result.Flight.ETAReview.DeadlineAt)
			require.Equal(t, flight.ArrivalBaseline.ArrivalAt, result.Flight.ETAReview.InitialBaselineTETA)
			require.Equal(t, flight.Prediction.OperationalTETA, result.Flight.ETAReview.CalculatedOperationalTETA)
			require.Equal(t, rawBefore, result.Flight.Prediction.RawTETA)
			require.NoError(t, result.Flight.Validate())
		})
	}

	notFirst := reviewFlight(now)
	notFirst.Prediction.OperationalReason = aman.OperationalReasonSmoothed
	result, err := etareview.Open(config, notFirst, now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, result.Changed)

	stale := reviewFlight(now)
	stale.DataStatus = aman.DataStale
	result, err = etareview.Open(config, stale, now.Add(time.Minute))
	require.NoError(t, err)
	require.False(t, result.Changed)
}

func TestTypedResolutionsSelectExactlyOneReviewStateWithoutChangingRawTETA(t *testing.T) {
	now := reviewTime()
	pending := openReview(t, now)
	rawBefore := pending.Prediction.RawTETA
	historyBefore := append([]aman.RawTETASample(nil), pending.RawTETASamples...)
	note := "confirmed with approach"

	t.Run("accept calculated", func(t *testing.T) {
		result, err := etareview.ResolveAcceptCalculated(pending, etareview.AcceptCalculated{At: now.Add(2 * time.Minute), Actor: "1345678", Note: &note})
		require.NoError(t, err)
		require.Equal(t, aman.ReviewAcceptedCalculatedTETA, result.Flight.ETAReview.Status)
		require.Equal(t, pending.Prediction.OperationalTETA, result.Flight.ETAReview.SelectedTETA)
		require.Equal(t, aman.FreezeNone, result.Flight.FreezeReason)
		requireRawUnchanged(t, rawBefore, historyBefore, result.Flight)
		require.NoError(t, result.Flight.Validate())
	})

	t.Run("keep initial", func(t *testing.T) {
		result, err := etareview.ResolveKeepInitial(pending, etareview.KeepInitial{At: now.Add(2 * time.Minute), Actor: "1345678"})
		require.NoError(t, err)
		require.Equal(t, aman.ReviewKeptInitialFPLETA, result.Flight.ETAReview.Status)
		require.Equal(t, pending.ArrivalBaseline.ArrivalAt, result.Flight.Prediction.OperationalTETA)
		require.Equal(t, aman.FreezeManual, result.Flight.FreezeReason)
		requireRawUnchanged(t, rawBefore, historyBefore, result.Flight)
		require.NoError(t, result.Flight.Validate())
	})

	t.Run("manual", func(t *testing.T) {
		manual := now.Add(26 * time.Minute)
		result, err := etareview.ResolveSetManual(pending, etareview.SetManual{At: now.Add(2 * time.Minute), Actor: "1345678", ManualTETA: manual})
		require.NoError(t, err)
		require.Equal(t, aman.ReviewManualETA, result.Flight.ETAReview.Status)
		require.Equal(t, manual, *result.Flight.ETAReview.ManualTETA)
		require.Equal(t, manual, result.Flight.Prediction.OperationalTETA)
		require.Equal(t, aman.FreezeManual, result.Flight.FreezeReason)
		requireRawUnchanged(t, rawBefore, historyBefore, result.Flight)
		require.NoError(t, result.Flight.Validate())
	})
}

func TestDeadlineIsDeterministicAndWinsAtEquality(t *testing.T) {
	now := reviewTime()
	pending := openReview(t, now)
	deadline := pending.ETAReview.DeadlineAt

	before, err := etareview.AutoAccept(pending, deadline.Add(-time.Nanosecond))
	require.NoError(t, err)
	require.False(t, before.Changed)

	at, err := etareview.AutoAccept(pending, deadline)
	require.NoError(t, err)
	require.True(t, at.Changed)
	require.Equal(t, aman.ReviewAutoAcceptedCalculatedTETA, at.Flight.ETAReview.Status)
	require.Equal(t, deadline, *at.Flight.ETAReview.ResolvedAt)
	require.NoError(t, at.Flight.Validate())

	after, err := etareview.AutoAccept(pending, deadline.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, deadline, *after.Flight.ETAReview.ResolvedAt)

	_, err = etareview.ResolveAcceptCalculated(pending, etareview.AcceptCalculated{At: deadline, Actor: "1345678"})
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
}

func TestResetReleasesOnlyTheOperationalOverrideAndRestartPreservesReview(t *testing.T) {
	now := reviewTime()
	pending := openReview(t, now)
	manual := now.Add(26 * time.Minute)
	resolved, err := etareview.ResolveSetManual(pending, etareview.SetManual{At: now.Add(2 * time.Minute), Actor: "1345678", ManualTETA: manual})
	require.NoError(t, err)

	payload, err := json.Marshal(resolved.Flight)
	require.NoError(t, err)
	var restored aman.AMANFlight
	require.NoError(t, json.Unmarshal(payload, &restored))
	require.Equal(t, resolved.Flight, restored)
	require.NoError(t, restored.Validate())

	rawBefore := restored.Prediction.RawTETA
	historyBefore := append([]aman.RawTETASample(nil), restored.RawTETASamples...)
	reset, err := etareview.ResolveReset(prediction.DefaultConfig(), restored, etareview.Reset{At: now.Add(3 * time.Minute), Actor: "1345678"})
	require.NoError(t, err)
	require.Nil(t, reset.Flight.ETAReview)
	require.Equal(t, aman.FreezeNone, reset.Flight.FreezeReason)
	require.NotEqual(t, manual, reset.Flight.Prediction.OperationalTETA)
	requireRawUnchanged(t, rawBefore, historyBefore, reset.Flight)
	require.NoError(t, reset.Flight.Validate())

	_, err = etareview.ResolveReset(prediction.DefaultConfig(), reset.Flight, etareview.Reset{At: now.Add(4 * time.Minute), Actor: "1345678"})
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
}

func TestInvalidTransitionsAndManualValueAreTyped(t *testing.T) {
	now := reviewTime()
	pending := openReview(t, now)

	_, err := etareview.ResolveSetManual(pending, etareview.SetManual{At: now.Add(2 * time.Minute), Actor: "1345678", ManualTETA: now.Add(time.Minute)})
	requireDomainClass(t, err, aman.ErrorInvalidArgument)

	accepted, err := etareview.ResolveAcceptCalculated(pending, etareview.AcceptCalculated{At: now.Add(2 * time.Minute), Actor: "1345678"})
	require.NoError(t, err)
	_, err = etareview.ResolveKeepInitial(accepted.Flight, etareview.KeepInitial{At: now.Add(3 * time.Minute), Actor: "1345678"})
	requireDomainClass(t, err, aman.ErrorInvalidTransition)

	_, err = etareview.ResolveAcceptCalculated(pending, etareview.AcceptCalculated{At: now.Add(2 * time.Minute), Actor: ""})
	requireDomainClass(t, err, aman.ErrorInvalidArgument)
}

func openReview(t *testing.T, now time.Time) aman.AMANFlight {
	t.Helper()
	result, err := etareview.Open(reviewConfig(), reviewFlight(now), now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, result.Changed)
	return result.Flight
}

func reviewConfig() etareview.Config {
	return etareview.Config{DiscrepancyThreshold: 5 * time.Minute, ReviewDeadline: 5 * time.Minute}
}

func reviewFlight(now time.Time) aman.AMANFlight {
	baseline := now.Add(20 * time.Minute)
	return aman.AMANFlight{
		ID: "flight-1", VATSIMCID: "1234567", CurrentCallsign: "SAS123",
		State: aman.StateUnstable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, UpdatedAt: now,
		ArrivalBaseline: &aman.BaselineState{
			ArrivalAt: baseline, AirborneSensedAt: now.Add(-time.Hour), Source: aman.BaselineSourceAirborneFiledEET,
			Confidence: aman.ConfidenceMedium, FlightPlanObservedAt: now.Add(-time.Hour), ModelVersion: "baseline-v1", ConfigVersion: "baseline-config-v1",
		},
		Prediction: &aman.Prediction{
			RawTETA: now.Add(31 * time.Minute), OperationalTETA: now.Add(30 * time.Minute), OperationalReason: aman.OperationalReasonFirstUnstable,
			GeneratedAt: now, InputObservedAt: now.Add(-time.Minute), Confidence: aman.ConfidenceHigh, Publishable: true,
			DatasetVersion: "2607", GeometryDigest: "geometry", ModelVersion: "model-v1", ConfigVersion: "config-v1", Sources: []string{"vatsim"},
		},
		RawTETASamples: []aman.RawTETASample{
			{TETA: now.Add(29 * time.Minute), GeneratedAt: now.Add(-2 * time.Minute)},
			{TETA: now.Add(30 * time.Minute), GeneratedAt: now.Add(-time.Minute)},
			{TETA: now.Add(31 * time.Minute), GeneratedAt: now},
		},
	}
}

func requireRawUnchanged(t *testing.T, raw time.Time, history []aman.RawTETASample, flight aman.AMANFlight) {
	t.Helper()
	require.Equal(t, raw, flight.Prediction.RawTETA)
	require.Equal(t, history, flight.RawTETASamples)
}

func requireDomainClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	require.Error(t, err)
	var domainError *aman.DomainError
	require.True(t, errors.As(err, &domainError), "expected domain error, got %v", err)
	require.Equal(t, class, domainError.Class)
}

func reviewTime() time.Time {
	return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
}
