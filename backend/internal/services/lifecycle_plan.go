package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
)

// LifecyclePlan is an isolated domain snapshot, never a persistence adapter.
// The caller owns cloning input and atomically committing the resulting diff.
// It shares the production lifecycle and SAT selection/occupancy policy.
type LifecyclePlan struct {
	Session                      *models.Session
	Strips                       map[string]*models.Strip
	Assignments                  map[string]*models.StandAssignment
	Blocks                       map[string]*models.StandBlock
	AssignmentBlocks             map[string][]string
	BlockAdjacency               map[string][]string
	PhysicalOccupancy            map[string]string
	FrozenObservations           map[string]bool
	LiveObservations             map[string]bool
	ConsumedPrefiles             map[string]bool
	Stands                       *sat.StandCapabilityRegistry
	Policy                       *sat.AirlineAssignmentConfig
	Aircraft                     *sat.AircraftRegistry
	Engines                      *sat.AircraftEngineRegistry
	Borders                      *sat.AirportCountryRegistry
	Now                          time.Time
	Random                       func() float64
	AllowPrefiles                bool
	HoldDuration, BlockExtension time.Duration
	Callsign                     string
	SweepOnly                    bool
	Messages                     []LifecycleMessage
	MessageAvailable             bool
	StandWrites                  map[string]string
	Episodes                     map[string]string
	nextID                       int64
}
type LifecycleMessage struct{ Callsign, Text string }

