package operational

import (
	"FlightStrips/internal/aman"
	"context"
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

func predictionPhaseDurations(p aman.Prediction) map[string]float64 {
	result := map[string]float64{}
	if p.Calculation != nil {
		for _, segment := range p.Calculation.Segments {
			result[segment.PhaseID] += segment.Duration.Seconds()
		}
	}
	return result
}
