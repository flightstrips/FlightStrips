package aman

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGainLossGuidance(t *testing.T) {
	target := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		drift time.Duration
		want  int64
	}{{90 * time.Second, 90}, {2 * time.Minute, 120}, {45 * time.Minute, 120}, {-100 * time.Minute, -6000}, {29900 * time.Millisecond, 30}} {
		seconds, err := GainLossGuidance(target.Add(test.drift), target)
		require.NoError(t, err)
		require.Equal(t, test.want, seconds)
	}
}

func TestGainLossTargetExcludesOnlyRemainingPlannedApproachAbsorption(t *testing.T) {
	now := time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		change func(*AMANFlight)
		want   int64
	}{
		{"partial absorption", func(*AMANFlight) {}, -120},
		{"full absorption", func(f *AMANFlight) { f.ApproachRelease.ReleaseAt = now.Add(8 * time.Minute) }, 0},
		{"delay already absorbed enroute", func(f *AMANFlight) { f.Prediction.RawTETA = now.Add(18 * time.Minute) }, 0},
		{"late arrival keeps real gain", func(f *AMANFlight) { f.Prediction.RawTETA = now.Add(25 * time.Minute) }, 120},
		{"slot change recomputes absorption", func(f *AMANFlight) { f.Slot.Time = now.Add(22 * time.Minute) }, -120},
		{"accepted admission survives prediction outage", func(f *AMANFlight) { f.Prediction.Publishable = false; f.DataStatus = DataStale }, -120},
		{"no admission or preview", func(f *AMANFlight) { f.ApproachRelease = nil }, -300},
		{"different runway", func(f *AMANFlight) { f.ApproachRelease.RunwayGroupID = "OTHER" }, -300},
		{"different holding", func(f *AMANFlight) { f.ApproachRelease.HoldingID = "OTHER" }, -300},
		{"inside approach absorbs landing delay", func(f *AMANFlight) { f.TMAEntry = &TMAEntryState{LastContainment: TMAInside} }, -300},
		{"expedited", func(f *AMANFlight) { f.Expedited = true }, -300},
		{"previous admission cannot mask a go-around delay", func(f *AMANFlight) { f.State = StateGoAround }, -300},
	} {
		t.Run(test.name, func(t *testing.T) {
			hold := "HOLD"
			f := AMANFlight{State: StateStable, DataStatus: DataFresh, SelectedHolding: &hold,
				Slot:            &Slot{Time: now.Add(20 * time.Minute), RunwayGroupID: "22"},
				Prediction:      &Prediction{RawTETA: now.Add(15 * time.Minute), GeneratedAt: now, Publishable: true},
				ApproachRelease: &ApproachReleaseReservation{HoldingID: hold, RunwayGroupID: "22", ReleaseAt: now.Add(10 * time.Minute), Transit: 7 * time.Minute}}
			test.change(&f)
			predicted, available := GainLossPrediction(f)
			require.True(t, available)
			seconds, err := GainLossGuidance(predicted, GainLossTarget(f, predicted))
			require.NoError(t, err)
			require.Equal(t, test.want, seconds)
		})
	}
}