func (p *LifecyclePlan) Run(ctx context.Context, departure bool, flights map[string]*vatsim.DepartureFlightInfo) error {
	if p.Session == nil || p.Stands == nil || p.Policy == nil {
		return fmt.Errorf("incomplete lifecycle policy snapshot")
	}
	for _, a := range p.Assignments {
		if a.ID > p.nextID {
			p.nextID = a.ID
		}
	}
	p.StandWrites = map[string]string{}
	if p.Episodes == nil {
		p.Episodes = map[string]string{}
	}
	memory := lifecycleMemory{p}
	random := p.Random
	if random == nil {
		random = func() float64 { return 0 }
	}
	policy := &StandAllocationService{stands: p.Stands, policy: p.Policy, now: func() time.Time { return p.Now }, random: random, departureReleaseBuffer: defaultDepartureBlockExtension}
	policy.planningBlocks = p.AssignmentBlocks
	policy.planningBlockAdjacency = p.BlockAdjacency
	policy.planningOccupancy = p.PhysicalOccupancy
	allocator := &planningAllocator{policy: policy, memory: memory}
	dep, err := NewDepartureLifecycleService(allocator, memory, memory, memory, p.Stands, p.Aircraft, p.Engines, p.Borders, WithDepartureLifecycleClock(policy.now), WithDeparturePrefileAssignments(p.AllowPrefiles), WithDepartureHoldDuration(p.HoldDuration), WithDepartureBlockExtension(p.BlockExtension))
	if err != nil {
		return err
	}
	arr, err := NewArrivalLifecycleService(allocator, memory, memory, memory, p.Stands, p.Aircraft, p.Engines, p.Borders, WithArrivalLifecycleClock(policy.now), WithArrivalPrefileAssignments(p.AllowPrefiles))
	if err != nil {
		return err
	}
	dep.SetWrongStandMessenger(p)
	dep.SetWarningEpisodes(p)
	dep.SetStandPublisher(p)
	keys := make([]string, 0, len(p.Strips))
	for key := range p.Strips {
		if p.SweepOnly {
			continue
		}
		if p.Callsign != "" && p.Callsign != key {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if !departure {
			priority := func(k string) int {
				f := flights[k]
				if f == nil {
					return 0
				}
				return arr.ArrivalProcessingPriority(p.Strips[k], arrivalInfo(f), p.Assignments[k])
			}
			if left, right := priority(keys[i]), priority(keys[j]); left != right {
				return left > right
			}
		}
		return keys[i] < keys[j]
	})
	for _, key := range keys {
		if p.FrozenObservations[key] {
			continue
		}
		strip := p.Strips[key]
		f := flights[key]
		origin, destination := strip.Origin, strip.Destination
		if f != nil {
			origin, destination = f.Origin, f.Destination
		}
		if departure {
			if !strings.EqualFold(strings.TrimSpace(origin), p.Session.Airport) {
				continue
			}
			if f == nil && strip.EuroscopeSeenAt == nil {
				err = dep.CancelDeparture(ctx, p.Session.ID, key)
			} else if f == nil && p.LiveObservations[key] && strip.PositionLatitude != nil && strip.PositionLongitude != nil {
				err = dep.ObserveDeparturePosition(ctx, p.Session.ID, strip, *strip.PositionLatitude, *strip.PositionLongitude)
			} else if f != nil && (f.Online && strip.EuroscopeSeenAt != nil || !f.Online && strip.EuroscopeSeenAt == nil && !(p.ConsumedPrefiles[key] && p.Assignments[key] == nil)) {
				err = dep.ProcessDeparture(ctx, p.Session.ID, strip, *f)
			}
			if err == nil && (strings.EqualFold(strings.TrimSpace(strip.Bay), "PUSH") || strings.EqualFold(valueString(strip.State), "PUSH")) {
				err = dep.ReleaseDepartureStand(ctx, p.Session.ID, key)
			}
		} else {
			if !strings.EqualFold(strings.TrimSpace(destination), p.Session.Airport) {
				continue
			}
			if f == nil {
				err = arr.CancelArrival(ctx, p.Session.ID, key)
			} else {
				err = arr.ProcessArrival(ctx, p.Session.ID, strip, arrivalInfo(f))
			}
		}
		if err != nil {
			return err
		}
	}
	// Sweeps use the persisted assignment deadlines, including orphaned strips.
	assignments, _ := memory.ListAssignments(ctx, p.Session.ID)
	for _, a := range assignments {
		if p.Callsign != "" && p.Callsign != a.Callsign {
			continue
		}
		if departure && a.Direction == string(sat.AssignmentDirectionDeparture) {
			err = dep.releaseIfDue(ctx, p.Session.ID, a, p.Now)
		}
		if !departure && a.Direction == string(sat.AssignmentDirectionArrival) {
			err = arr.releaseIfDue(ctx, p.Session.ID, a, p.Now)
		}
		if err != nil {
			return err
		}
	}
	if !departure && p.Callsign == "" {
		if err = allocator.ReleaseExpiredBlocks(ctx, p.Session.ID); err != nil {
			return err
		}
		return allocator.ReconcileUnsafeAssignments(ctx, p.Session.ID, p.Session.Airport)
	}
	return nil
}
func arrivalInfo(f *vatsim.DepartureFlightInfo) vatsim.ArrivalFlightInfo {
	return vatsim.ArrivalFlightInfo{Callsign: f.Callsign, CID: f.CID, Online: f.Online, Revision: f.Revision, Origin: f.Origin, Destination: f.Destination, AircraftType: f.AircraftType}
}
func (p *LifecyclePlan) SendPrivateMessageFromDelivery(_ int32, callsign, text string) bool {
	if !p.MessageAvailable {
		return false
	}
	p.Messages = append(p.Messages, LifecycleMessage{callsign, text})
	return true
}
func (p *LifecyclePlan) SendStandEvent(_ int32, callsign, stand string) {
	p.StandWrites[callsign] = stand
}
func (p *LifecyclePlan) WarningStand(key string) string { return p.Episodes[key] }
func (p *LifecyclePlan) SetWarningStand(key, stand string) {
	if stand == "" {
		delete(p.Episodes, key)
	} else {
		p.Episodes[key] = stand
	}
}

type lifecycleMemory struct{ p *LifecyclePlan }

