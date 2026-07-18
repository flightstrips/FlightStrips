package testtools

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/vatsim"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	testSource  = "TEST_TOOLS"
	testCIDBase = int64(990000000)
)

var (
	ErrUnavailable = errors.New("SAT test tools are unavailable")
	ErrNotFound    = errors.New("test scenario not found")
	ErrConflict    = errors.New("test scenario conflicts with existing data")
	ErrInvalid     = errors.New("invalid test scenario request")
)

type ScenarioPreset string

const (
	PresetDeparture  ScenarioPreset = "departure"
	PresetArrival    ScenarioPreset = "arrival"
	PresetWrongStand ScenarioPreset = "wrong_stand"
)

type CreateScenarioRequest struct {
	SessionID     int32          `json:"session_id"`
	Preset        ScenarioPreset `json:"preset"`
	Callsign      string         `json:"callsign"`
	AircraftType  string         `json:"aircraft_type"`
	Origin        string         `json:"origin"`
	Destination   string         `json:"destination"`
	Route         string         `json:"route"`
	InitialState  string         `json:"initial_state"`
	EOBT          string         `json:"eobt"`
	EnrouteTime   string         `json:"enroute_time"`
	Altitude      int            `json:"altitude"`
	Groundspeed   int            `json:"groundspeed"`
	ObservedStand string         `json:"observed_stand"`
}

type ScenarioCommand struct {
	Command string `json:"command"`
	Minutes int    `json:"minutes,omitempty"`
	Stand   string `json:"stand,omitempty"`
}

type Scenario struct {
	ID               string          `json:"id"`
	SessionID        int32           `json:"session_id"`
	Preset           ScenarioPreset  `json:"preset"`
	Step             int             `json:"step"`
	Callsign         string          `json:"callsign"`
	CID              string          `json:"cid"`
	AircraftType     string          `json:"aircraft_type"`
	Origin           string          `json:"origin"`
	Destination      string          `json:"destination"`
	Route            string          `json:"route"`
	EOBT             string          `json:"eobt"`
	EnrouteTime      string          `json:"enroute_time"`
	FeedState        string          `json:"feed_state"`
	Altitude         int             `json:"altitude"`
	Groundspeed      int             `json:"groundspeed"`
	ObservedStand    string          `json:"observed_stand,omitempty"`
	StripBay         string          `json:"strip_bay,omitempty"`
	Assignment       *AssignmentView `json:"assignment,omitempty"`
	LastAction       string          `json:"last_action,omitempty"`
	GeneratedMessage string          `json:"generated_message,omitempty"`
	Error            string          `json:"error,omitempty"`
}

