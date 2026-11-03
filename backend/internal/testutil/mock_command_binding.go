package testutil

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"slices"
	"sync"
	"sync/atomic"
)

// MockCommandBinding models one captured connection for service-boundary tests.
type MockCommandBinding struct {
	SessionID   int32
	CIDValue    string
	Unavailable atomic.Bool
	mu          sync.Mutex
	sent        []euroscope.OutgoingMessage
}

func (b *MockCommandBinding) Session() int32                        { return b.SessionID }
func (b *MockCommandBinding) CID() string                           { return b.CIDValue }
func (b *MockCommandBinding) Same(other shared.CommandBinding) bool { return other == b }
func (b *MockCommandBinding) Validate() error {
	if b.Unavailable.Load() {
		return shared.RejectAction("captured plugin disconnected")
	}
	return nil
}
func (b *MockCommandBinding) Send(event euroscope.OutgoingMessage) error {
	if err := b.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, event)
	return nil
}
func (b *MockCommandBinding) Sent() []euroscope.OutgoingMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.sent)
}
