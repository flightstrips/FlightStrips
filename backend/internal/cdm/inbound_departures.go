package cdm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/pkg/events/frontend"
)

const inboundPollInterval = 5 * time.Minute

func (c *Client) IFPSByArrivalAirport(ctx context.Context, airport string) (BulkIFPSData, error) {
	data, err := c.doRequest(ctx, http.MethodGet, "/ifps/arrAirport", map[string]string{"airport": airport}, nil, nil)
	if err != nil {
		return nil, err
	}
	var rows BulkIFPSData
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("vIFF arrivals response must be an array")
	}
	return rows, nil
}

// The feed supplies UTC clock times without dates. Choose the nearest day to
// CTOT, so 2358 -> 0008 is ten minutes late, rather than almost a day early.
func inboundClockSeconds(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 4 && len(value) != 6 {
		return 0, false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return 0, false
		}
	}
	h, _ := strconv.Atoi(value[:2])
	m, _ := strconv.Atoi(value[2:4])
	sec := 0
	if len(value) == 6 {
		sec, _ = strconv.Atoi(value[4:])
	}
	return h*3600 + m*60 + sec, h < 24 && m < 60 && sec < 60
}

func assessInboundDeparture(row IFPSData) (frontend.InboundDeparture, bool) {
	ctot, validCtot := inboundClockSeconds(row.CTOT)
	atot, validAtot := inboundClockSeconds(row.ATOT)
	if !validCtot || !validAtot {
		return frontend.InboundDeparture{}, false
	}
	delta := atot - ctot
	if delta > 12*3600 {
		delta -= 24 * 3600
	}
	if delta < -12*3600 {
		delta += 24 * 3600
	}
	status, outside := "compliant", 0
	if delta < -5*60 {
		status, outside = "early", -5*60-delta
	}
	if delta > 10*60 {
		status, outside = "late", delta-10*60
	}
	return frontend.InboundDeparture{
		Callsign: strings.ToUpper(strings.TrimSpace(row.Callsign)), Departure: strings.ToUpper(strings.TrimSpace(row.Departure)),
		Arrival: strings.ToUpper(strings.TrimSpace(row.Arrival)), Ctot: strings.TrimSpace(row.CTOT), Atot: strings.TrimSpace(row.ATOT),
		ETA: row.ETA, Status: status, DeviationSeconds: delta, OutsideWindowSeconds: outside,
	}, true
}

type inboundAirport struct {
	ctx      context.Context
	cancel   context.CancelFunc
	sessions map[int32]struct{}
	snapshot frontend.InboundDeparturesEvent
	// vIFF excludes flights with ATOT from recalculation. Freeze the first
	// departed snapshot (including an absent/invalid CTOT), never assess a
	// departed flight against a subsequently supplied or revised slot.
	// https://cdm.vatsimspain.es/docs.html (Final CTOT logic / DPIs)
	departed map[string]inboundDepartureSnapshot
}

type inboundDepartureSnapshot struct {
	row        IFPSData
	observedAt time.Time
}

type inboundDepartures struct {
	mu       sync.Mutex
	ctx      context.Context
	airports map[string]*inboundAirport
	service  *Service
}

func (s *Service) RegisterInboundSession(session int32, name, airport string) {
	if s.inbound == nil || !s.client.isValid || !isViffEnabledSession(name) || s.isSessionRemoved(session) {
		return
	}
	s.inbound.addSession(session, strings.ToUpper(strings.TrimSpace(airport)))
}

func (s *Service) CurrentInboundDepartures(session int32, airport string) frontend.InboundDeparturesEvent {
	result := frontend.InboundDeparturesEvent{SessionID: session, Airport: airport, Flights: []frontend.InboundDeparture{}}
	if s.inbound == nil {
		return result
	}
	s.inbound.mu.Lock()
	defer s.inbound.mu.Unlock()
	entry := s.inbound.airports[strings.ToUpper(strings.TrimSpace(airport))]
	if entry == nil {
		return result
	}
	if _, ok := entry.sessions[session]; !ok {
		return result
	}
	result = entry.snapshot
	result.SessionID = session
	result.Flights = append([]frontend.InboundDeparture{}, result.Flights...)
	return result
}

