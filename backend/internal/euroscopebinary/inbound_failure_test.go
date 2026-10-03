package euroscopebinary

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"FlightStrips/internal/cluster"
)

func TestInboundFailureClassifiesWrappedPositionIntegrityWithoutPayload(t *testing.T) {
	private := "private frame content and authentication input"
	err := fmt.Errorf("%s: %w", private, cluster.ErrPositionIntegrityStale)
	if got := inboundFailureReason(err); got != "position_integrity_stale" {
		t.Fatalf("wrapped integrity failure classification = %q", got)
	}
	if got := inboundFailureReason(errors.New(private)); got != "inbound_failure" {
		t.Fatalf("unclassified payload escaped into telemetry: %q", got)
	}
	if got := inboundFailureReason(fmt.Errorf("%s: %w", private, context.Canceled)); got != "context_canceled" {
		t.Fatalf("cancellation classification = %q", got)
	}
}