func (m lifecycleMemory) List(context.Context) ([]*models.Session, error) {
	return []*models.Session{m.p.Session}, nil
}
func (m lifecycleMemory) GetByCallsign(_ context.Context, _ int32, key string) (*models.Strip, error) {
	s := m.p.Strips[key]
	if s == nil {
		return nil, errLifecycleNotFound
	}
	return s, nil
}
func (m lifecycleMemory) LockByCallsign(ctx context.Context, id int32, key string) (*models.Strip, error) {
	return m.GetByCallsign(ctx, id, key)
}
func (m lifecycleMemory) UpdateStand(_ context.Context, _ int32, key string, stand *string, _ *int32) (int64, error) {
	s := m.p.Strips[key]
	if s == nil {
		return 0, nil
	}
	if valueString(s.Stand) == valueString(stand) {
		return 0, nil
	}
	s.Stand = stand
	m.p.StandWrites[key] = valueString(stand)
	return 1, nil
}
func (m lifecycleMemory) GetAssignment(_ context.Context, _ int32, key string) (*models.StandAssignment, error) {
	a := m.p.Assignments[key]
	if a == nil {
		return nil, errLifecycleNotFound
	}
	copy := *a
	return &copy, nil
}
func (m lifecycleMemory) ListAssignments(context.Context, int32) ([]*models.StandAssignment, error) {
	keys := make([]string, 0, len(m.p.Assignments))
	for key := range m.p.Assignments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*models.StandAssignment, 0, len(keys))
	for _, key := range keys {
		copy := *m.p.Assignments[key]
		out = append(out, &copy)
	}
	return out, nil
}
func (m lifecycleMemory) CreateAssignment(_ context.Context, a *models.StandAssignment) error {
	if m.p.Assignments[a.Callsign] != nil {
		return fmt.Errorf("duplicate assignment")
	}
	m.p.nextID++
	a.ID = m.p.nextID
	a.Version = 1
	a.CreatedAt = m.p.Now
	a.UpdatedAt = m.p.Now
	copy := *a
	m.p.Assignments[a.Callsign] = &copy
	return nil
}
func (m lifecycleMemory) UpdateAssignment(_ context.Context, a *models.StandAssignment) (int64, error) {
	old := m.p.Assignments[a.Callsign]
	if old == nil || old.Version != a.Version {
		return 0, nil
	}
	copy := *a
	copy.Version++
	copy.UpdatedAt = m.p.Now
	m.p.Assignments[a.Callsign] = &copy
	return 1, nil
}
func (m lifecycleMemory) DeleteAssignment(_ context.Context, _ int32, id int64, version int32) (int64, error) {
	for key, a := range m.p.Assignments {
		if a.ID == id && a.Version == version {
			delete(m.p.Assignments, key)
			return 1, nil
		}
	}
	return 0, nil
}

type planningAllocator struct {
	policy   *StandAllocationService
	memory   lifecycleMemory
	relocate DisplacedArrivalHandler
}