func TestProvisionalApproachGuidanceCannotOutliveItsInputs(t *testing.T) {
	now := time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		change func(*AMANFlight)
		want   int64
	}{
		{"current preview", func(*AMANFlight) {}, -120},
		{"stale data", func(f *AMANFlight) { f.DataStatus = DataStale }, -300},
		{"unpublishable", func(f *AMANFlight) { f.Prediction.Publishable = false }, -300},
		{"changed slot", func(f *AMANFlight) { f.ApproachGuidance.SlotAt = now.Add(time.Minute) }, -300},
		{"changed prediction", func(f *AMANFlight) { f.Prediction.GeneratedAt = now.Add(time.Second) }, -300},
		{"changed route", func(f *AMANFlight) { f.Prediction.GeometryDigest = "new-route" }, -300},
		{"changed holding", func(f *AMANFlight) { f.ApproachGuidance.HoldingID = "OTHER" }, -300},
		{"changed runway", func(f *AMANFlight) { f.ApproachGuidance.RunwayGroupID = "OTHER" }, -300},
		{"manual protection", func(f *AMANFlight) { f.FreezeReason = FreezeManual }, -300},
		{"holding order blocker", func(f *AMANFlight) { blocker := Callsign("OLDER"); f.Prediction.HoldingPlanBlockedBy = &blocker }, -300},
	} {
		t.Run(test.name, func(t *testing.T) {
			hold := "HOLD"
			f := AMANFlight{State: StateStable, DataStatus: DataFresh, SelectedHolding: &hold,
				Slot:       &Slot{Time: now.Add(20 * time.Minute), RunwayGroupID: "22"},
				Prediction: &Prediction{RawTETA: now.Add(15 * time.Minute), GeneratedAt: now, Publishable: true, GeometryDigest: "route"}}
			f.ApproachGuidance = &ApproachGuidancePlan{HoldingID: hold, RunwayGroupID: "22", SlotAt: f.Slot.Time,
				PredictionAt: now, RouteDigest: "route", Plan: HoldingPlan{HoldingEntryTime: now.Add(5 * time.Minute),
					ApproachReleaseTime: now.Add(10 * time.Minute), ExpectedHoldingDuration: 5 * time.Minute,
					PostHoldingTransit: 7 * time.Minute, AdditionalApproachTime: 3 * time.Minute}}
			require.NoError(t, f.ApproachGuidance.Validate())
			test.change(&f)
			seconds, err := GainLossGuidance(f.Prediction.RawTETA, GainLossTarget(f, f.Prediction.RawTETA))
			require.NoError(t, err)
			require.Equal(t, test.want, seconds)
		})
	}
}

func TestGainLossPredictionFiltersOutliersWithoutFreezingLiveGuidance(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	target := now.Add(20 * time.Minute)
	flight := AMANFlight{State: StateStable, FreezeReason: FreezeSuperstable, Slot: &Slot{Time: target},
		Prediction: &Prediction{RawTETA: target.Add(-15 * time.Minute), OperationalTETA: target.Add(-10 * time.Minute), GeneratedAt: now, OperationalReason: OperationalReasonSuperstableFreeze},
		RawTETASamples: []RawTETASample{
			{TETA: target.Add(-time.Minute), GeneratedAt: now.Add(-20 * time.Second)},
			{TETA: target.Add(-50 * time.Second), GeneratedAt: now.Add(-10 * time.Second)},
			{TETA: target.Add(-15 * time.Minute), GeneratedAt: now},
		}}
	predicted, available := GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, target.Add(-time.Minute), predicted, "one outlier cannot replace the established live estimate")
	flight.RawTETASamples[1].TETA = flight.Prediction.RawTETA
	predicted, available = GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted, "confirmed changes must flow through even during an operational freeze")
	flight.Prediction.Publishable = false
	predicted, available = GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted, "temporary data loss retains the latest estimate")
	flight.State = StateLanded
	_, available = GainLossPrediction(flight)
	require.False(t, available)
	flight.State, flight.Slot = StateStable, nil
	_, available = GainLossPrediction(flight)
	require.False(t, available)
}

func TestGainLossPredictionAcceptsExplicitRouteChangeImmediately(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	flight := AMANFlight{Slot: &Slot{Time: now.Add(20 * time.Minute)},
		Prediction: &Prediction{RawTETA: now.Add(5 * time.Minute), GeneratedAt: now, OperationalReason: OperationalReasonRouteRevision},
		RawTETASamples: []RawTETASample{
			{TETA: now.Add(20 * time.Minute), GeneratedAt: now.Add(-20 * time.Second)},
			{TETA: now.Add(20 * time.Minute), GeneratedAt: now.Add(-10 * time.Second)},
			{TETA: now.Add(5 * time.Minute), GeneratedAt: now},
		}}
	predicted, available := GainLossPrediction(flight)
	require.True(t, available)
	require.Equal(t, flight.Prediction.RawTETA, predicted)
}
