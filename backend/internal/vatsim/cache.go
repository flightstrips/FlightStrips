package vatsim

import (
	"FlightStrips/internal/metrics"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	defaultStatusURL       = "https://status.vatsim.net/status.json"
	defaultRefreshInterval = 15 * time.Second
	defaultHTTPTimeout     = 10 * time.Second
)

// FlightState identifies whether VATSIM reported the flight as connected or
// merely prefiled in the current network-data snapshot.
type FlightState string

const (
	FlightStateOnline  FlightState = "online"
	FlightStatePrefile FlightState = "prefile"
)

// FlightPlan contains the parts of a VATSIM flight plan used by downstream
// consumers. EOBT and EnrouteDuration retain the feed's HHMM representation.
type FlightPlan struct {
	FlightRules     string
	Aircraft        string
	AircraftFAA     string
	AircraftShort   string
	Origin          string
	Destination     string
	Alternate       string
	EOBT            string
	EnrouteDuration string
	Remarks         string
	Route           string
	AssignedSquawk  string
	Revision        int64
}

// Flight is one immutable, normalized VATSIM pilot or prefile record.
type Flight struct {
	CID         string
	Callsign    string
	State       FlightState
	Latitude    float64
	Longitude   float64
	Altitude    int
	Groundspeed int
	LogonTime   time.Time
	LastUpdated time.Time
	FlightPlan  FlightPlan
}

// Online reports whether the flight is currently connected to VATSIM.
func (f Flight) Online() bool {
	return f.State == FlightStateOnline
}

// Prefile reports whether the flight is an offline VATSIM prefile.
func (f Flight) Prefile() bool {
	return f.State == FlightStatePrefile
}

// Snapshot is a read-only view of a single VATSIM feed generation. The lookup
// methods return values, so callers cannot mutate cache state.
type Snapshot struct {
	Timestamp        time.Time
	LastRefreshError error

	flightsByCallsign map[string]Flight
	flightsByCID      map[string]Flight
}

// SnapshotSource supplies the latest immutable network-data generation.
type SnapshotSource interface {
	Snapshot() Snapshot
}

// FlightSource is the complete VATSIM lookup surface consumed by the app.
type FlightSource interface {
	SnapshotSource
	VerifyPilotOwnsCallsign(context.Context, string, string) (bool, error)
	GetCallsignByCID(context.Context, string) (string, bool, error)
}

// FlightByCallsign retrieves either an online pilot or prefile by normalized
// callsign.
func (s Snapshot) FlightByCallsign(callsign string) (Flight, bool) {
	flight, ok := s.flightsByCallsign[normalizeCallsign(callsign)]
	return flight, ok
}

// FlightByCID retrieves either an online pilot or prefile by VATSIM CID.
func (s Snapshot) FlightByCID(cid string) (Flight, bool) {
	flight, ok := s.flightsByCID[strings.TrimSpace(cid)]
	return flight, ok
}

// Flights returns the snapshot records sorted by callsign.
func (s Snapshot) Flights() []Flight {
	flights := make([]Flight, 0, len(s.flightsByCallsign))
	for _, flight := range s.flightsByCallsign {
		flights = append(flights, flight)
	}
	slices.SortFunc(flights, func(left, right Flight) int {
		return strings.Compare(left.Callsign, right.Callsign)
	})
	return flights
}

type cacheSnapshot struct {
	timestamp         time.Time
	flightsByCallsign map[string]Flight
	flightsByCID      map[string]Flight
}

type Cache struct {
	client          *http.Client
	statusURL       string
	refreshInterval time.Duration

	mu               sync.RWMutex
	snapshot         cacheSnapshot
	dataURL          string
	lastRefreshError error
}

type statusResponse struct {
	Data struct {
		V3 []string `json:"v3"`
	} `json:"data"`
}

type networkDataResponse struct {
	General struct {
		UpdateTimestamp time.Time `json:"update_timestamp"`
	} `json:"general"`
	Pilots   []networkFlight `json:"pilots"`
	Prefiles []networkFlight `json:"prefiles"`
}

type networkFlight struct {
	CID         int64       `json:"cid"`
	Callsign    string      `json:"callsign"`
	Latitude    float64     `json:"latitude"`
	Longitude   float64     `json:"longitude"`
	Altitude    int         `json:"altitude"`
	Groundspeed int         `json:"groundspeed"`
	LogonTime   time.Time   `json:"logon_time"`
	LastUpdated time.Time   `json:"last_updated"`
	FlightPlan  *flightPlan `json:"flight_plan"`
}

