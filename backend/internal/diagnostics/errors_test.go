package diagnostics

import (
	euroscope "FlightStrips/pkg/events/euroscope"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMessagePreservesJoinedCausesAndRedactsCredentials(t *testing.T) {
	err := fmt.Errorf("session 42: %w", errors.Join(
		errors.New("CDM reconciliation: position stale"),
		errors.New("PDC reconciliation: nats://user:private-password@localhost:4222 unavailable token=private-token Bearer private-bearer"),
	))
	got := Message(err)
	for _, want := range []string{"session 42", "CDM reconciliation: position stale", "PDC reconciliation", "localhost:4222"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	for _, secret := range []string{"private-password", "private-token", "private-bearer", "\n"} {
		if strings.Contains(got, secret) {
			t.Fatalf("unsafe diagnostics: %q", got)
		}
	}
}

func TestOneofNameUsesWireField(t *testing.T) {
	frame := &euroscope.Envelope{Event: &euroscope.Envelope_TrackingControllerChanged{}}
	if got := OneofName(frame, "event"); got != "tracking_controller_changed" {
		t.Fatalf("event name = %q", got)
	}
}
