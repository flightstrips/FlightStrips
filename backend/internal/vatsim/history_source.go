package vatsim

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

// SnapshotReplaySource replays saved VATSIM v3 feed generations using the
// same normalization and revision protection as the live cache.
type SnapshotReplaySource struct {
	mu       sync.RWMutex
	snapshot cacheSnapshot
}

func NewSnapshotReplaySource() *SnapshotReplaySource {
	return &SnapshotReplaySource{snapshot: newCacheSnapshot(time.Time{}, time.Time{})}
}

func (s *SnapshotReplaySource) Load(reader io.Reader, receivedAt time.Time) error {
	return s.LoadFiltered(reader, receivedAt, nil)
}

// LoadFiltered replays a saved VATSIM generation while retaining only flights
// accepted by include. The timestamp is the feed's embedded update time, not
// the file's filesystem timestamp. A nil include keeps every flight. It is intended for
// focused historical replays that must retain the live cache's normalization
// and revision semantics without processing unrelated traffic.
func (s *SnapshotReplaySource) LoadFiltered(reader io.Reader, receivedAt time.Time, include func(time.Time, Flight) bool) error {
	var payload networkDataResponse
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return fmt.Errorf("decode VATSIM history snapshot: %w", err)
	}
	next := newCacheSnapshot(payload.General.UpdateTimestamp, receivedAt.UTC())
	for _, pilot := range payload.Pilots {
		flight := toFlight(pilot, FlightStateOnline)
		if include == nil || include(next.timestamp, flight) {
			next.add(flight)
		}
	}
	for _, prefile := range payload.Prefiles {
		flight := toFlight(prefile, FlightStatePrefile)
		if include == nil || include(next.timestamp, flight) {
			next.add(flight)
		}
	}
	s.mu.Lock()
	s.snapshot = preserveNewerFlightPlans(s.snapshot, next)
	s.mu.Unlock()
	return nil
}

func (s *SnapshotReplaySource) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{Timestamp: s.snapshot.timestamp, flightsByCallsign: s.snapshot.flightsByCallsign, flightsByCID: s.snapshot.flightsByCID}
}

var _ SnapshotSource = (*SnapshotReplaySource)(nil)
