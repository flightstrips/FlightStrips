package aman

import (
	"slices"
	"time"
)

// MaxScheduledGain is the maximum time an automatic arrival target may ask
// an aircraft to recover from its current physical prediction.
const MaxScheduledGain = 2 * time.Minute

// GainLossPrediction smooths live guidance independently of the operational
// freeze. Retain the last physical estimate when inputs temporarily become
// unusable; the flight's DataStatus still tells clients whether it is current.
// A retired flight or released slot must never retain actionable guidance.
func GainLossPrediction(flight AMANFlight) (time.Time, bool) {
	if flight.State == StateLanded || flight.State == StateRemoved || flight.Slot == nil || flight.Prediction == nil || flight.Prediction.RawTETA.IsZero() {
		return time.Time{}, false
	}
	if flight.PredictionFilter != nil && flight.PredictionFilter.Unreliable {
		return time.Time{}, false
	}
	return SmoothedPhysicalPrediction(flight)
}

// SmoothedPhysicalPrediction also supports demand from flights without slots.
// It deliberately excludes operational freezes and controller target times.
func SmoothedPhysicalPrediction(flight AMANFlight) (time.Time, bool) {
	if flight.Prediction == nil || flight.Prediction.RawTETA.IsZero() {
		return time.Time{}, false
	}
	raw := flight.Prediction
	samples := flight.RawTETASamples
	if len(samples) < 3 || !samples[len(samples)-1].GeneratedAt.Equal(raw.GeneratedAt) || !samples[len(samples)-1].TETA.Equal(raw.RawTETA) ||
		raw.GeneratedAt.Sub(samples[0].GeneratedAt) > 5*time.Minute {
		return raw.RawTETA, true
	}
	switch raw.OperationalReason {
	case OperationalReasonRouteRevision, OperationalReasonRouteRecovery, OperationalReasonRunwayGroupChanged, OperationalReasonGoAround, OperationalReasonFirstUnstable:
		return raw.RawTETA, true
	}
	values := make([]time.Time, len(samples))
	for index, sample := range samples {
		values[index] = sample.TETA
	}
	slices.SortFunc(values, func(a, b time.Time) int { return a.Compare(b) })
	return values[len(values)/2], true
}

// GainLossTarget removes only planned downstream absorption from remaining
// loss before approach. Landing slots and physical predictions stay intact.
// Never manufacture gain because an aircraft has already absorbed some delay.
func GainLossTarget(flight AMANFlight, predicted time.Time) time.Time {
	if flight.Slot == nil {
		return time.Time{}
	}
	target := flight.Slot.Time
	if !predicted.Before(target) || flight.Expedited || flight.State == StateGoAround || !flight.SequenceDisposition.Participates() ||
		flight.TMAEntry != nil && flight.TMAEntry.LastContainment == TMAInside && ConfirmedActiveHoldingSince(flight) == nil {
		return target
	}
	var absorption time.Duration
	if r := flight.ApproachRelease; r != nil && r.RunwayGroupID == flight.Slot.RunwayGroupID && r.Transit > 0 &&
		(flight.SelectedHolding == nil || r.HoldingID == *flight.SelectedHolding) {
		// An accepted admission survives source outages and loss of holding
		// timing. Recompute against the current slot rather than a cached budget.
		absorption = max(time.Duration(0), target.Sub(r.ReleaseAt)-r.Transit)
	} else if p := flight.ApproachGuidance; p != nil && flight.Prediction != nil && flight.Prediction.Publishable &&
		flight.Prediction.HoldingPlanBlockedBy == nil &&
		flight.DataStatus == DataFresh && flight.FreezeReason != FreezeManual && flight.ManualOrder == nil &&
		flight.SelectedHolding != nil && p.HoldingID == *flight.SelectedHolding && p.RunwayGroupID == flight.Slot.RunwayGroupID &&
		p.SlotAt.Equal(target) && p.PredictionAt.Equal(flight.Prediction.GeneratedAt) {
		digest := flight.Prediction.GeometryDigest
		if b := flight.HoldingReleaseBasis; b != nil && b.HoldingID == p.HoldingID && b.RunwayGroupID == p.RunwayGroupID {
			digest = b.RouteDigest
		}
		if p.RouteDigest == digest {
			absorption = p.Plan.AdditionalApproachTime
		}
	}
	return target.Add(-min(max(time.Duration(0), absorption), target.Sub(predicted)))
}

// GainLossGuidance limits actionable gain to two minutes. Callers supply the
// effective guidance target; the committed landing slot remains inspectable.
// Loss beyond planned approach absorption is not capped.
func GainLossGuidance(predicted, target time.Time) (int64, error) {
	return WholeSeconds(min(predicted.Sub(target).Round(time.Second), MaxScheduledGain))
}