func (m *inboundDepartures) addSession(session int32, airport string) {
	if airport == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.service.isSessionRemoved(session) {
		return
	}
	entry := m.airports[airport]
	if entry == nil {
		entry = &inboundAirport{sessions: make(map[int32]struct{}), departed: make(map[string]inboundDepartureSnapshot), snapshot: frontend.InboundDeparturesEvent{Airport: airport, Enabled: true, Stale: true, Flights: []frontend.InboundDeparture{}}}
		m.airports[airport] = entry
		if m.ctx != nil {
			m.launch(entry)
		}
	}
	entry.sessions[session] = struct{}{}
}

func (m *inboundDepartures) launch(entry *inboundAirport) {
	entry.ctx, entry.cancel = context.WithCancel(m.ctx)
	go m.poll(entry)
}

func (m *inboundDepartures) start(ctx context.Context) {
	m.mu.Lock()
	m.ctx = ctx
	for _, entry := range m.airports {
		m.launch(entry)
	}
	m.mu.Unlock()
	sessions, err := m.service.sessionRepo.List(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to initialize inbound CTOT monitoring", slog.Any("error", err))
	}
	for _, session := range sessions {
		if session != nil {
			m.service.RegisterInboundSession(session.ID, session.Name, session.Airport)
		}
	}
}

func (m *inboundDepartures) removeSession(session int32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for airport, entry := range m.airports {
		delete(entry.sessions, session)
		if len(entry.sessions) == 0 {
			if entry.cancel != nil {
				entry.cancel()
			}
			delete(m.airports, airport)
		}
	}
}

func (m *inboundDepartures) poll(entry *inboundAirport) {
	for {
		if entry.ctx.Err() != nil {
			return
		}
		rows, err := m.service.client.IFPSByArrivalAirport(entry.ctx, entry.snapshot.Airport)
		if entry.ctx.Err() != nil {
			return
		}
		now := time.Now().UTC()
		m.mu.Lock()
		m.apply(entry, rows, err, now)
		event := entry.snapshot
		sessions := make([]int32, 0, len(entry.sessions))
		for session := range entry.sessions {
			sessions = append(sessions, session)
		}
		m.mu.Unlock()
		for _, session := range sessions {
			if m.service.isSessionRemoved(session) {
				continue
			}
			event.SessionID = session
			m.service.publisher.SendInboundDepartures(session, event)
		}
		delay := inboundPollInterval
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.RetryAfter > delay {
			delay = apiError.RetryAfter
		}
		if err != nil {
			slog.WarnContext(entry.ctx, "Failed to refresh inbound CTOT findings", slog.String("airport", event.Airport), slog.Any("error", err))
		}
		timer := time.NewTimer(delay)
		select {
		case <-entry.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// apply runs under the monitor lock. Failed fetches retain every finding.
func (m *inboundDepartures) apply(entry *inboundAirport, rows BulkIFPSData, err error, now time.Time) {
	entry.snapshot.Stale = err != nil
	if err != nil {
		return
	}
	// Keep departed slot history through temporary disappearance from a
	// successful feed, while bounding history and allowing next-day reuse.
	for key, snapshot := range entry.departed {
		if now.Sub(snapshot.observedAt) >= 24*time.Hour {
			delete(entry.departed, key)
		}
	}
	flights := []frontend.InboundDeparture{}
	seen := make(map[string]struct{})
	for _, row := range rows {
		if strings.TrimSpace(row.Callsign) == "" || !strings.EqualFold(strings.TrimSpace(row.Arrival), entry.snapshot.Airport) {
			continue
		}
		atot, departed := inboundClockSeconds(row.ATOT)
		if !departed {
			continue
		}
		key := fmt.Sprintf("%s/%s/%s/%d", strings.ToUpper(strings.TrimSpace(row.Callsign)), strings.ToUpper(strings.TrimSpace(row.Departure)), strings.TrimSpace(row.CID), atot)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		if old, ok := entry.departed[key]; ok {
			row.CTOT = old.row.CTOT
		} else {
			entry.departed[key] = inboundDepartureSnapshot{row: row, observedAt: now}
		}
		flight, valid := assessInboundDeparture(row)
		if valid && flight.Status != "compliant" {
			flights = append(flights, flight)
		}
	}
	sort.Slice(flights, func(i, j int) bool { return flights[i].Callsign < flights[j].Callsign })
	entry.snapshot.Flights = flights
	entry.snapshot.UpdatedAt = now.Format(time.RFC3339)
}
