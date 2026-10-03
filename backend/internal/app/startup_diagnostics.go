package app

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/nats-io/nats.go"
)

type startupStageError struct {
	stage string
	cause error
}

func (e *startupStageError) Error() string { return "application startup " + e.stage + " failed" }
func (e *startupStageError) Unwrap() error { return e.cause }

// StartupFailureDiagnostics reports structural diagnostics without rendering
// errors: transport errors may embed credential-bearing URLs or provider data.
func StartupFailureDiagnostics(err error) (stage, class, causeType string) {
	stage, class = "unknown", "other"
	var startup *startupStageError
	if errors.As(err, &startup) {
		stage = startup.stage
	}
	switch {
	case errors.Is(err, context.Canceled):
		class = "canceled"
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, nats.ErrTimeout):
		class = "timeout"
	case errors.Is(err, nats.ErrNoResponders):
		class = "no_responders"
	case errors.Is(err, nats.ErrStreamNotFound):
		class = "stream_not_found"
	case errors.Is(err, nats.ErrConnectionClosed):
		class = "connection_closed"
	default:
		var network net.Error
		if errors.As(err, &network) && network.Timeout() {
			class = "timeout"
		}
	}
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return stage, class, fmt.Sprintf("%T", err)
}
