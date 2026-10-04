package aman

import (
	"slices"
	"time"
)

// GainLossPrediction smooths live guidance independently of the operational
// freeze. Retain the last physical estimate when inputs temporarily become
// unusable; the flight's DataStatus still tells clients whether it is current.
// A retired flight or released slot must never retain actionable guidance.
func GainLossPrediction(flight AMANFlight) (time.Time, bool) {
	if flight.State == StateLanded || flight.State == StateRemoved || flight.Slot == nil || flight.Prediction == nil || flight.Prediction.RawTETA.IsZero() {
		return time.Time{}, false
	}
	raw := flight.Prediction
	samples := flight.RawTETASamples
	if len(samples) < 3 || !samples[len(samples)-1].GeneratedAt.Equal(raw.GeneratedAt) || !samples[len(samples)-1].TETA.Equal(raw.RawTETA) ||
		raw.GeneratedAt.Sub(samples[0].GeneratedAt) > 5*time.Minute {
		return raw.RawTETA, true
	}
	switch raw.OperationalReason {
	case OperationalReasonRouteRevision, OperationalReasonRunwayGroupChanged, OperationalReasonGoAround, OperationalReasonFirstUnstable:
		return raw.RawTETA, true
	}
	values := make([]time.Time, len(samples))
	for index, sample := range samples {
		values[index] = sample.TETA
	}
	slices.SortFunc(values, func(a, b time.Time) int { return a.Compare(b) })
	return values[len(values)/2], true
}

// GainLossGuidance limits actionable gain to two minutes. Keep target and
// prediction timestamps intact so clients can still inspect the actual drift.
// Loss is not capped: hiding a large delay would conceal a sequencing problem.
func GainLossGuidance(predicted, target time.Time) (int64, error) {
	return WholeSeconds(min(predicted.Sub(target).Round(time.Second), 2*time.Minute))
}
