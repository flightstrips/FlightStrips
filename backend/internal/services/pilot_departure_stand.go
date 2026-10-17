package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	PilotStandAssigned      = "ASSIGNED"
	PilotStandSelected      = "SELECTED"
	PilotStandConfirmed     = "CONFIRMED"
	PilotStandMismatch      = "MISMATCH"
	PilotStandOverwritten   = "OVERWRITTEN"
	PilotStandExpired       = "EXPIRED"
	PilotStandNotDetected   = "NO STAND DETECTED"
	pilotStandHold          = 15 * time.Minute
	pilotStandMismatchDelay = 5 * time.Minute
)

// PilotDepartureStand persists the EFB episode separately from the allocator's
// reservation. In particular, EXPIRED retains its display after releasing space.
type PilotDepartureStand struct {
	CID           string     `json:"cid"`
	Origin        string     `json:"origin"`
	Destination   string     `json:"destination"`
	Stand         string     `json:"stand"`
	State         string     `json:"state"`
	Online        bool       `json:"online"`
	ObservedStand string     `json:"observed_stand"`
	MismatchSince *time.Time `json:"mismatch_since,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Version       int32      `json:"version"`
}

type PilotDepartureStandService struct {
	lifecycle *DepartureLifecycleService
}

func NewPilotDepartureStandService(lifecycle *DepartureLifecycleService) *PilotDepartureStandService {
	if lifecycle == nil {
		return nil
	}
	service := &PilotDepartureStandService{lifecycle: lifecycle}
	lifecycle.pilotStands = service
	return service
}

func (s *PilotDepartureStandService) Open(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) (*PilotDepartureStand, error) {
	return s.run(ctx, session, strip, flight, true, "", nil, false)
}

func (s *PilotDepartureStandService) read(ctx context.Context, session int32, callsign string) (*PilotDepartureStand, error) {
	var encoded []byte
	err := s.lifecycle.allocations.pool.QueryRow(ctx,
		"SELECT state FROM efb_departure_stands WHERE session_id=$1 AND callsign=$2", session, callsign).Scan(&encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state PilotDepartureStand
	if err := json.Unmarshal(encoded, &state); err != nil {
		return nil, fmt.Errorf("decode pilot stand state: %w", err)
	}
	return &state, nil
}

func (s *PilotDepartureStandService) Read(ctx context.Context, session int32, flight vatsim.DepartureFlightInfo) (*PilotDepartureStand, error) {
	state, err := s.read(ctx, session, flight.Callsign)
	if err != nil || state == nil {
		return state, err
	}
	if state.CID != flight.CID || !strings.EqualFold(state.Origin, flight.Origin) || !strings.EqualFold(state.Destination, flight.Destination) {
		return nil, nil
	}
	if !flight.Online && state.ExpiresAt != nil && !state.ExpiresAt.After(s.lifecycle.now()) {
		state.State, state.Version = PilotStandExpired, 0
	}
	return state, nil
}

func (s *PilotDepartureStandService) Select(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo, stand string, version int32, allowUnsuitable bool) (*PilotDepartureStand, error) {
	if flight.Online {
		return nil, errors.New("online departures follow their detected stand; manual selection is prefile-only")
	}
	if strings.TrimSpace(stand) == "" {
		return nil, errors.New("stand is required")
	}
	return s.run(ctx, session, strip, flight, false, standName(stand), &version, allowUnsuitable)
}

func (s *PilotDepartureStandService) opened(ctx context.Context, session int32, strip *models.Strip) (bool, error) {
	state, err := s.read(ctx, session, strip.Callsign)
	if err != nil || state == nil {
		return false, err
	}
	return (strip.VatsimCID == nil || *strip.VatsimCID == state.CID) &&
		strings.EqualFold(strip.Origin, state.Origin) && strings.EqualFold(strip.Destination, state.Destination), nil
}

func (s *PilotDepartureStandService) cancelDeparture(ctx context.Context, session int32, callsign string) error {
	state, err := s.read(ctx, session, callsign)
	if err != nil || state == nil {
		return err
	}
	strip, err := s.lifecycle.strips.GetByCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	if strip.VatsimCID != nil && *strip.VatsimCID != state.CID ||
		!strings.EqualFold(strip.Origin, state.Origin) || !strings.EqualFold(strip.Destination, state.Destination) {
		return nil
	}
	_, err = s.run(ctx, session, strip, vatsim.DepartureFlightInfo{
		Callsign: callsign, CID: state.CID, Origin: state.Origin,
		Destination: state.Destination, AircraftType: valueString(strip.AircraftType),
	}, false, "", nil, false)
	return err
}

func (s *PilotDepartureStandService) reconcile(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo) (bool, error) {
	state, err := s.Read(ctx, session, flight)
	if err != nil || state == nil {
		return state != nil, err
	}
	_, err = s.run(ctx, session, strip, flight, false, "", nil, false)
	return true, err
}

func (s *PilotDepartureStandService) run(ctx context.Context, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo, open bool, manual string, version *int32, allowUnsuitable bool) (*PilotDepartureStand, error) {
	if strip == nil || flight.CID == "" || !strings.EqualFold(strip.Callsign, flight.Callsign) {
		return nil, errors.New("pilot stand assignment requires an identified flight")
	}
	var state *PilotDepartureStand
	var result *StandAllocationResult
	err := retrySerializableOperation(ctx, func() error {
		state, result = nil, nil
		tx, err := s.lifecycle.allocations.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		var name, airport string
		if err := tx.QueryRow(ctx, "SELECT name, airport FROM sessions WHERE id=$1 FOR UPDATE", session).Scan(&name, &airport); err != nil {
			return err
		}
		if !strings.EqualFold(name, "LIVE") || !strings.EqualFold(airport, strip.Origin) {
			return errors.New("pilot departure stand assignment requires a matching LIVE airport session")
		}
		lockedStrip, err := s.lifecycle.strips.WithTx(tx).LockByCallsign(ctx, session, strip.Callsign)
		if err != nil {
			return err
		}
		if !strings.EqualFold(lockedStrip.Origin, flight.Origin) || !strings.EqualFold(lockedStrip.Destination, flight.Destination) {
			return errors.New("stand assignment flight plan has changed")
		}
		var encoded []byte
		err = tx.QueryRow(ctx, "SELECT state FROM efb_departure_stands WHERE session_id=$1 AND callsign=$2",
			session, strip.Callsign).Scan(&encoded)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			state = &PilotDepartureStand{}
			if err := json.Unmarshal(encoded, state); err != nil {
				return fmt.Errorf("decode pilot stand state: %w", err)
			}
			if state.CID != flight.CID || !strings.EqualFold(state.Origin, flight.Origin) || !strings.EqualFold(state.Destination, flight.Destination) {
				state = nil
			}
		}
		if state == nil {
			if !open {
				return errors.New("open the EFB flight before selecting a stand")
			}
			state = &PilotDepartureStand{CID: flight.CID, Origin: flight.Origin, Destination: flight.Destination}
		}
		result, err = s.advance(ctx, tx, session, lockedStrip, flight, state, manual, version, allowUnsuitable)
		if err != nil {
			return err
		}
		encoded, err = json.Marshal(state)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO efb_departure_stands (session_id, callsign, state) VALUES ($1,$2,$3)
			ON CONFLICT (session_id, callsign) DO UPDATE SET state=EXCLUDED.state`, session, strip.Callsign, encoded); err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return nil, err
	}
	if result != nil {
		ctx = beginRelocationChain(ctx, strip.Callsign)
		s.lifecycle.allocations.publishCommitted(ctx, *result)
		s.lifecycle.allocations.relocateDisplacedArrivals(ctx, result.RemovedAssignments, "EFB stand overwrite")
	}
	return state, nil
}

