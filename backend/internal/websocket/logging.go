package websocket

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/shared"
	"context"
	"log/slog"
	"net"

	"github.com/gorilla/websocket"
)

// Classify typed rejections rather than matching error text. Unknown failures,
// including exhausted AMAN revision retries, retain ERROR severity.
func messageErrorLogLevel(err error) slog.Level {
	if err == context.Canceled {
		return slog.LevelDebug
	}
	switch e := err.(type) {
	case *shared.ActionRejection:
		return slog.LevelInfo
	case *aman.DomainError:
		switch e.Class {
		case aman.ErrorInvalidArgument, aman.ErrorUnauthorized, aman.ErrorReadOnly,
			aman.ErrorInvalidTransition, aman.ErrorActiveFlightConflict:
			return slog.LevelInfo
		}
	case interface{ Unwrap() []error }:
		// A rejected action joined with a real failure must remain a failure.
		level := slog.LevelDebug
		for _, child := range e.Unwrap() {
			if child != nil {
				level = max(level, messageErrorLogLevel(child))
			}
		}
		return level
	case interface{ Unwrap() error }:
		if child := e.Unwrap(); child != nil {
			return messageErrorLogLevel(child)
		}
	}
	return slog.LevelError
}

func logWebsocketWriteError(ctx context.Context, client Client, message string, err error, attrs ...any) {
	level := slog.LevelWarn
	// A writer can race with connection teardown. Timeouts and other transport
	// failures still warrant a warning.
	if isClosedWriteError(err) {
		level = slog.LevelDebug
	}
	fields := []any{slog.String("source", client.GetSource()), slog.String("cid", client.GetCid()), slog.Any("error", err)}
	slog.Log(ctx, level, message, append(fields, attrs...)...)
}

func isClosedWriteError(err error) bool {
	if err == websocket.ErrCloseSent || err == net.ErrClosed || err == context.Canceled {
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return isClosedWriteError(wrapped.Unwrap())
	}
	return websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}
