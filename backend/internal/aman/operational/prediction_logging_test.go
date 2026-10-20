package operational

import (
	"FlightStrips/internal/aman"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPredictionMovementSeverityRequiresAnExplainedInputChange(t *testing.T) {
	previous := aman.Prediction{GeometryDigest: "route-1", DatasetVersion: "2610", ModelVersion: "model-1", ConfigVersion: "config-1"}
	for _, tc := range []struct {
		name         string
		change       func(*aman.Prediction)
		routeChanged bool
		level        slog.Level
	}{
		{"unexplained movement", func(*aman.Prediction) {}, false, slog.LevelWarn},
		{"new observation alone", func(p *aman.Prediction) { p.InputObservedAt = p.InputObservedAt.Add(1) }, false, slog.LevelWarn},
		{"accepted direct or route", func(*aman.Prediction) {}, true, slog.LevelInfo},
		{"geometry changed", func(p *aman.Prediction) { p.GeometryDigest = "route-2" }, false, slog.LevelInfo},
		{"navigation changed", func(p *aman.Prediction) { p.DatasetVersion = "2611" }, false, slog.LevelInfo},
		{"model changed", func(p *aman.Prediction) { p.ModelVersion = "model-2" }, false, slog.LevelInfo},
		{"configuration changed", func(p *aman.Prediction) { p.ConfigVersion = "config-2" }, false, slog.LevelInfo},
		{"weather source changed", func(p *aman.Prediction) { v := "weather-2"; p.WeatherSource = &v }, false, slog.LevelInfo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := previous
			tc.change(&current)
			assert.Equal(t, tc.level, predictionMovementLogLevel(previous, current, tc.routeChanged))
		})
	}
}