func (s *PilotDepartureStandService) advance(ctx context.Context, tx pgx.Tx, session int32, strip *models.Strip, flight vatsim.DepartureFlightInfo, state *PilotDepartureStand, manual string, version *int32, allowUnsuitable bool) (*StandAllocationResult, error) {
	lifecycle := s.lifecycle
	now := lifecycle.now().UTC()
	store := lifecycle.assignments.WithTx(tx)
	existing, err := store.GetAssignment(ctx, session, strip.Callsign)
	if err != nil && !isNotFound(err) {
		return nil, err
	}
	if version != nil && (existing == nil && *version != 0 || existing != nil && existing.Version != *version && !(standAssignmentExpired(existing, now) && *version == 0)) {
		return nil, ErrStandActionStaleVersion
	}
	state.Version = 0
	if existing != nil {
		state.Version = existing.Version
	}
	request := lifecycle.buildRequest(session, strip, flight, StageReserved, nil)
	request.ProtectReservations = true
	request.AllowUnsuitable = allowUnsuitable
	allocate := func(command StandAllocationCommand) (*StandAllocationResult, error) {
		if err := validateStandAllocationRequest(command, &request); err != nil {
			return nil, err
		}
		result, _, err := lifecycle.allocations.allocateInTransaction(ctx, tx, command, request, nil, 1)
		if err != nil {
			return nil, err
		}
		state.Stand, state.Version = result.Assignment.Stand, result.Assignment.Version
		return result, nil
	}
	release := func() (*StandAllocationResult, error) {
		state.Version = 0
		if existing == nil {
			return nil, nil
		}
		deleted, err := store.DeleteAssignment(ctx, session, existing.ID, existing.Version)
		if err != nil {
			return nil, err
		}
		if deleted != 1 {
			return nil, ErrStandActionStaleVersion
		}
		return &StandAllocationResult{Assignment: *existing, Removed: true}, nil
	}
	if !flight.Online {
		state.Online, state.ObservedStand, state.MismatchSince = false, "", nil
		if manual == "" && state.State != "" {
			if state.ExpiresAt == nil || !state.ExpiresAt.After(now) {
				state.State = PilotStandExpired
				return release()
			}
			return nil, nil
		}
		if manual != "" && state.Stand == manual && state.ExpiresAt != nil && state.ExpiresAt.After(now) {
			return nil, nil
		}
		expires := now.Add(pilotStandHold)
		request.ExpiresAt = &expires
		command := AutomaticStandAllocation
		state.State = PilotStandAssigned
		if manual != "" {
			command, request.Stand, state.State = CompatibleManualStand, manual, PilotStandSelected
		}
		result, err := allocate(command)
		state.ExpiresAt = &expires
		return result, err
	}
	state.Online, state.ExpiresAt = true, nil
	observed, found := lifecycle.stands.StandAtPosition(strip.Origin, flight.Latitude, flight.Longitude)
	if !found {
		state.State, state.Stand, state.ObservedStand, state.MismatchSince = PilotStandNotDetected, "", "", nil
		return release()
	}
	if state.ObservedStand != observed.Name {
		state.MismatchSince = nil
	}
	state.ObservedStand = observed.Name
	request.Stage, request.Stand, request.ObservedStand = StageDepartureBlock, observed.Name, &observed.Name
	// No projected release may make a physically occupied stand look available.
	request.DepartureTOBT, request.DepartureTSAT = nil, nil
	overwrite := state.MismatchSince != nil && !now.Before(state.MismatchSince.Add(pilotStandMismatchDelay))
	if overwrite {
		request.DisplaceArrivalStages = []string{StageEstimated, StageAssigned, StageConfirmed}
	}
	// A completed claim remains stable, but recheck conflicts on every feed poll.
	if existing != nil && existing.Stand == observed.Name &&
		(state.State == PilotStandConfirmed || state.State == PilotStandOverwritten) {
		assignments, err := store.LockAssignments(ctx, session, strip.Callsign)
		if err != nil {
			return nil, err
		}
		blocks, err := store.LockActiveManualBlocks(ctx, session)
		if err != nil {
			return nil, err
		}
		physical, _ := lifecycle.stands.Lookup(strip.Origin, observed.Name)
		matches := map[string]sat.StandCompatibilityMatch{observed.Name: {Stand: physical, Blocks: physical.Blocks}}
		if len(lifecycle.allocations.availability(request, assignments, blocks, matches)[observed.Name]) == 0 {
			return nil, nil
		}
	}
	result, err := allocate(pilotObservedStandAllocation)
	if err == nil {
		state.State = PilotStandConfirmed
		if overwrite && len(result.RemovedAssignments) > 0 {
			state.State = PilotStandOverwritten
		}
		state.MismatchSince = nil
		return result, nil
	}
	if !errors.Is(err, ErrIncompatibleManualAssignment) {
		return nil, err
	}
	if state.MismatchSince == nil {
		state.MismatchSince = &now
	}
	state.State = PilotStandMismatch
	if existing != nil && existing.Stand != observed.Name && !standAssignmentExpired(existing, now) {
		state.Stand = existing.Stand
		if existing.ExpiresAt != nil {
			existing.ExpiresAt, existing.ProjectedReleaseAt = nil, nil
			existing.Stage = StageDepartureBlock
			affected, err := store.UpdateAssignment(ctx, existing)
			if err != nil {
				return nil, err
			}
			if affected != 1 {
				return nil, ErrStandActionStaleVersion
			}
			existing.Version++
			state.Version = existing.Version
			return &StandAllocationResult{Assignment: *existing}, nil
		}
		return nil, nil
	}
	request.Stand, request.ObservedStand, request.DisplaceArrivalStages = "", nil, nil
	result, err = allocate(AutomaticStandAllocation)
	return result, err
}
