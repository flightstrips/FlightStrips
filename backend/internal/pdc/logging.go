package pdc

import (
	"FlightStrips/internal/models"
	"context"
	"log/slog"
)

func pdcRequestLogger(sessionID int32, callsign, channel string) *slog.Logger {
	return slog.With("session", sessionID, "callsign", callsign, "channel", channel)
}

func pdcRequestChannel(strip *models.Strip) string {
	if isWebPDCRequest(strip) {
		return models.PdcChannelWeb
	}
	return models.PdcChannelCPDLC
}

func logPendingPdcRequest(ctx context.Context, logger *slog.Logger, outcome PdcRequestOutcome) {
	logger.InfoContext(ctx, "PDC Service: Request pending manual review",
		"outcome", outcome.MetricOutcome, "pending_state", outcome.State,
		"faults", outcome.Faults)
}
