package operational

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/trajectory"
)

func activeHoldingReleaseBasis(flight aman.AMANFlight) *aman.HoldingReleaseBasis {
	basis := flight.HoldingReleaseBasis
	if basis == nil || flight.SelectedHolding == nil || flight.SelectedRunwayGroup == nil ||
		flight.HoldingStack == nil || !flight.HoldingStack.Confirmed ||
		flight.State == aman.StateLanded || flight.State == aman.StateRemoved ||
		basis.HoldingID != *flight.SelectedHolding || basis.HoldingID != flight.HoldingStack.HoldingID ||
		!basis.HoldingEntryTime.Equal(flight.HoldingStack.FirstObservedAt) ||
		basis.RunwayGroupID != *flight.SelectedRunwayGroup || basis.Fix != string(operationalHoldingClearanceFix(flight)) {
		return nil
	}
	return basis
}

func (s *Service) captureHoldingReleaseBasis(ctx context.Context, flight *aman.AMANFlight, projection trajectory.Result, input predictor.PerformanceWindInput) error {
	if activeHoldingReleaseBasis(*flight) == nil {
		flight.HoldingReleaseBasis = nil
	}
	if projection.SelectedHolding == nil || flight.SelectedRunwayGroup == nil ||
		flight.State == aman.StateLanded || flight.State == aman.StateRemoved ||
		flight.HoldingStack == nil || !flight.HoldingStack.Confirmed ||
		flight.HoldingStack.HoldingID != string(projection.SelectedHolding.ID) ||
		operationalHoldingClearanceFix(*flight) != projection.SelectedHolding.Fix {
		return nil
	}
	// The prefix is PPOS-to-fix. Only the published path after that fix belongs
	// in a release calculation, regardless of the aircraft's racetrack leg.
	var remaining []trajectory.RemainingLeg
	for i, leg := range projection.Remaining {
		if leg.To == projection.SelectedHolding.Fix {
			remaining = projection.Remaining[i+1:]
			break
		}
	}
	if len(remaining) == 0 {
		return nil
	}
	legs := predictorLegs(remaining)
	geometry := slices.Clone(legs)
	for i := range geometry {
		// Rematerialized leg identifiers do not change the physical journey.
		geometry[i].ID = ""
	}
	encoded, err := json.Marshal(geometry)
	if err != nil {
		return err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(encoded))
	if basis := activeHoldingReleaseBasis(*flight); basis != nil && basis.RouteDigest == digest {
		return nil
	}
	input.Remaining = legs
	if cfl := flight.HoldingClearance.ClearedAltitude; cfl != nil && *cfl > 0 {
		input.AltitudeFeet = float64(*cfl)
	}
	input.CruiseAltitudeFeet = input.AltitudeFeet
	input.DescentConfirmed = true
	input.UseObservedGroundspeedBeforeTOD = false
	// An orbit's track and GS cannot supply a wind estimate for a different
	// journey starting at the fix. Provider wind or nominal performance applies.
	input.CurrentTrackTrueDegrees = nil
	var transit time.Duration
	if input.WakeTurbulenceCategory == predictor.CategoryLight {
		estimate, err := predictor.EstimateRETA(input, predictor.PerformanceWindConfig{})
		if err != nil {
			return err
		}
		transit = estimate.Duration
	} else {
		estimate, err := predictor.EstimatePerformanceWind(ctx, nil, s.deps.Wind, input, predictor.PerformanceWindConfig{})
		if err != nil {
			return err
		}
		transit = estimate.Duration
	}
	flight.HoldingReleaseBasis = &aman.HoldingReleaseBasis{
		HoldingID: string(projection.SelectedHolding.ID), Fix: string(projection.SelectedHolding.Fix),
		RunwayGroupID: *flight.SelectedRunwayGroup, RouteDigest: digest,
		HoldingEntryTime: flight.HoldingStack.FirstObservedAt, PostHoldingTransit: transit,
	}
	return nil
}

func holdingPlanForFlight(flight aman.AMANFlight, prediction aman.Prediction) *aman.HoldingPlan {
	basis := activeHoldingReleaseBasis(flight)
	if basis == nil {
		return holdingPlan(prediction, flight.Slot)
	}
	if !prediction.Publishable || flight.Slot == nil || flight.Slot.RunwayGroupID != basis.RunwayGroupID {
		return nil
	}
	release := flight.Slot.Time.Add(-basis.PostHoldingTransit)
	if !release.After(basis.HoldingEntryTime) {
		return nil
	}
	// Passing the EAT is a missed controller target, not a reason to erase it.
	// A real slot change moves this clock by exactly the same amount.
	return &aman.HoldingPlan{
		HoldingEntryTime: basis.HoldingEntryTime, ApproachReleaseTime: release,
		ExpectedHoldingDuration: release.Sub(basis.HoldingEntryTime), PostHoldingTransit: basis.PostHoldingTransit,
	}
}
