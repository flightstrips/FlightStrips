package vatsim

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// SyntheticSource is an in-memory VATSIM source for explicitly enabled local
// test tools. It never performs network I/O.
type SyntheticSource struct {
	mu      sync.RWMutex
	flights map[string]Flight
	replay  *SnapshotReplaySource
}

func NewSyntheticSource() *SyntheticSource {
	return &SyntheticSource{flights: make(map[string]Flight), replay: NewSnapshotReplaySource()}
}

// LoadReplay adds one saved VATSIM generation to this offline source. The
// synthetic records remain available for the SAT test console as well.
func (s *SyntheticSource) LoadReplay(reader io.Reader, receivedAt time.Time) error {
	if s == nil {
		return errors.New("synthetic source is unavailable")
	}
	return s.replay.Load(reader, receivedAt)
}

func (s *SyntheticSource) ResetReplay() {
	if s != nil {
		s.replay.Reset()
	}
}

func (s *SyntheticSource) Upsert(flight Flight) {
	if s == nil {
		return
	}
	flight.Callsign = normalizeCallsign(flight.Callsign)
	flight.CID = strings.TrimSpace(flight.CID)
	if flight.Callsign == "" || flight.CID == "" {
		return
	}
	s.mu.Lock()
	s.flights[flight.Callsign] = flight
	s.mu.Unlock()
}

func (s *SyntheticSource) Remove(callsign string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.flights, normalizeCallsign(callsign))
	s.mu.Unlock()
}

func (s *SyntheticSource) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.flights = make(map[string]Flight)
	s.mu.Unlock()
}

func (s *SyntheticSource) Snapshot() Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	replayed := s.replay.Snapshot()
	now := replayed.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}

	byCallsign := make(map[string]Flight, len(s.flights))
	byCID := make(map[string]Flight, len(s.flights))
	for callsign, flight := range s.flights {
		byCallsign[callsign] = flight
		if current, ok := byCID[flight.CID]; !ok || preferFlight(flight, current) {
			byCID[flight.CID] = flight
		}
	}
	for _, flight := range replayed.Flights() {
		byCallsign[flight.Callsign] = flight
		if current, ok := byCID[flight.CID]; !ok || preferFlight(flight, current) {
			byCID[flight.CID] = flight
		}
	}
	return Snapshot{
		Timestamp:         now,
		flightsByCallsign: byCallsign,
		flightsByCID:      byCID,
	}
}

func (s *SyntheticSource) VerifyPilotOwnsCallsign(_ context.Context, cid, callsign string) (bool, error) {
	flight, ok := s.Snapshot().FlightByCallsign(callsign)
	return ok && flight.Online() && flight.CID == strings.TrimSpace(cid), nil
}

func (s *SyntheticSource) GetCallsignByCID(_ context.Context, cid string) (string, bool, error) {
	flight, ok := s.Snapshot().FlightByCID(cid)
	if !ok || !flight.Online() {
		return "", false, nil
	}
	return flight.Callsign, true, nil
}