func (a *planningAllocator) SetDisplacedArrivalHandler(f DisplacedArrivalHandler) { a.relocate = f }
func (a *planningAllocator) Allocate(ctx context.Context, r StandAllocationRequest) (*StandAllocationResult, error) {
	return a.allocate(ctx, AutomaticStandAllocation, r)
}
func (a *planningAllocator) Reallocate(ctx context.Context, r StandAllocationRequest) (*StandAllocationResult, error) {
	return a.allocate(ctx, AutomaticStandReallocation, r)
}
func (a *planningAllocator) assignObservedStand(ctx context.Context, r StandAllocationRequest) (*StandAllocationResult, error) {
	return a.allocate(ctx, observedStandAllocation, r)
}
func (a *planningAllocator) allocate(ctx context.Context, command StandAllocationCommand, r StandAllocationRequest) (*StandAllocationResult, error) {
	if err := validateStandAllocationRequest(command, &r); err != nil {
		return nil, err
	}
	ctx = beginRelocationChain(ctx, r.Callsign)
	p := a.memory.p
	assignments, _ := a.memory.ListAssignments(ctx, r.SessionID)
	blocks := a.blocks()
	if command == observedStandAllocation {
		filtered := blocks[:0]
		for _, b := range blocks {
			if standName(b.Stand) != standName(r.Stand) {
				filtered = append(filtered, b)
			}
		}
		blocks = filtered
	}
	evaluation := p.Stands.EvaluateCompatibility(r.Airport, r.FlightFacts)
	selected, selection, match, available, conflict, err := a.policy.selectStand(command, r, evaluation, assignments, blocks, map[string]struct{}{})
	if err != nil {
		return nil, err
	}
	if isAutomaticStandAllocation(command) && a.policy.selectedHasOverlappingEstimatedReservation(assignments, selected, matchBlocks(match), r, p.Now, a.policy.departureReleaseBuffer) {
		r.addDisplaceArrivalStage(StageEstimated)
	}
	removed, standChanges, err := a.policy.displaceAssignments(ctx, a.memory, a.memory, r, selected, matchBlocks(match), assignments)
	if err != nil {
		return nil, err
	}
	r.Stand = selected
	assignment, err := a.policy.persistStandAllocation(ctx, a.memory, command, r, assignments, selection, match, conflict)
	if err != nil {
		return nil, err
	}
	if p.AssignmentBlocks != nil {
		p.AssignmentBlocks[r.Callsign] = append([]string(nil), matchBlocks(match)...)
	}
	if _, err = a.memory.UpdateStand(ctx, r.SessionID, r.Callsign, &selected, nil); err != nil {
		return nil, err
	}
	if command == observedStandAllocation {
		delete(p.Blocks, standName(selected))
	}
	if r.Stage == StageConfirmed {
		p.StandWrites[r.Callsign] = selected
	}
	for _, old := range removed {
		if a.relocate != nil {
			_ = a.relocate(ctx, old)
		}
	}
	return &StandAllocationResult{Command: command, Assignment: *assignment, Selection: selection, MatchedVariant: match, Compatibility: evaluation, AvailableCandidates: available, ConflictReason: conflict, RemovedAssignments: removed, RemovedStandChanges: standChanges}, nil
}
func (a *planningAllocator) blocks() []*models.StandBlock {
	out := make([]*models.StandBlock, 0, len(a.memory.p.Blocks))
	for _, b := range a.memory.p.Blocks {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stand < out[j].Stand })
	return out
}
func (a *planningAllocator) StandAvailable(ctx context.Context, r StandAllocationRequest, stand string) (bool, error) {
	evaluation := a.policy.stands.EvaluateCompatibility(r.Airport, r.FlightFacts)
	matches := map[string]sat.StandCompatibilityMatch{}
	for _, m := range evaluation.Matches {
		matches[standName(m.Stand.Name)] = m
	}
	if _, ok := matches[standName(stand)]; !ok {
		return false, nil
	}
	assignments, _ := a.memory.ListAssignments(ctx, r.SessionID)
	return len(a.policy.availability(r, assignments, a.blocks(), matches)[standName(stand)]) == 0, nil
}
func (a *planningAllocator) ConfirmedArrivalConflictAtStand(ctx context.Context, r StandAllocationRequest, stand string) (bool, error) {
	assignments, _ := a.memory.ListAssignments(ctx, r.SessionID)
	return a.policy.confirmedArrivalConflicts(r, standName(stand), assignments), nil
}
func (a *planningAllocator) PublishAssignment(context.Context, models.StandAssignment) error {
	// Replacements are already in the snapshot; the caller emits the complete
	// diff at its single commit boundary, never an intermediate publication.
	return nil
}
func (a *planningAllocator) PublishConfirmedArrival(_ context.Context, s models.StandAssignment) error {
	a.memory.p.StandWrites[s.Callsign] = s.Stand
	return nil
}
func (a *planningAllocator) ReleaseAssignment(ctx context.Context, s *models.StandAssignment) error {
	return a.releaseAssignment(ctx, s, true)
}
func (a *planningAllocator) ReleaseAssignmentRetainingStand(ctx context.Context, s *models.StandAssignment) error {
	return a.releaseAssignment(ctx, s, false)
}
func (a *planningAllocator) releaseAssignment(ctx context.Context, s *models.StandAssignment, clear bool) error {
	current := a.memory.p.Assignments[s.Callsign]
	if current == nil {
		return nil
	}
	if current.Version != s.Version {
		return fmt.Errorf("stale assignment")
	}
	strip := a.memory.p.Strips[s.Callsign]
	if clear && strip != nil && valueString(strip.Stand) == s.Stand {
		_, _ = a.memory.UpdateStand(ctx, s.SessionID, s.Callsign, nil, nil)
	}
	delete(a.memory.p.Assignments, s.Callsign)
	return nil
}
func (a *planningAllocator) ReleaseExpiredBlocks(_ context.Context, _ int32) error {
	for key, b := range a.memory.p.Blocks {
		if expired(b.ExpiresAt, a.memory.p.Now) {
			delete(a.memory.p.Blocks, key)
		}
	}
	return nil
}
func (a *planningAllocator) ReconcileUnsafeAssignments(ctx context.Context, id int32, airport string) error {
	assignments, _ := a.memory.ListAssignments(ctx, id)
	for _, s := range a.policy.unsafeAssignmentLosers(airport, assignments, a.memory.p.Now) {
		if err := a.releaseAssignment(ctx, s, true); err != nil {
			return err
		}
		if a.relocate != nil {
			_ = a.relocate(ctx, *s)
		}
	}
	return nil
}
func (a *planningAllocator) ReconcileObservedDepartureConflict(ctx context.Context, r StandAllocationRequest, stand string) error {
	current, err := a.memory.GetAssignment(ctx, r.SessionID, r.Callsign)
	if err != nil {
		return err
	}
	conflict, _ := a.ConfirmedArrivalConflictAtStand(ctx, r, stand)
	if conflict {
		current.ConflictReason = observedDepartureConflictReason(current.ConflictReason)
	} else {
		current.ConflictReason = priorObservedDepartureConflict(current.ConflictReason)
	}
	current.Acknowledged = false
	current.AcknowledgedAt = nil
	current.AcknowledgedBy = nil
	_, err = a.memory.UpdateAssignment(ctx, current)
	return err
}
