package operational

import (
	"FlightStrips/internal/aman"
	"context"
	"encoding/json"
	"time"
)

type predictionRefreshKey struct{}

func withPredictionRefresh(ctx context.Context) context.Context {
	return context.WithValue(ctx, predictionRefreshKey{}, true)
}
func predictionRefreshRequested(ctx context.Context) bool {
	value, _ := ctx.Value(predictionRefreshKey{}).(bool)
	return value
}

func predictionTimeValue(p *aman.Prediction) time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.RawTETA
}

func predictionDistanceValue(p *aman.Prediction) *float64 {
	if p == nil {
		return nil
	}
	return p.DistanceToGoNM
}

// Preserve the accepted model's speed basis as well as the new candidate's.
// The latest observation alone cannot explain a change from an older estimate.
func predictionCalculationContext(p *aman.Prediction) string {
	if p == nil {
		return ""
	}
	context := map[string]any{"model": p.ModelVersion, "observed_at": p.InputObservedAt, "weather_source": p.WeatherSource}
	if p.Calculation != nil && len(p.Calculation.Segments) > 0 {
		first := p.Calculation.Segments[0]
		context["phase"], context["formula"] = first.PhaseID, first.PhaseFormula
		context["groundspeed_knots"], context["altitude_feet"] = first.GroundspeedKnots, first.AltitudeFeet
	}
	encoded, err := json.Marshal(context)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func predictionPhaseDurations(p aman.Prediction) map[string]float64 {
	result := map[string]float64{}
	if p.Calculation != nil {
		for _, segment := range p.Calculation.Segments {
			result[segment.PhaseID] += segment.Duration.Seconds()
		}
	}
	return result
}
