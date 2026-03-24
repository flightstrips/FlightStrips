package euroscope

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// buildMinimalHub returns a Hub with enough fields initialised for
// HasActiveClientForAirport / airportClientCount tests (no network connections needed).
func buildMinimalHub() *Hub {
	return &Hub{
		airportClientCount: make(map[string]int),
	}
}

// TestHasActiveClientForAirport_NoClients verifies that a newly-created hub
// reports no active clients for any airport.
func TestHasActiveClientForAirport_NoClients(t *testing.T) {
	hub := buildMinimalHub()
	assert.False(t, hub.HasActiveClientForAirport("EKCH"), "empty hub must have no active clients")
}

// TestHasActiveClientForAirport_AfterRegister verifies that incrementing the
// count (as OnRegister does) causes HasActiveClientForAirport to return true.
func TestHasActiveClientForAirport_AfterRegister(t *testing.T) {
	hub := buildMinimalHub()

	// Simulate OnRegister for one EKCH client.
	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"]++
	hub.airportClientsMu.Unlock()

	assert.True(t, hub.HasActiveClientForAirport("EKCH"))
	assert.False(t, hub.HasActiveClientForAirport("EGLL"), "different airport should still return false")
}

// TestHasActiveClientForAirport_AfterUnregister verifies that decrementing the
// count (as OnUnregister does) returns false once all clients disconnect.
func TestHasActiveClientForAirport_AfterUnregister(t *testing.T) {
	hub := buildMinimalHub()

	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"]++
	hub.airportClientsMu.Unlock()

	require_true(t, hub.HasActiveClientForAirport("EKCH"))

	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"]--
	hub.airportClientsMu.Unlock()

	assert.False(t, hub.HasActiveClientForAirport("EKCH"), "must return false after all clients disconnect")
}

// TestHasActiveClientForAirport_MultipleClients verifies that two clients for
// the same airport are tracked and both must disconnect before returning false.
func TestHasActiveClientForAirport_MultipleClients(t *testing.T) {
	hub := buildMinimalHub()

	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"] += 2
	hub.airportClientsMu.Unlock()

	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"]--
	hub.airportClientsMu.Unlock()

	assert.True(t, hub.HasActiveClientForAirport("EKCH"), "still one client connected")

	hub.airportClientsMu.Lock()
	hub.airportClientCount["EKCH"]--
	hub.airportClientsMu.Unlock()

	assert.False(t, hub.HasActiveClientForAirport("EKCH"), "all clients disconnected")
}

func require_true(t *testing.T, v bool) {
	t.Helper()
	if !v {
		t.Fatal("expected true")
	}
}
