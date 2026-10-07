package aman

import "time"

// SequenceDemandArrivalAt returns accepted prediction timing without using slots.
func SequenceDemandArrivalAt(flight AMANFlight) *time.Time {
	prediction := flight.Prediction
	if prediction == nil || !prediction.Publishable {
		return nil
	}
	at := prediction.RawTETA
	if at.IsZero() {
		at = prediction.OperationalTETA
	}
	if at.IsZero() {
		return nil
	}
	return &at
}

// ConfirmedActiveHoldingSince excludes assignments, approaching traffic and
// evidence superseded by a controller cancellation or release.
func ConfirmedActiveHoldingSince(flight AMANFlight) *time.Time {
	stack := flight.HoldingStack
	if stack == nil || !stack.Confirmed || stack.HoldingID == "" || stack.FirstObservedAt.IsZero() ||
		flight.SelectedHolding == nil || stack.HoldingID != *flight.SelectedHolding ||
		flight.Prediction == nil || flight.Prediction.HoldingFixETA == nil {
		return nil
	}
	if canceled := flight.HoldingClearanceCanceledAt; canceled != nil {
		observed := flight.Prediction.InputObservedAt
		if observed.IsZero() {
			observed = flight.Prediction.GeneratedAt
		}
		if !observed.After(*canceled) {
			return nil
		}
	}
	entered := stack.FirstObservedAt
	return &entered
}