type AssignmentView struct {
	Stand          string     `json:"stand"`
	Direction      string     `json:"direction"`
	Stage          string     `json:"stage"`
	Source         string     `json:"source"`
	Version        int32      `json:"version"`
	ETA            *time.Time `json:"eta,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	ConflictReason *string    `json:"conflict_reason,omitempty"`
}

type SessionView struct {
	ID      int32  `json:"id"`
	Name    string `json:"name"`
	Airport string `json:"airport"`
}

type BlockView struct {
	ID        int64   `json:"id"`
	SessionID int32   `json:"session_id"`
	Stand     string  `json:"stand"`
	Reason    *string `json:"reason,omitempty"`
	Version   int32   `json:"version"`
}

type StripDeleter interface {
	DeleteStrip(context.Context, int32, string) error
}

type Service struct {
	source       *vatsim.SyntheticSource
	reconciler   *vatsim.Reconciler
	departures   *services.DepartureLifecycleService
	arrivals     *services.ArrivalLifecycleService
	allocations  *services.StandAllocationService
	sessions     repository.SessionRepository
	strips       repository.StripRepository
	stripDeleter StripDeleter
	assignments  repository.StandAssignmentRepository
	stands       *sat.StandCapabilityRegistry
	clock        *Clock

	operations sync.Mutex
	mu         sync.RWMutex
	scenarios  map[string]*Scenario
	nextCID    int64
}

type ServiceConfig struct {
	Source       *vatsim.SyntheticSource
	Reconciler   *vatsim.Reconciler
	Departures   *services.DepartureLifecycleService
	Arrivals     *services.ArrivalLifecycleService
	Allocations  *services.StandAllocationService
	Sessions     repository.SessionRepository
	Strips       repository.StripRepository
	StripDeleter StripDeleter
	Assignments  repository.StandAssignmentRepository
	Stands       *sat.StandCapabilityRegistry
	Clock        *Clock
}

func NewService(cfg ServiceConfig) *Service {
	return &Service{
		source: cfg.Source, reconciler: cfg.Reconciler, departures: cfg.Departures,
		arrivals: cfg.Arrivals, allocations: cfg.Allocations, sessions: cfg.Sessions,
		strips: cfg.Strips, stripDeleter: cfg.StripDeleter,
		assignments: cfg.Assignments, stands: cfg.Stands,
		clock: cfg.Clock, scenarios: make(map[string]*Scenario), nextCID: testCIDBase,
	}
}

func (s *Service) Available() bool {
	return s != nil && s.source != nil && s.reconciler != nil && s.departures != nil &&
		s.arrivals != nil && s.allocations != nil && s.sessions != nil &&
		s.strips != nil && s.stripDeleter != nil && s.assignments != nil &&
		s.stands != nil && s.clock != nil
}

func (s *Service) Now() time.Time {
	if s == nil || s.clock == nil {
		return time.Now().UTC()
	}
	return s.clock.Now()
}

func (s *Service) Sessions(ctx context.Context) ([]SessionView, error) {
	if s == nil || s.sessions == nil {
		return nil, ErrUnavailable
	}
	rows, err := s.sessions.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]SessionView, 0, len(rows))
	for _, session := range rows {
		if session != nil && strings.EqualFold(session.Airport, "EKCH") {
			result = append(result, SessionView{ID: session.ID, Name: session.Name, Airport: session.Airport})
		}
	}
	return result, nil
}

func (s *Service) CreateScenario(ctx context.Context, request CreateScenarioRequest) (*Scenario, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	s.operations.Lock()
	defer s.operations.Unlock()

	request = normalizeCreateRequest(request, s.Now())
	if err := validateCreateRequest(request); err != nil {
		return nil, err
	}
	session, err := s.sessions.GetByID(ctx, request.SessionID)
	if err != nil || session == nil || !strings.EqualFold(session.Airport, "EKCH") {
		return nil, fmt.Errorf("%w: select an existing EKCH session", ErrInvalid)
	}
	exists, err := s.callsignExists(ctx, request.Callsign)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("%w: callsign %s already exists", ErrConflict, request.Callsign)
	}

	s.mu.Lock()
	for _, current := range s.scenarios {
		if current.Callsign == request.Callsign {
			s.mu.Unlock()
			return nil, fmt.Errorf("%w: callsign %s already has a scenario", ErrConflict, request.Callsign)
		}
	}
	s.nextCID++
	scenario := &Scenario{
		ID: uuid.NewString(), SessionID: request.SessionID, Preset: request.Preset,
		Callsign: request.Callsign, CID: strconv.FormatInt(s.nextCID, 10),
		AircraftType: request.AircraftType, Origin: request.Origin,
		Destination: request.Destination, Route: request.Route,
		EOBT: request.EOBT, EnrouteTime: request.EnrouteTime,
		FeedState: request.InitialState, Altitude: request.Altitude,
		Groundspeed: request.Groundspeed, ObservedStand: request.ObservedStand,
		LastAction: "scenario created",
	}
	s.scenarios[scenario.ID] = scenario
	s.mu.Unlock()

	flight := s.flightForScenario(scenario)
	s.source.Upsert(flight)
	if err := s.reconcileScenario(ctx, scenario); err != nil {
		s.source.Remove(scenario.Callsign)
		s.mu.Lock()
		delete(s.scenarios, scenario.ID)
		s.mu.Unlock()
		return nil, err
	}
	if err := s.markAssignmentOwned(ctx, scenario); err != nil {
		_ = s.cleanupScenario(ctx, scenario)
		s.mu.Lock()
		delete(s.scenarios, scenario.ID)
		s.mu.Unlock()
		return nil, err
	}
	return s.refreshScenario(ctx, scenario), nil
}

func (s *Service) callsignExists(ctx context.Context, callsign string) (bool, error) {
	if _, ok := s.source.Snapshot().FlightByCallsign(callsign); ok {
		return true, nil
	}
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return false, err
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		strip, err := s.strips.GetByCallsign(ctx, session.ID, callsign)
		switch {
		case err == nil && strip != nil:
			return true, nil
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return false, err
		}
	}
	return false, nil
}

func normalizeCreateRequest(request CreateScenarioRequest, now time.Time) CreateScenarioRequest {
	request.Preset = ScenarioPreset(strings.ToLower(strings.TrimSpace(string(request.Preset))))
	request.Callsign = strings.ToUpper(strings.TrimSpace(request.Callsign))
	request.AircraftType = strings.ToUpper(strings.TrimSpace(request.AircraftType))
	request.Origin = strings.ToUpper(strings.TrimSpace(request.Origin))
	request.Destination = strings.ToUpper(strings.TrimSpace(request.Destination))
	request.Route = strings.ToUpper(strings.TrimSpace(request.Route))
	request.InitialState = strings.ToLower(strings.TrimSpace(request.InitialState))
	request.ObservedStand = strings.ToUpper(strings.TrimSpace(request.ObservedStand))
	if request.Preset == "" {
		request.Preset = PresetDeparture
	}
	if request.Callsign == "" {
		request.Callsign = "TST" + now.Format("150405")
	}
	if request.AircraftType == "" {
		request.AircraftType = "A320"
	}
	if request.Route == "" {
		request.Route = "DCT"
	}
	if request.InitialState == "" {
		request.InitialState = "prefile"
	}
	if request.EOBT == "" {
		request.EOBT = now.UTC().Format("1504")
	}
	if request.EnrouteTime == "" {
		request.EnrouteTime = "0045"
	}
	if request.Preset == PresetArrival {
		if request.Origin == "" {
			request.Origin = "EGLL"
		}
		if request.Destination == "" {
			request.Destination = "EKCH"
		}
	} else {
		if request.Origin == "" {
			request.Origin = "EKCH"
		}
		if request.Destination == "" {
			request.Destination = "EGLL"
		}
	}
	return request
}

func validateCreateRequest(request CreateScenarioRequest) error {
	switch request.Preset {
	case PresetDeparture, PresetArrival, PresetWrongStand:
	default:
		return fmt.Errorf("%w: unsupported preset %q", ErrInvalid, request.Preset)
	}
	if request.SessionID <= 0 || request.Callsign == "" || request.AircraftType == "" ||
		request.Origin == "" || request.Destination == "" {
		return fmt.Errorf("%w: session, callsign, aircraft, origin, and destination are required", ErrInvalid)
	}
	if request.InitialState != "prefile" && request.InitialState != "online" {
		return fmt.Errorf("%w: initial_state must be prefile or online", ErrInvalid)
	}
	if _, err := time.Parse("1504", request.EOBT); err != nil {
		return fmt.Errorf("%w: eobt must use HHMM", ErrInvalid)
	}
	if len(request.EnrouteTime) != 4 {
		return fmt.Errorf("%w: enroute_time must use HHMM", ErrInvalid)
	}
	hours, hoursErr := strconv.Atoi(request.EnrouteTime[:2])
	minutes, minutesErr := strconv.Atoi(request.EnrouteTime[2:])
	if hoursErr != nil || minutesErr != nil || hours < 0 || minutes < 0 || minutes >= 60 {
		return fmt.Errorf("%w: enroute_time must use HHMM", ErrInvalid)
	}
	if request.Altitude < 0 || request.Groundspeed < 0 {
		return fmt.Errorf("%w: altitude and groundspeed cannot be negative", ErrInvalid)
	}
	return nil
}

func (s *Service) flightForScenario(scenario *Scenario) vatsim.Flight {
	snapshot := s.snapshotScenario(scenario)
	state := vatsim.FlightStatePrefile
	if snapshot.FeedState == "online" {
		state = vatsim.FlightStateOnline
	}
	return vatsim.Flight{
		CID: snapshot.CID, Callsign: snapshot.Callsign, State: state,
		Altitude: snapshot.Altitude, Groundspeed: snapshot.Groundspeed,
		LastUpdated: s.Now(),
		FlightPlan: vatsim.FlightPlan{
			FlightRules: "I", Aircraft: snapshot.AircraftType,
			AircraftShort: snapshot.AircraftType, Origin: snapshot.Origin,
			Destination: snapshot.Destination, Route: snapshot.Route,
			EOBT: snapshot.EOBT, EnrouteDuration: snapshot.EnrouteTime, Revision: int64(snapshot.Step + 1),
		},
	}
}

func (s *Service) snapshotScenario(scenario *Scenario) Scenario {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *scenario
}

func (s *Service) updateScenario(scenario *Scenario, update func(*Scenario)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(scenario)
}

func (s *Service) ListScenarios(ctx context.Context, sessionID int32) ([]*Scenario, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	s.mu.RLock()
	selected := make([]*Scenario, 0, len(s.scenarios))
	for _, scenario := range s.scenarios {
		if sessionID == 0 || scenario.SessionID == sessionID {
			selected = append(selected, scenario)
		}
	}
	s.mu.RUnlock()
	result := make([]*Scenario, 0, len(selected))
	for _, scenario := range selected {
		result = append(result, s.refreshScenario(ctx, scenario))
	}
	return result, nil
}

func (s *Service) Command(ctx context.Context, id string, command ScenarioCommand) (*Scenario, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	s.operations.Lock()
	defer s.operations.Unlock()

	s.mu.RLock()
	scenario := s.scenarios[id]
	s.mu.RUnlock()
	if scenario == nil {
		return nil, ErrNotFound
	}

	var err error
	switch strings.ToLower(strings.TrimSpace(command.Command)) {
	case "advance":
		err = s.advance(ctx, scenario)
	case "advance_time":
		if command.Minutes <= 0 || command.Minutes > 24*60 {
			return nil, fmt.Errorf("%w: minutes must be between 1 and 1440", ErrInvalid)
		}
		s.clock.Advance(time.Duration(command.Minutes) * time.Minute)
		err = s.runSweeps(ctx)
		s.updateScenario(scenario, func(state *Scenario) {
			state.LastAction = fmt.Sprintf("advanced simulated time by %d minutes", command.Minutes)
		})
	case "move_to_stand":
		err = s.moveToStand(ctx, scenario, command.Stand)
	case "remove":
		snapshot := s.snapshotScenario(scenario)
		s.source.Remove(snapshot.Callsign)
		s.updateScenario(scenario, func(state *Scenario) {
			state.FeedState = "removed"
			state.LastAction = "removed from synthetic feed"
		})
		err = s.reconcileScenario(ctx, scenario)
	default:
		return nil, fmt.Errorf("%w: unsupported command %q", ErrInvalid, command.Command)
	}
	if err != nil {
		s.updateScenario(scenario, func(state *Scenario) { state.Error = err.Error() })
		return s.refreshScenario(ctx, scenario), err
	}
	if err := s.markAssignmentOwned(ctx, scenario); err != nil {
		s.updateScenario(scenario, func(state *Scenario) { state.Error = err.Error() })
		return s.refreshScenario(ctx, scenario), err
	}
	s.updateScenario(scenario, func(state *Scenario) { state.Error = "" })
	return s.refreshScenario(ctx, scenario), nil
}

func (s *Service) markAssignmentOwned(ctx context.Context, scenario *Scenario) error {
	assignment, err := s.assignments.GetAssignment(ctx, scenario.SessionID, scenario.Callsign)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil || assignment == nil {
		return err
	}
	if assignment.Source == testSource {
		return nil
	}
	assignment.Source = testSource
	updated, err := s.assignments.UpdateAssignment(ctx, assignment)
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("test-tools assignment changed concurrently")
	}
	assignment.Version++
	return s.allocations.PublishAssignment(ctx, *assignment)
}

func (s *Service) advance(ctx context.Context, scenario *Scenario) error {
	snapshot := s.snapshotScenario(scenario)
	switch snapshot.Preset {
	case PresetDeparture:
		switch snapshot.Step {
		case 0:
			assignment, err := s.assignments.GetAssignment(ctx, snapshot.SessionID, snapshot.Callsign)
			if err != nil {
				return err
			}
			if err := s.moveToStand(ctx, scenario, assignment.Stand); err != nil {
				return err
			}
		case 1:
			s.source.Remove(snapshot.Callsign)
			s.updateScenario(scenario, func(state *Scenario) {
				state.FeedState = "removed"
				state.LastAction = "departure removed from feed"
			})
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		default:
			if strip, err := s.strips.GetByCallsign(ctx, snapshot.SessionID, snapshot.Callsign); err == nil && strip != nil && strip.VatsimCID != nil && isTestCID(*strip.VatsimCID) {
				if err := s.stripDeleter.DeleteStrip(ctx, snapshot.SessionID, snapshot.Callsign); err != nil {
					return err
				}
			}
			s.clock.Advance(20 * time.Minute)
			s.updateScenario(scenario, func(state *Scenario) { state.LastAction = "departure expiry swept" })
			if err := s.runSweeps(ctx); err != nil {
				return err
			}
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		}
	case PresetArrival:
		switch snapshot.Step {
		case 0:
			if err := s.advanceToETAWindow(ctx, snapshot, 9*time.Minute); err != nil {
				return err
			}
			s.updateScenario(scenario, func(state *Scenario) {
				state.FeedState, state.Altitude, state.Groundspeed = "online", 9000, 250
				state.LastAction = "arrival advanced to ASSIGNED threshold"
			})
			s.source.Upsert(s.flightForScenario(scenario))
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		case 1:
			if err := s.advanceToETAWindow(ctx, snapshot, time.Minute); err != nil {
				return err
			}
			s.updateScenario(scenario, func(state *Scenario) {
				state.Altitude = 2000
				state.LastAction = "arrival advanced to CONFIRMED threshold"
			})
			s.source.Upsert(s.flightForScenario(scenario))
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		default:
			s.source.Remove(snapshot.Callsign)
			s.clock.Advance(32 * time.Minute)
			s.updateScenario(scenario, func(state *Scenario) {
				state.FeedState = "removed"
				state.LastAction = "arrival timeout swept"
			})
			if err := s.runSweeps(ctx); err != nil {
				return err
			}
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		}
	case PresetWrongStand:
		switch snapshot.Step {
		case 0:
			assignment, err := s.assignments.GetAssignment(ctx, snapshot.SessionID, snapshot.Callsign)
			if err != nil {
				return err
			}
			stand, err := s.pickWrongStand(assignment.Stand, snapshot.ObservedStand)
			if err != nil {
				return err
			}
			reason := "test-tools wrong-stand scenario"
			callsign := snapshot.Callsign
			block := &models.StandBlock{SessionID: snapshot.SessionID, Stand: stand.Name, BlockType: "CLOSURE", Source: testSource, Reason: &reason, Callsign: &callsign, Manual: true}
			if err := s.allocations.CreateManualBlock(ctx, "EKCH", block); err != nil {
				return err
			}
			s.updateScenario(scenario, func(state *Scenario) { state.ObservedStand = stand.Name })
			if err := s.moveToStand(ctx, scenario, stand.Name); err != nil {
				return err
			}
		case 1:
			s.clock.Advance(6 * time.Minute)
			s.updateScenario(scenario, func(state *Scenario) { state.LastAction = "wrong-stand assignment retained" })
			if err := s.departures.ReleaseExpired(ctx); err != nil {
				return err
			}
		default:
			s.source.Remove(snapshot.Callsign)
			s.updateScenario(scenario, func(state *Scenario) {
				state.FeedState = "removed"
				state.LastAction = "wrong-stand flight removed"
			})
			if err := s.reconcileScenario(ctx, scenario); err != nil {
				return err
			}
		}
	}
	s.updateScenario(scenario, func(state *Scenario) { state.Step++ })
	return nil
}

func (s *Service) advanceToETAWindow(ctx context.Context, scenario Scenario, window time.Duration) error {
	assignment, err := s.assignments.GetAssignment(ctx, scenario.SessionID, scenario.Callsign)
	if err != nil {
		return err
	}
	if assignment.ETA == nil {
		return fmt.Errorf("%w: scenario has no arrival ETA", ErrInvalid)
	}
	if advance := assignment.ETA.Add(-window).Sub(s.Now()); advance > 0 {
		s.clock.Advance(advance)
	}
	return nil
}

func (s *Service) pickWrongStand(assigned, preferred string) (sat.Stand, error) {
	if preferred != "" && !strings.EqualFold(preferred, assigned) {
		if stand, ok := s.stands.Lookup("EKCH", preferred); ok {
			return stand, nil
		}
	}
	for _, stand := range s.stands.Stands("EKCH") {
		if !strings.EqualFold(stand.Name, assigned) && !stand.Variants[0].Manual {
			return stand, nil
		}
	}
	return sat.Stand{}, fmt.Errorf("%w: no alternate stand available", ErrInvalid)
}

func (s *Service) moveToStand(ctx context.Context, scenario *Scenario, name string) error {
	stand, ok := s.stands.Lookup("EKCH", strings.ToUpper(strings.TrimSpace(name)))
	if !ok {
		return fmt.Errorf("%w: unknown stand %q", ErrInvalid, name)
	}
	flight := s.flightForScenario(scenario)
	flight.State = vatsim.FlightStateOnline
	flight.Latitude, flight.Longitude = stand.Latitude, stand.Longitude
	snapshot := s.snapshotScenario(scenario)
	if flight.Altitude == 0 && snapshot.Preset == PresetArrival {
		flight.Altitude = 9000
	}
	s.updateScenario(scenario, func(state *Scenario) {
		state.FeedState = "online"
		state.ObservedStand = stand.Name
		state.LastAction = "moved online flight to stand " + stand.Name
		if state.Altitude == 0 && state.Preset == PresetArrival {
			state.Altitude = 9000
		}
	})
	s.source.Upsert(flight)
	if err := s.reconcileScenario(ctx, scenario); err != nil {
		return err
	}
	if !strings.EqualFold(snapshot.Origin, "EKCH") {
		return nil
	}
	strip, err := s.strips.GetByCallsign(ctx, snapshot.SessionID, snapshot.Callsign)
	if err != nil {
		return err
	}
	return s.departures.ObserveDeparturePosition(ctx, snapshot.SessionID, strip, stand.Latitude, stand.Longitude)
}

func (s *Service) reconcileScenario(ctx context.Context, scenario *Scenario) error {
	return s.reconciler.ReconcileSession(ctx, s.snapshotScenario(scenario).SessionID)
}

func (s *Service) runSweeps(ctx context.Context) error {
	if err := s.departures.ReleaseExpired(ctx); err != nil {
		return err
	}
	return s.arrivals.ReleaseExpired(ctx)
}

func (s *Service) refreshScenario(ctx context.Context, scenario *Scenario) *Scenario {
	copy := s.snapshotScenario(scenario)
	strip, err := s.strips.GetByCallsign(ctx, copy.SessionID, copy.Callsign)
	if err == nil && strip != nil {
		copy.StripBay = strip.Bay
	}
	assignment, err := s.assignments.GetAssignment(ctx, copy.SessionID, copy.Callsign)
	if err == nil && assignment != nil {
		copy.Assignment = &AssignmentView{
			Stand: assignment.Stand, Direction: assignment.Direction, Stage: assignment.Stage,
			Source: assignment.Source, Version: assignment.Version, ETA: assignment.ETA,
			ExpiresAt: assignment.ExpiresAt, ConflictReason: assignment.ConflictReason,
		}
	}
	return &copy
}

func (s *Service) DeleteScenario(ctx context.Context, id string) error {
	s.operations.Lock()
	defer s.operations.Unlock()

	s.mu.RLock()
	scenario := s.scenarios[id]
	s.mu.RUnlock()
	if scenario == nil {
		return ErrNotFound
	}
	if err := s.cleanupScenario(ctx, scenario); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.scenarios, id)
	s.mu.Unlock()
	return nil
}

func (s *Service) cleanupScenario(ctx context.Context, scenario *Scenario) error {
	s.source.Remove(scenario.Callsign)
	if assignment, err := s.assignments.GetAssignment(ctx, scenario.SessionID, scenario.Callsign); err == nil && assignment != nil {
		if err := s.allocations.ReleaseAssignment(ctx, assignment); err != nil {
			return err
		}
	}
	blocks, err := s.assignments.ListBlocks(ctx, scenario.SessionID)
	if err != nil {
		return err
	}
	for _, block := range blocks {
		if block != nil && block.Source == testSource && block.Callsign != nil && strings.EqualFold(*block.Callsign, scenario.Callsign) {
			if _, err := s.allocations.DeleteManualBlock(ctx, scenario.SessionID, block.ID, block.Version); err != nil {
				return err
			}
		}
	}
	if strip, err := s.strips.GetByCallsign(ctx, scenario.SessionID, scenario.Callsign); err == nil && strip != nil {
		if strip.VatsimCID != nil && isTestCID(*strip.VatsimCID) {
			if err := s.stripDeleter.DeleteStrip(ctx, scenario.SessionID, scenario.Callsign); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Reset(ctx context.Context) error {
	if !s.Available() {
		return ErrUnavailable
	}
	s.operations.Lock()
	defer s.operations.Unlock()

	s.mu.RLock()
	scenarios := make([]*Scenario, 0, len(s.scenarios))
	for _, scenario := range s.scenarios {
		scenarios = append(scenarios, scenario)
	}
	s.mu.RUnlock()
	for _, scenario := range scenarios {
		if err := s.cleanupScenario(ctx, scenario); err != nil {
			return err
		}
	}
	s.source.Reset()
	s.clock.Reset()
	s.mu.Lock()
	s.scenarios = make(map[string]*Scenario)
	s.mu.Unlock()
	return s.cleanupOrphans(ctx)
}

func (s *Service) cleanupOrphans(ctx context.Context) error {
	sessions, err := s.sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		strips, err := s.strips.List(ctx, session.ID)
		if err != nil {
			return err
		}
		for _, strip := range strips {
			if strip == nil || strip.VatsimCID == nil || !isTestCID(*strip.VatsimCID) {
				continue
			}
			if assignment, err := s.assignments.GetAssignment(ctx, session.ID, strip.Callsign); err == nil && assignment != nil {
				if err := s.allocations.ReleaseAssignment(ctx, assignment); err != nil {
					return err
				}
			}
			if err := s.stripDeleter.DeleteStrip(ctx, session.ID, strip.Callsign); err != nil {
				return err
			}
		}
		blocks, err := s.assignments.ListBlocks(ctx, session.ID)
		if err != nil {
			return err
		}
		for _, block := range blocks {
			if block != nil && block.Source == testSource {
				if _, err := s.allocations.DeleteManualBlock(ctx, session.ID, block.ID, block.Version); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isTestCID(value string) bool {
	cid, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && cid > testCIDBase && cid < testCIDBase+1000000
}

func (s *Service) CreateBlock(ctx context.Context, sessionID int32, stand, reason string) (*BlockView, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	session, err := s.sessions.GetByID(ctx, sessionID)
	if err != nil || session == nil || !strings.EqualFold(session.Airport, "EKCH") {
		return nil, fmt.Errorf("%w: select an existing EKCH session", ErrInvalid)
	}
	stand = strings.ToUpper(strings.TrimSpace(stand))
	if _, ok := s.stands.Lookup("EKCH", stand); !ok {
		return nil, fmt.Errorf("%w: unknown stand %q", ErrInvalid, stand)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "Local test console"
	}
	block := &models.StandBlock{SessionID: sessionID, Stand: stand, BlockType: "CLOSURE", Source: testSource, Reason: &reason, Manual: true}
	if err := s.allocations.CreateManualBlock(ctx, "EKCH", block); err != nil {
		return nil, err
	}
	return &BlockView{ID: block.ID, SessionID: sessionID, Stand: block.Stand, Reason: block.Reason, Version: block.Version}, nil
}

func (s *Service) DeleteBlock(ctx context.Context, sessionID int32, id int64, version int32) error {
	if !s.Available() {
		return ErrUnavailable
	}
	block, err := s.assignments.GetBlock(ctx, sessionID, id)
	if err != nil {
		return err
	}
	if block.Source != testSource {
		return fmt.Errorf("%w: only test-tools blocks can be removed here", ErrConflict)
	}
	_, err = s.allocations.DeleteManualBlock(ctx, sessionID, id, version)
	return err
}

func (s *Service) Blocks(ctx context.Context, sessionID int32) ([]BlockView, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	rows, err := s.assignments.ListBlocks(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]BlockView, 0)
	for _, block := range rows {
		if block != nil && block.Source == testSource {
			result = append(result, BlockView{ID: block.ID, SessionID: block.SessionID, Stand: block.Stand, Reason: block.Reason, Version: block.Version})
		}
	}
	return result, nil
}

// RecordGeneratedMessage lets the wrong-stand fallback messenger expose the
// message on the scenario card.
func (s *Service) RecordGeneratedMessage(callsign, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, scenario := range s.scenarios {
		if strings.EqualFold(scenario.Callsign, callsign) {
			scenario.GeneratedMessage = message
		}
	}
}

type privateMessenger interface {
	SendPrivateMessageFromDelivery(session int32, callsign, message string) bool
}

type FallbackMessenger struct {
	primary  privateMessenger
	recorder *Service
}

func NewFallbackMessenger(primary privateMessenger, recorder *Service) *FallbackMessenger {
	return &FallbackMessenger{primary: primary, recorder: recorder}
}

func (m *FallbackMessenger) SendPrivateMessageFromDelivery(session int32, callsign, message string) bool {
	delivered := m.primary != nil && m.primary.SendPrivateMessageFromDelivery(session, callsign, message)
	if m.recorder != nil {
		m.recorder.RecordGeneratedMessage(callsign, message)
		return true
	}
	return delivered
}
