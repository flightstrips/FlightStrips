package websocket

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/shared"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"

	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageErrorLogLevelPreservesRealFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		level slog.Level
	}{
		{"ownership rejection", shared.RejectAction("not authorized: strip is owned by another controller"), slog.LevelInfo},
		{"wrapped validation rejection", fmt.Errorf("move: %w", shared.RejectAction("pushback requires TSAT override")), slog.LevelInfo},
		{"AMAN authorization", &aman.DomainError{Class: aman.ErrorUnauthorized}, slog.LevelInfo},
		{"AMAN invalid command", &aman.DomainError{Class: aman.ErrorInvalidArgument}, slog.LevelInfo},
		{"AMAN read only", &aman.DomainError{Class: aman.ErrorReadOnly}, slog.LevelInfo},
		{"AMAN invalid transition", &aman.DomainError{Class: aman.ErrorInvalidTransition}, slog.LevelInfo},
		{"AMAN active conflict", &aman.DomainError{Class: aman.ErrorActiveFlightConflict}, slog.LevelInfo},
		{"cancelled work", fmt.Errorf("position: %w", context.Canceled), slog.LevelDebug},
		{"dependency unavailable", &aman.DomainError{Class: aman.ErrorDependencyUnavailable}, slog.LevelError},
		{"corrupt data", &aman.DomainError{Class: aman.ErrorCorruptData}, slog.LevelError},
		{"missing flight", &aman.DomainError{Class: aman.ErrorNotFound}, slog.LevelError},
		{"exhausted revision retries", &aman.DomainError{Class: aman.ErrorRevisionConflict}, slog.LevelError},
		{"deadline", context.DeadlineExceeded, slog.LevelError},
		{"untyped lookalike", errors.New("pushback requires TSAT override"), slog.LevelError},
		{"joined rejection and failure", errors.Join(shared.RejectAction("not allowed"), io.ErrUnexpectedEOF), slog.LevelError},
		{"wrapped joined failure and cancellation", fmt.Errorf("handle: %w", errors.Join(context.Canceled, io.ErrUnexpectedEOF)), slog.LevelError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.level, messageErrorLogLevel(tc.err))
		})
	}
}

func TestWebsocketWriteLoggingDistinguishesCleanupFromTransportFailure(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, tc := range []struct {
		name  string
		err   error
		level string
	}{
		{"close sent", gorilla.ErrCloseSent, "DEBUG"},
		{"closed socket", &net.OpError{Op: "write", Net: "tcp", Err: net.ErrClosed}, "DEBUG"},
		{"normal close", &gorilla.CloseError{Code: gorilla.CloseNormalClosure}, "DEBUG"},
		{"unexpected close", &gorilla.CloseError{Code: gorilla.CloseProtocolError}, "WARN"},
		{"unexpected EOF", io.ErrUnexpectedEOF, "WARN"},
		{"deadline", context.DeadlineExceeded, "WARN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output.Reset()
			logWebsocketWriteError(context.Background(), testClient{}, "Failed to write websocket ping", tc.err)
			var entry map[string]any
			require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
			assert.Equal(t, tc.level, entry["level"])
			assert.Equal(t, "euroscope", entry["source"])
			assert.Equal(t, tc.err.Error(), entry["error"])
		})
	}
}
