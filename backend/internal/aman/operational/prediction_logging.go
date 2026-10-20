package operational

import (
	"FlightStrips/internal/aman"
	"log/slog"
)

func predictionMovementLogLevel(previous, current aman.Prediction, routeChanged bool) slog.Level {
	// Large movements are expected when the accepted route or prediction
	// inputs change. Retain warnings for unexplained movements on the same model.
	if routeChanged || previous.GeometryDigest != current.GeometryDigest ||
		previous.DatasetVersion != current.DatasetVersion || previous.ModelVersion != current.ModelVersion ||
		previous.ConfigVersion != current.ConfigVersion || previous.Basis != current.Basis ||
		stringValue(previous.PerformanceProfileID) != stringValue(current.PerformanceProfileID) ||
		stringValue(previous.WeatherSource) != stringValue(current.WeatherSource) {
		return slog.LevelInfo
	}
	return slog.LevelWarn
}
