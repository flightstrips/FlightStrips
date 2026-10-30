package operational

import "FlightStrips/internal/aman"

// Filed-duration estimates are advisory, including when an already-airborne
// flight is first observed. Recover automatic slots written from them by older
// servers before holding or freeze logic can turn them into commitments.
func discardAutomaticPreliminarySlot(flight *aman.AMANFlight) {
	if !isPreliminaryPrediction(flight.Prediction) || flight.FreezeReason == aman.FreezeManual {
		return
	}
	flight.Slot, flight.FrozenSlot, flight.Order, flight.QueueOffers = nil, nil, nil, nil
	flight.FreezeReason, flight.FrozenAt, flight.FrozenOperationalTETA = aman.FreezeNone, nil, nil
	// Controller ordering remains available for the first physical prediction.
	prediction := *flight.Prediction
	prediction.OperationalReason = aman.OperationalReasonPredicted
	prediction.HoldingPlan = nil
	flight.Prediction = &prediction
}
