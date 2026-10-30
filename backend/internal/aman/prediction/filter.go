package prediction

import (
	"FlightStrips/internal/aman"
	"time"
)

const (
	SpikeThreshold       = 3 * time.Minute
	SpikeAgreement       = time.Minute
	SpikeMinimumSpan     = 30 * time.Second
	SpikeMaximumAge      = 2 * time.Minute
	UnreliablePrediction = "prediction_unreliable"
)

// Filter is automatic; no controller acknowledgement is involved. The caller
// must apply prediction-derived timing only when accepted is true. Physical
// surveillance and containment evidence are independent of this decision.
func Filter(flight aman.AMANFlight, raw aman.Prediction, feeder *time.Time, bypass, disabled bool) (updated aman.AMANFlight, accepted bool, outcome string) {
	updated = flight
	state := aman.PredictionFilterState{}
	if flight.PredictionFilter != nil {
		state = *flight.PredictionFilter
		if state.Pending != nil {
			copy := *state.Pending
			state.Pending = &copy
		}
	}
	updated.PredictionFilter = &state
	if disabled {
		bypass = true
	}
	at := raw.InputObservedAt
	if at.IsZero() {
		at = raw.GeneratedAt
	}
	if state.LastObservedAt.IsZero() && flight.Prediction != nil {
		state.LastObservedAt = flight.Prediction.InputObservedAt
	}
	if !bypass && !state.LastObservedAt.IsZero() && !at.After(state.LastObservedAt) {
		expirePending(&updated, &state, raw.GeneratedAt)
		return updated, false, "duplicate"
	}
	state.LastObservedAt = at
	if bypass || disabled || flight.Prediction == nil || flight.Prediction.RawTETA.IsZero() {
		state.Pending, state.Unreliable = nil, false
		return updated, true, "bypass"
	}
	priorFeeder := flight.DerivedFeederETA
	if priorFeeder == nil {
		priorFeeder = flight.FeederETA
	}
	spike := absolute(raw.RawTETA.Sub(flight.Prediction.RawTETA)) > SpikeThreshold
	if feeder != nil && priorFeeder != nil && priorFeeder.ETA != nil && priorFeeder.Source == aman.FeederETASourceRoute {
		spike = spike || absolute(feeder.Sub(*priorFeeder.ETA)) > SpikeThreshold
	}
	if !spike {
		state.Pending, state.Unreliable = nil, false
		return updated, true, "accepted"
	}
	pending := state.Pending
	agrees := pending != nil && at.Sub(pending.FirstObservedAt) <= SpikeMaximumAge && absolute(raw.RawTETA.Sub(pending.TETA)) <= SpikeAgreement
	if agrees && feeder != nil && pending.FeederETA != nil {
		agrees = absolute(feeder.Sub(*pending.FeederETA)) <= SpikeAgreement
	}
	if agrees {
		pending.Samples++
		if pending.Samples >= 2 && at.Sub(pending.FirstObservedAt) >= SpikeMinimumSpan {
			state.Pending, state.Unreliable = nil, false
			return updated, true, "sustained_change"
		}
	} else {
		since := raw.GeneratedAt
		if pending != nil {
			since = pending.Since
		}
		state.Pending = &aman.PredictionCandidate{TETA: raw.RawTETA, FeederETA: cloneTime(feeder), FirstObservedAt: at, Since: since, Samples: 1}
	}
	expirePending(&updated, &state, raw.GeneratedAt)
	return updated, false, "filtered"
}

func expirePending(flight *aman.AMANFlight, state *aman.PredictionFilterState, now time.Time) {
	if state.Pending == nil || now.Sub(state.Pending.Since) < SpikeMaximumAge {
		return
	}
	state.Unreliable = true
	if flight.Prediction != nil {
		copy := *flight.Prediction
		copy.Publishable = false
		reason := UnreliablePrediction
		copy.DegradationReason = &reason
		flight.Prediction = &copy
	}
}

// ExpireFilter also runs when stale surveillance prevents a new calculation.
func ExpireFilter(flight aman.AMANFlight, now time.Time) aman.AMANFlight {
	if flight.PredictionFilter == nil {
		return flight
	}
	state := *flight.PredictionFilter
	flight.PredictionFilter = &state
	expirePending(&flight, &state, now)
	return flight
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
