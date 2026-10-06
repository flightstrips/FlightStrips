package services

import (
	"context"
	"log/slog"
	"time"

	"FlightStrips/internal/models"
)

type pushbackTransitionContextKey struct{}
type pushbackTransitionLoggedContextKey struct{}

type pushbackTransitionTiming struct {
	tobt, tsat, ctot string
}

type pushbackTransitionContext struct {
	source  string
	initial pushbackTransitionTiming
}

func snapshotPushbackTransitionTiming(strip *models.Strip) pushbackTransitionTiming {
	if strip == nil {
		return pushbackTransitionTiming{}
	}
	return pushbackTransitionTiming{valueOrEmptyStripTime(strip.EffectiveTobt()), valueOrEmptyStripTime(strip.EffectiveTsat()), valueOrEmptyStripTime(strip.EffectiveCtot())}
}

func logPushbackTransition(ctx context.Context, session int32, callsign, source, previousBay string, initial pushbackTransitionTiming, strip *models.Strip) {
	final := snapshotPushbackTransitionTiming(strip)
	now := time.Now().UTC()
	slog.InfoContext(ctx, "Pushback bay transition recorded",
		slog.Int("session", int(session)), slog.String("callsign", callsign),
		slog.String("source", source), slog.String("previous_bay", previousBay),
		slog.String("initial_tobt", initial.tobt), slog.String("initial_tsat", initial.tsat), slog.String("initial_ctot", initial.ctot),
		slog.String("tobt", final.tobt), slog.String("tsat", final.tsat), slog.String("ctot", final.ctot),
		slog.String("initial_window_state", pushbackWindowState(initial.tsat, now)), slog.String("window_state", pushbackWindowState(final.tsat, now)),
		slog.Bool("override", strip != nil && strip.ValidationStatus != nil && strip.ValidationStatus.IssueType == pushbackTsatValidationIssueType && !strip.ValidationStatus.Active))
}
