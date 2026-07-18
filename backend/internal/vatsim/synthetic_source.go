package vatsim

import (
	"context"
	"strings"
	"sync"
	"time"
)

// SyntheticSource is an in-memory VATSIM source for explicitly enabled local
// test tools. It never performs network I/O.
type SyntheticSource struct {
	mu      sync.RWMutex
	flights map[string]Flight
}

func NewSyntheticSource() *SyntheticSource {
	return &SyntheticSource{flights: make(map[string]Flight)}
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
	now := time.Now().UTC()

	byCallsign := make(map[string]Flight, len(s.flights))
	byCID := make(map[string]Flight, len(s.flights))
	for callsign, flight := range s.flights {
		byCallsign[callsign] = flight
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