type flightPlan struct {
	FlightRules         string `json:"flight_rules"`
	Aircraft            string `json:"aircraft"`
	AircraftFAA         string `json:"aircraft_faa"`
	AircraftShort       string `json:"aircraft_short"`
	Departure           string `json:"departure"`
	Arrival             string `json:"arrival"`
	Alternate           string `json:"alternate"`
	DepartureTime       string `json:"deptime"`
	EnrouteTime         string `json:"enroute_time"`
	Remarks             string `json:"remarks"`
	Route               string `json:"route"`
	RevisionID          int64  `json:"revision_id"`
	AssignedTransponder string `json:"assigned_transponder"`
}

func NewCache(statusURL string, refreshInterval time.Duration, client *http.Client) *Cache {
	if strings.TrimSpace(statusURL) == "" {
		statusURL = defaultStatusURL
	}
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &Cache{
		client:          client,
		statusURL:       statusURL,
		refreshInterval: refreshInterval,
		snapshot: cacheSnapshot{
			flightsByCallsign: make(map[string]Flight),
			flightsByCID:      make(map[string]Flight),
		},
	}
}

func (c *Cache) Start(ctx context.Context) {
	_ = c.refresh(ctx)

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.refresh(ctx)
		}
	}
}

// Snapshot returns the current data and refresh health without initiating I/O.
func (c *Cache) Snapshot() Snapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	snapshot := Snapshot{
		Timestamp:         c.snapshot.timestamp,
		LastRefreshError:  c.lastRefreshError,
		flightsByCallsign: c.snapshot.flightsByCallsign,
		flightsByCID:      c.snapshot.flightsByCID,
	}
	return snapshot
}

func (c *Cache) VerifyPilotOwnsCallsign(ctx context.Context, cid string, callsign string) (bool, error) {
	normalizedCID := strings.TrimSpace(cid)
	normalizedCallsign := normalizeCallsign(callsign)
	if normalizedCID == "" || normalizedCallsign == "" {
		return false, nil
	}

	if pilot, ok := c.Snapshot().FlightByCallsign(normalizedCallsign); ok {
		return pilot.CID == normalizedCID && pilot.Online(), nil
	}

	if err := c.refresh(ctx); err != nil {
		return false, err
	}

	pilot, ok := c.Snapshot().FlightByCallsign(normalizedCallsign)
	if !ok {
		return false, nil
	}
	return pilot.CID == normalizedCID && pilot.Online(), nil
}

func (c *Cache) GetCallsignByCID(ctx context.Context, cid string) (string, bool, error) {
	normalizedCID := strings.TrimSpace(cid)
	if normalizedCID == "" {
		return "", false, nil
	}

	if pilot, ok := c.getOnlinePilotByCID(normalizedCID); ok {
		return pilot.Callsign, true, nil
	}

	if err := c.refresh(ctx); err != nil {
		return "", false, err
	}

	pilot, ok := c.getOnlinePilotByCID(normalizedCID)
	if !ok {
		return "", false, nil
	}
	return pilot.Callsign, true, nil
}

func (c *Cache) refresh(ctx context.Context) error {
	snapshot, dataURL, err := c.fetch(ctx)
	if err != nil {
		c.recordRefreshError(err)
		slog.WarnContext(ctx, "SAT VATSIM feed refresh failed", slog.Any("error", err))
		return err
	}

	c.mu.Lock()
	snapshot = preserveNewerFlightPlans(c.snapshot, snapshot)
	c.snapshot = snapshot
	c.dataURL = dataURL
	c.lastRefreshError = nil
	c.mu.Unlock()
	age := time.Since(snapshot.timestamp)
	if age < 0 {
		age = 0
	}
	pilots, prefiles := 0, 0
	for _, flight := range snapshot.flightsByCallsign {
		if flight.Prefile() {
			prefiles++
		} else {
			pilots++
		}
	}
	metrics.RecordSATFeedSnapshot(ctx, age, pilots, prefiles)
	slog.InfoContext(ctx, "SAT VATSIM feed refreshed", slog.Time("snapshot_at", snapshot.timestamp), slog.Duration("snapshot_age", age), slog.Int("pilots", pilots), slog.Int("prefiles", prefiles))

	return nil
}

func (c *Cache) fetch(ctx context.Context) (cacheSnapshot, string, error) {
	dataURL, err := c.resolveDataURL(ctx)
	if err != nil {
		return cacheSnapshot{}, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dataURL, nil)
	if err != nil {
		return cacheSnapshot{}, "", fmt.Errorf("create vatsim data request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return cacheSnapshot{}, "", fmt.Errorf("fetch vatsim network data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cacheSnapshot{}, "", fmt.Errorf("fetch vatsim network data: unexpected status %d", resp.StatusCode)
	}

	var payload networkDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return cacheSnapshot{}, "", fmt.Errorf("decode vatsim network data: %w", err)
	}

	now := time.Now().UTC()
	snapshot := newCacheSnapshot(payload.General.UpdateTimestamp, now)
	for _, pilot := range payload.Pilots {
		snapshot.add(toFlight(pilot, FlightStateOnline))
	}
	for _, prefile := range payload.Prefiles {
		snapshot.add(toFlight(prefile, FlightStatePrefile))
	}
	return snapshot, dataURL, nil
}

