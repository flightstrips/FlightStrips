package frontendbinary

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type closeDiagnosticCapture struct{ records chan slog.Record }

func (h closeDiagnosticCapture) Enabled(context.Context, slog.Level) bool { return true }
func (h closeDiagnosticCapture) Handle(_ context.Context, r slog.Record) error {
	h.records <- r.Clone()
	return nil
}
func (h closeDiagnosticCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h closeDiagnosticCapture) WithGroup(string) slog.Handler      { return h }

type closeDiagnosticProjection struct {
	*fixtureProjection
	failure error
}

func (p closeDiagnosticProjection) ReadyForRead() error { return p.failure }

func TestFrontendCloseDiagnosticsPreserveWireAndExcludePayload(t *testing.T) {
	const secret = "private-command-payload-and-bearer-token"
	for _, tc := range []struct {
		name, phase, cause string
		failure            error
		gap                bool
	}{
		{name: "readiness unavailable", phase: "delivery_readiness", cause: "state_replay_behind", failure: errors.New("replay behind stream: 10 < 11")},
		{name: "revision gap", phase: "session_delta", cause: "revision_gap", gap: true},
		{name: "wrapped transport failure", phase: "delivery_readiness", cause: "stream_no_response", failure: fmt.Errorf(secret+": %w", nats.ErrNoStreamResponse)},
		{name: "unknown error text", phase: "delivery_readiness", cause: "unclassified", failure: errors.New(secret)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := closeDiagnosticCapture{records: make(chan slog.Record, 16)}
			previous := slog.Default()
			slog.SetDefault(slog.New(capture))
			defer slog.SetDefault(previous)
			fixture := frontendFixture()
			if tc.gap {
				fixture.buffered = &pb.FrontendDelta{Aggregate: fixture.session.Ref, AggregateRevision: fixture.session.Revision + 2}
			}
			server := httptest.NewServer(Handler{Projection: closeDiagnosticProjection{fixtureProjection: fixture, failure: tc.failure}, Router: &fixtureRouter{}, Auth: fixtureAuth{}})
			defer server.Close()
			conn := dial(t, server.URL)
			defer conn.Close()
			writeTestFrame(t, conn, authFrame())
			initial := readTestFrame(t, conn).GetInitial()
			require.NotNil(t, initial)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err := conn.ReadMessage()
			var closed *websocket.CloseError
			require.ErrorAs(t, err, &closed)
			require.Equal(t, websocket.CloseTryAgainLater, closed.Code)
			require.Equal(t, "projection unavailable", closed.Text)
			var record slog.Record
			select {
			case record = <-capture.records:
			case <-time.After(2 * time.Second):
				t.Fatal("missing close diagnostics")
			}
			attrs := map[string]any{}
			record.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value.Any(); return true })
			require.Equal(t, "frontend session failed", record.Message)
			require.Equal(t, tc.phase, attrs["phase"])
			require.Equal(t, tc.cause, attrs["cause"])
			require.Equal(t, initial.AggregateRevision, attrs["session_revision"])
			require.NotContains(t, fmt.Sprint(attrs), secret)
			require.NotContains(t, attrs, "error")
			require.NotContains(t, attrs, "error_reason")
		})
	}
}

func TestFrontendCloseClassificationPreservesWrappedProtocolFailure(t *testing.T) {
	original := closeFailure{code: websocket.ClosePolicyViolation, reason: "authentication failed"}
	wrapped := fmt.Errorf("private error: %w", original)
	require.Equal(t, "protocol_close", frontendFailureCause(wrapped))
	var failure closeFailure
	require.True(t, errors.As(wrapped, &failure))
	require.Equal(t, original, failure)
	require.False(t, strings.Contains(frontendFailureCause(wrapped), "private"))
}
