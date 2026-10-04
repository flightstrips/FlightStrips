package aman

import "time"

// GainLossGuidance limits actionable gain to two minutes. Keep target and
// prediction timestamps intact so clients can still inspect the actual drift.
// Loss is not capped: hiding a large delay would conceal a sequencing problem.
func GainLossGuidance(predicted, target time.Time) (int64, error) {
	return WholeSeconds(min(predicted.Sub(target).Round(time.Second), 2*time.Minute))
}
