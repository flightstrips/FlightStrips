package operational

import (
	"math"
	"strconv"
	"strings"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
)

// Use route-derived feeder timing, never a manual ETA or a holding release,
// to detect terminal arrival order. Stale or missing evidence leaves the
// existing sequence policy in control.
func sequenceSTARProgress(flight aman.AMANFlight) *sequence.STARProgress {
	if flight.DataStatus != aman.DataFresh || flight.SelectedSTARFamily == nil ||
		flight.SelectedFeederFix == nil || flight.RouteProgress == nil ||
		flight.RouteProgress.TerminalDigest == "" || flight.Prediction == nil || !flight.Prediction.Publishable {
		return nil
	}
	feeder := flight.DerivedFeederETA
	if feeder == nil {
		feeder = flight.FeederETA
	}
	if feeder == nil || (feeder.Source != aman.FeederETASourceRoute && feeder.Source != aman.FeederETASourcePassed) {
		return nil
	}
	progress := &sequence.STARProgress{
		Fix:    strings.ToUpper(strings.TrimSpace(*flight.SelectedFeederFix)),
		Passed: feeder.Passed, TerminalDigest: flight.RouteProgress.TerminalDigest,
	}
	if !feeder.Passed {
		if feeder.ETA == nil {
			return nil
		}
		eta := *feeder.ETA
		progress.ETA = &eta
		return progress
	}
	calculation := flight.Prediction.Calculation
	if calculation == nil || len(calculation.Legs) == 0 {
		return nil
	}
	for _, leg := range calculation.Legs {
		if leg.From == "" || leg.To == "" || math.IsNaN(leg.DistanceNM) || math.IsInf(leg.DistanceNM, 0) || leg.DistanceNM < 0 {
			return nil
		}
		progress.DistanceToGoNM += leg.DistanceNM
		// Include endpoint coordinates so differently resolved/direct routes
		// are not compared solely because a waypoint name happens to match.
		endpoint := leg.From + ">" + leg.To + ":" + strconv.FormatFloat(leg.EndLatitude, 'g', -1, 64) + ":" + strconv.FormatFloat(leg.EndLongitude, 'g', -1, 64)
		progress.RemainingFixes = append(progress.RemainingFixes, endpoint)
	}
	return progress
}