func newCacheSnapshot(timestamp time.Time, receivedAt time.Time) cacheSnapshot {
	if timestamp.IsZero() {
		timestamp = receivedAt
	}
	return cacheSnapshot{
		timestamp:         timestamp.UTC(),
		flightsByCallsign: make(map[string]Flight),
		flightsByCID:      make(map[string]Flight),
	}
}

func (s *cacheSnapshot) add(flight Flight) {
	if flight.CID == "" || flight.Callsign == "" {
		return
	}

	if current, ok := s.flightsByCallsign[flight.Callsign]; ok && !preferFlight(flight, current) {
		return
	}
	s.flightsByCallsign[flight.Callsign] = flight
}

func preserveNewerFlightPlans(current, next cacheSnapshot) cacheSnapshot {
	for callsign, nextFlight := range next.flightsByCallsign {
		currentFlight, ok := current.flightsByCallsign[callsign]
		if !ok || currentFlight.CID != nextFlight.CID || currentFlight.FlightPlan.Revision <= nextFlight.FlightPlan.Revision {
			continue
		}
		nextFlight.FlightPlan = currentFlight.FlightPlan
		next.flightsByCallsign[callsign] = nextFlight
	}

	for callsign, flight := range next.flightsByCallsign {
		if current, ok := next.flightsByCID[flight.CID]; ok && !preferFlight(flight, current) {
			delete(next.flightsByCallsign, callsign)
			continue
		}
		next.flightsByCID[flight.CID] = flight
	}
	return next
}

func preferFlight(candidate, current Flight) bool {
	if candidate.Online() != current.Online() {
		return candidate.Online()
	}
	if candidate.FlightPlan.Revision != current.FlightPlan.Revision {
		return candidate.FlightPlan.Revision > current.FlightPlan.Revision
	}
	return candidate.LastUpdated.After(current.LastUpdated)
}

func toFlight(entry networkFlight, state FlightState) Flight {
	flight := Flight{
		CID:         fmt.Sprintf("%d", entry.CID),
		Callsign:    normalizeCallsign(entry.Callsign),
		State:       state,
		Latitude:    entry.Latitude,
		Longitude:   entry.Longitude,
		Altitude:    entry.Altitude,
		Groundspeed: entry.Groundspeed,
		LogonTime:   entry.LogonTime,
		LastUpdated: entry.LastUpdated,
	}
	if entry.FlightPlan == nil {
		return flight
	}
	flight.FlightPlan = FlightPlan{
		FlightRules:     entry.FlightPlan.FlightRules,
		Aircraft:        entry.FlightPlan.Aircraft,
		AircraftFAA:     entry.FlightPlan.AircraftFAA,
		AircraftShort:   entry.FlightPlan.AircraftShort,
		Origin:          entry.FlightPlan.Departure,
		Destination:     entry.FlightPlan.Arrival,
		Alternate:       entry.FlightPlan.Alternate,
		EOBT:            entry.FlightPlan.DepartureTime,
		EnrouteDuration: entry.FlightPlan.EnrouteTime,
		Remarks:         entry.FlightPlan.Remarks,
		Route:           entry.FlightPlan.Route,
		AssignedSquawk:  entry.FlightPlan.AssignedTransponder,
		Revision:        entry.FlightPlan.RevisionID,
	}
	return flight
}

func (c *Cache) recordRefreshError(err error) {
	c.mu.Lock()
	c.lastRefreshError = err
	c.mu.Unlock()
}

func (c *Cache) resolveDataURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.statusURL, nil)
	if err != nil {
		return "", fmt.Errorf("create vatsim status request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch vatsim status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch vatsim status: unexpected status %d", resp.StatusCode)
	}

	var payload statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode vatsim status: %w", err)
	}

	if len(payload.Data.V3) == 0 || strings.TrimSpace(payload.Data.V3[0]) == "" {
		return "", fmt.Errorf("vatsim status did not include a v3 data feed URL")
	}

	return payload.Data.V3[0], nil
}

func (c *Cache) getOnlinePilotByCID(cid string) (Flight, bool) {
	pilot, ok := c.Snapshot().FlightByCID(cid)
	return pilot, ok && pilot.Online()
}

func normalizeCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}
