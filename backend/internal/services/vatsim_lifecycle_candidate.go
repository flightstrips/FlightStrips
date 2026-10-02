package services

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VatsimLifecycleCandidate consumes the existing global source and submits
// concrete owner commands. It never polls a provider or starts itself.
type VatsimLifecycleCandidate struct {
	Source                       cluster.NavigationWeather
	Writer                       cluster.Writer
	Stands                       cluster.StandState
	Secrets                      cluster.EffectSecrets
	AllowPrefiles                bool
	HoldDuration, BlockExtension time.Duration
	ESMessages                   *bool
	// Positions resolves the current owner/master dispatcher; required when
	// deriving a transition from operational FS_POSITIONS observations.
	Positions func(int32) *cluster.PositionWriter
}

func NewVatsimLifecycleCandidate(source cluster.NavigationWeather, writer cluster.Writer, stands cluster.StandState, secrets cluster.EffectSecrets) (*VatsimLifecycleCandidate, error) {
	if source.Objects == nil || writer.Store == nil || writer.Projection == nil || writer.Lease == nil || stands.Stands == nil || stands.Policy == nil || secrets.Objects == nil || len(secrets.Keys[secrets.ActiveKeyID]) != 32 {
		return nil, fmt.Errorf("VATSIM lifecycle requires source, owner writer, projection, SAT policy and effect secrets")
	}
	stands.Projection = writer.Projection
	return &VatsimLifecycleCandidate{Source: source, Writer: writer, Stands: stands, Secrets: secrets}, nil
}

func (c *VatsimLifecycleCandidate) Departure(ctx context.Context, id int32) error {
	return c.reconcile(ctx, id, true)
}
func (c *VatsimLifecycleCandidate) Arrival(ctx context.Context, id int32) error {
	return c.reconcile(ctx, id, false)
}

func (c *VatsimLifecycleCandidate) reconcile(ctx context.Context, id int32, departure bool) error {
	if c.Writer.Lease != nil && !c.Writer.Lease.CanWrite(sessionRef(id)) {
		return fmt.Errorf("VATSIM lifecycle session is not owned")
	}
	checkpoint, page, revision, err := c.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	if err != nil {
		return err
	}
	if checkpoint == nil || page.GetVatsim() == nil || revision == 0 {
		return fmt.Errorf("accepted VATSIM generation unavailable")
	}
	reply := (cluster.VatsimSessionAdapter{Source: c.Source, Writer: c.Writer}).Reconcile(ctx, id)
	if err = lifecycleReply(reply); err != nil {
		return err
	}
	state, err := c.Writer.Projection.ReadLifecyclePlanning(sessionRef(id))
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))].GetValue().GetSession()
	flights := lifecycleFlights(page.GetVatsim(), seed.GetAirport())
	keys := map[string]bool{}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STRIP) {
		keys[e.Key] = true
	}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STAND_ASSIGNMENT) {
		keys[e.Key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	priorities := map[string]int{}
	if !departure {
		observations, _, err := c.Writer.Projection.ObservationSnapshot(id)
		if err != nil {
			return err
		}
		policy := ArrivalLifecycleService{stands: c.Stands.Stands, now: c.clock}
		for _, key := range ordered {
			entity := state.Indexes[pb.EntityKind_STRIP][key]
			f := flights[key]
			if entity == nil || f == nil {
				continue
			}
			strip := lifecycleModelStrip(entity.GetValue().GetStrip(), id)
			for _, sourceFlight := range page.GetVatsim().Flights {
				if sourceFlight.Callsign == key && sourceFlight.State == "online" {
					strip.PositionLatitude, strip.PositionLongitude, strip.PositionAltitude = &sourceFlight.Latitude, &sourceFlight.Longitude, &sourceFlight.Altitude
				}
			}
			for _, o := range observations {
				if !o.Stale && o.Value.AircraftKey == key && o.Value.GetPosition() != nil {
					pos := o.Value.GetPosition()
					strip.PositionLatitude, strip.PositionLongitude, strip.PositionAltitude = &pos.Latitude, &pos.Longitude, &pos.AltitudeFeet
				}
			}
			var assignment *models.StandAssignment
			if a := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key]; a != nil {
				assignment = lifecycleModelAssignment(a.GetValue().GetStandAssignment(), id, 1)
			}
			priorities[key] = policy.ArrivalProcessingPriority(strip, arrivalInfo(f), assignment)
		}
	}
	// Cancellation precedes allocation. Existing hard arrivals are processed
	// before new soft reservations, matching the legacy reconciler.
	sort.Slice(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if (flights[left] == nil) != (flights[right] == nil) {
			return flights[left] == nil
		}
		if !departure {
			if l, r := priorities[left], priorities[right]; l != r {
				return l > r
			}
			if (state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][left] != nil) != (state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][right] != nil) {
				return state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][left] != nil
			}
		}
		return left < right
	})
	cached := state
	passes := 1
	if !departure {
		passes = 5
	}
	for pass := 0; pass < passes; pass++ {
		before, err := c.Writer.Projection.ReadLifecyclePlanning(sessionRef(id))
		if err != nil {
			return err
		}
		for _, key := range ordered {
			if departure && flights[key] == nil {
				if err = c.transitionWithSnapshot(ctx, id, key, false, revision, checkpoint.Sha256, page.GetVatsim(), &cached); err != nil {
					return err
				}
			}
			if err = c.transitionWithSnapshot(ctx, id, key, departure, revision, checkpoint.Sha256, page.GetVatsim(), &cached); err != nil {
				return err
			}
		}
		after, err := c.Writer.Projection.ReadLifecyclePlanning(sessionRef(id))
		if err != nil {
			return err
		}
		if departure || lifecycleArrivalStateEqual(before, after) {
			break
		}
		if pass == passes-1 {
			return fmt.Errorf("SAT arrival reconciliation reached the fixed cap of %d passes", passes)
		}
	}
	if err = c.transitionWithSnapshot(ctx, id, "", departure, revision, checkpoint.Sha256, page.GetVatsim(), &cached); err != nil {
		return err
	}
	return c.resumeStandEffects(ctx, id)
}

func lifecycleReply(reply *pb.CommandReply) error {
	if reply == nil || (reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING) || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return fmt.Errorf("lifecycle command failed: %v", reply)
	}
	return nil
}
func lifecycleFlights(page *pb.VatsimPage, airport string) map[string]*vatsim.DepartureFlightInfo {
	out := map[string]*vatsim.DepartureFlightInfo{}
	for _, f := range page.Flights {
		p := f.FlightPlan
		if !strings.EqualFold(strings.TrimSpace(p.Origin), airport) && !strings.EqualFold(strings.TrimSpace(p.Destination), airport) {
			continue
		}
		out[f.Callsign] = &vatsim.DepartureFlightInfo{Callsign: f.Callsign, CID: f.Cid, Online: f.State == "online", Revision: p.Revision, Origin: p.Origin, Destination: p.Destination, AircraftType: p.AircraftShort, Latitude: f.Latitude, Longitude: f.Longitude}
	}
	return out
}

func (c *VatsimLifecycleCandidate) transition(ctx context.Context, id int32, key string, departure bool, revision uint64, sha string, page *pb.VatsimPage) error {
	var cached *cluster.Aggregate
	return c.transitionWithSnapshot(ctx, id, key, departure, revision, sha, page, &cached)
}

func (c *VatsimLifecycleCandidate) transitionWithSnapshot(ctx context.Context, id int32, key string, departure bool, revision uint64, sha string, page *pb.VatsimPage, cached **cluster.Aggregate) error {
	for attempts := 0; attempts < 4; attempts++ {
		state, err := c.Writer.Projection.ReadLifecyclePlanningCached(sessionRef(id), *cached)
		if err != nil {
			return err
		}
		*cached = state
		positions, _, err := c.Writer.Projection.ObservationSnapshot(id)
		if err != nil {
			return err
		}
		action := "arrival"
		if departure {
			action = "departure"
		}
		// The identity includes the accepted source, callsign, action, entity read
		// revision and tagged observation revisions. Deadlines remain in entities.
		tag := lifecyclePositionTag(positions)
		commandID, _ := cluster.ProviderEventCommandID("vatsim-lifecycle", "vatsim", fmt.Sprintf("%d/%d/%s/%s/%s/%d/%s", id, revision, sha, key, action, state.Revision, tag))
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: sessionRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-lifecycle"}, ExpectedEntityRevision: &state.Revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_VatsimSessionCursor{VatsimSessionCursor: &pb.VatsimSessionCursor{Provider: "vatsim", SourceRevision: revision, SourceSha256: sha, SnapshotAt: page.SnapshotAt}}}}}}}}
		// Avoid outcome-only events for unchanged polls. The authoritative planner
		// below reconstructs this decision from fresh owner state before publication.
		change, err := c.plan(ctx, request, state, key, departure, revision, sha, page, positions, false)
		if err != nil {
			return err
		}
		if len(change.Changes) == 0 && len(change.Workflows) == 0 && len(change.Effects) == 0 {
			return nil
		}
		writer := c.Writer
		writer.Plan = func(ctx context.Context, req *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != current.Revision {
				return nil, pb.CommandReply_UNAVAILABLE, current.Revision, fmt.Errorf("lifecycle state changed; rederive")
			}
			cp, _, rev, err := c.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
			if err != nil || cp == nil || rev != revision || cp.Sha256 != sha {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("VATSIM source changed; rederive")
			}
			cursor := current.Indexes[pb.EntityKind_VATSIM_SESSION_CURSOR]["vatsim"].GetValue().GetVatsimSessionCursor()
			if cursor == nil || cursor.SourceRevision != revision || cursor.SourceSha256 != sha {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("session source changed")
			}
			currentPositions, _, err := c.Writer.Projection.ObservationSnapshot(id)
			if err != nil || lifecyclePositionTag(currentPositions) != tag {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("position observation changed; rederive")
			}
			changes, err := c.plan(ctx, req, current, key, departure, revision, sha, page, currentPositions, true)
			if err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, 0, err
			}
			return changes, pb.CommandReply_COMMITTED, 0, nil
		}
		run := func(runCtx context.Context) (*pb.CommandReply, error) {
			reply := writer.Execute(runCtx, request)
			return reply, lifecycleReply(reply)
		}
		var result *pb.CommandReply
		fresh := false
		for _, p := range positions {
			fresh = fresh || !p.Stale
		}
		var dispatcher *cluster.PositionWriter
		if c.Positions != nil {
			dispatcher = c.Positions(id)
		}
		sync, syncErr := c.Writer.Projection.OperationalSync(sessionRef(id))
		if syncErr != nil {
			return syncErr
		}
		if fresh || sync != nil || dispatcher != nil {
			if dispatcher == nil {
				return fmt.Errorf("operational lifecycle requires position dispatcher")
			}
			result, err = dispatcher.ExecuteLifecycleContext(ctx, positions, run)
		} else {
			result, err = run(ctx)
		}
		if err == nil {
			return nil
		}
		if result != nil && result.Status != pb.CommandReply_UNAVAILABLE {
			return err
		}
	}
	return fmt.Errorf("lifecycle inputs did not settle")
}
func lifecyclePositionTag(positions []cluster.KVPosition) string {
	var b strings.Builder
	for _, p := range positions {
		fmt.Fprintf(&b, "%s:%d:%d:%t;", p.Value.AircraftKey, p.Value.OwnerEpoch, p.Revision, p.Stale)
	}
	return b.String()
}

func (c *VatsimLifecycleCandidate) plan(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate, key string, departure bool, revision uint64, sha string, page *pb.VatsimPage, positions []cluster.KVPosition, stage bool) (*pb.DomainChange, error) {
	seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(request.Aggregate.GetSession().Id))].GetValue().GetSession()
	if seed == nil || seed.Tombstoned {
		return nil, fmt.Errorf("live session unavailable")
	}
	p := LifecyclePlan{Session: &models.Session{ID: seed.Id, Name: seed.Name, Airport: seed.Airport}, Callsign: key, Strips: map[string]*models.Strip{}, Assignments: map[string]*models.StandAssignment{}, Blocks: map[string]*models.StandBlock{}, Stands: c.Stands.Stands, Policy: c.Stands.Policy, Aircraft: c.Stands.Aircraft, Engines: c.Stands.Engines, Borders: c.Stands.Borders, Now: c.clock(), AllowPrefiles: c.AllowPrefiles, Episodes: map[string]string{}}
	p.HoldDuration, p.BlockExtension = c.HoldDuration, c.BlockExtension
	// Retain SAT's weighted choice with reproducible draws on planner retries.
	drawSeed := sha256.Sum256([]byte(request.CommandId))
	p.Random = rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(drawSeed[:8])))).Float64
	p.SweepOnly = key == ""
	p.AssignmentBlocks = map[string][]string{}
	p.BlockAdjacency = map[string][]string{}
	p.PhysicalOccupancy = map[string]string{}
	p.FrozenObservations = map[string]bool{}
	p.LiveObservations = map[string]bool{}
	p.ConsumedPrefiles = map[string]bool{}
	prefileID := lifecycleID(fmt.Sprintf("vatsim-prefile/%d/%s/%d/%s", seed.Id, key, revision, sha), "input")
	w, err := state.LookupWorkflow(prefileID)
	if err != nil {
		return nil, err
	}
	if w != nil && w.Status == pb.WorkflowRecord_COMPLETED {
		p.ConsumedPrefiles[key] = true
	}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STRIP) {
		p.Strips[e.Key] = lifecycleModelStrip(e.GetValue().GetStrip(), seed.Id)
	}
	flights := lifecycleFlights(page, seed.Airport)
	for _, f := range page.Flights {
		if s := p.Strips[f.Callsign]; s != nil && f.State == "online" {
			lat, lon, alt := f.Latitude, f.Longitude, f.Altitude
			s.PositionLatitude = &lat
			s.PositionLongitude = &lon
			s.PositionAltitude = &alt
		}
	}
	// Last accepted positions survive dropouts for cancellation/grace checks.
	for _, o := range positions {
		if o.Stale && flights[o.Value.AircraftKey] != nil {
			if o.Value.ObservedAt == nil || o.Value.ObservedAt.AsTime().Before(page.SnapshotAt.AsTime()) {
				continue
			}
			p.FrozenObservations[o.Value.AircraftKey] = true
		}
		if s := p.Strips[o.Value.AircraftKey]; s != nil && o.Value.GetPosition() != nil {
			pos := o.Value.GetPosition()
			s.PositionLatitude = &pos.Latitude
			s.PositionLongitude = &pos.Longitude
			s.PositionAltitude = &pos.AltitudeFeet
			if !o.Stale {
				p.LiveObservations[o.Value.AircraftKey] = true
				if f := flights[o.Value.AircraftKey]; f != nil {
					f.Latitude, f.Longitude = pos.Latitude, pos.Longitude
				}
			}
		}
	}
	for i, e := range lifecycleEntities(state, pb.EntityKind_STAND_ASSIGNMENT) {
		p.Assignments[e.Key] = lifecycleModelAssignment(e.GetValue().GetStandAssignment(), seed.Id, int64(i+1))
		p.AssignmentBlocks[e.Key] = append([]string(nil), e.GetValue().GetStandAssignment().BlockedStands...)
	}
	// A callsign may reconnect under a different pilot. Preserve the booking,
	// but start a new warning episode rather than suppressing it with old state.
	if f := flights[key]; f != nil {
		if a := p.Assignments[key]; a != nil && a.VatsimCID != nil {
			if cid := parseCID(f.CID); cid != nil && *cid != *a.VatsimCID && a.ConflictReason != nil && isWrongStandConflictReason(*a.ConflictReason) {
				a.ConflictReason = nil
				a.Acknowledged, a.AcknowledgedAt, a.AcknowledgedBy = false, nil, nil
				a.UpdatedAt = p.Now
			}
		}
	}
	for _, o := range positions {
		pos := o.Value.GetPosition()
		if o.Stale || pos == nil || pos.AltitudeFeet > 1000 || p.Assignments[o.Value.AircraftKey] != nil {
			continue
		}
		if stand, ok := p.Stands.StandAtPosition(seed.Airport, pos.Latitude, pos.Longitude); ok {
			p.PhysicalOccupancy[o.Value.AircraftKey] = stand.Name
		}
	}
	for i, e := range lifecycleEntities(state, pb.EntityKind_STAND_BLOCK) {
		b := e.GetValue().GetStandBlock()
		p.BlockAdjacency[e.Key] = append([]string(nil), b.BlockedStands...)
		p.Blocks[e.Key] = &models.StandBlock{ID: int64(i + 1), SessionID: seed.Id, Stand: b.Stand, BlockType: b.BlockType, Source: b.Source, Reason: &b.Reason, Callsign: b.Callsign, CreatedBy: &b.Actor, ExpiresAt: lifecycleTime(b.ExpiresAt), Manual: b.Manual, Version: int32(e.Revision), CreatedAt: b.CreatedAt.AsTime(), UpdatedAt: b.UpdatedAt.AsTime()}
	}
	pilot := ""
	if s := p.Strips[key]; s != nil {
		pilot = valueString(s.VatsimCID)
	}
	episodeID := lifecycleID(fmt.Sprintf("vatsim-warning/%d/%s/%s", seed.Id, key, pilot), "episode")
	w, err = state.LookupWorkflow(episodeID)
	if err != nil {
		return nil, err
	}
	if w != nil && w.Status == pb.WorkflowRecord_COMPLETED {
		p.Episodes[fmt.Sprintf("%d:%s", seed.Id, key)] = w.Step
	}
	target := c.deliveryCID(state, seed.Id)
	p.MessageAvailable = target != ""
	if c.ESMessages != nil && !*c.ESMessages {
		p.MessageAvailable = false
	}
	if err := p.Run(ctx, departure, flights); err != nil {
		return nil, err
	}
	if f := flights[key]; f != nil && !p.FrozenObservations[key] {
		if a := p.Assignments[key]; a != nil {
			prior := c.lifecycleAssignment(a, state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key], seed.Airport)
			applyVatsimIdentity(a, p.Strips[key], f.CID, f.Revision)
			if !proto.Equal(prior, c.lifecycleAssignment(a, state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key], seed.Airport)) {
				a.UpdatedAt = p.Now
			}
		}
	}
	change := &pb.DomainChange{}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STRIP) {
		s := p.Strips[e.Key]
		prior := e.GetValue().GetStrip()
		if s == nil {
			continue
		}
		if e.Key == key && prior.VatsimOnly && prior.VatsimCid != "" && flights[key] == nil && p.Assignments[key] == nil && prior.OwnerCid == "" && len(prior.ControllerModifiedFields) == 0 {
			change.Changes = append(change.Changes, candidateDelete(e.Key, e, pb.EntityKind_STRIP))
			continue
		}
		stand := valueString(s.Stand)
		// SAT changes only the stand on the persisted strip. Copying and comparing
		// the entire aircraft for an unchanged stand adds no planning information.
		if stand != prior.Stand {
			copy := proto.Clone(prior).(*pb.Strip)
			copy.Stand = stand
			change.Changes = append(change.Changes, stripChange(e, copy))
		}
	}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STAND_ASSIGNMENT) {
		if p.Assignments[e.Key] == nil {
			change.Changes = append(change.Changes, candidateDelete(e.Key, e, pb.EntityKind_STAND_ASSIGNMENT))
		}
	}
	for callsign, a := range p.Assignments {
		old := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][callsign]
		value := c.lifecycleAssignment(a, old, seed.Airport)
		value.BlockedStands = append([]string(nil), p.AssignmentBlocks[callsign]...)
		sort.Strings(value.BlockedStands)
		if old == nil || !proto.Equal(value, old.GetValue().GetStandAssignment()) {
			value.Revision = standRevision(old) + 1
			change.Changes = append(change.Changes, candidateUpsert(callsign, old, &pb.EntityRecord{Value: &pb.EntityRecord_StandAssignment{StandAssignment: value}}))
		}
	}
	// A due hold may disappear while its prefile is still in this generation.
	// Record that input with the transition so replay cannot recreate the hold.
	// A newly accepted generation remains a new input, as in the reconciler.
	if departure && key != "" && !p.ConsumedPrefiles[key] {
		for _, update := range change.Changes {
			if update.Key != key {
				continue
			}
			old := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key].GetValue().GetStandAssignment()
			next := update.GetUpsert().GetStandAssignment()
			if (old != nil && old.Stage == StageReserved && !old.Manual) || (next != nil && next.Stage == StageReserved && !next.Manual) {
				change.Workflows = append(change.Workflows, &pb.WorkflowRecord{WorkflowId: prefileID, Source: request.Aggregate, Destination: request.Aggregate, Step: "vatsim-prefile-input", DerivedCommandId: request.CommandId, Status: pb.WorkflowRecord_COMPLETED, SourceRevision: &revision})
				break
			}
		}
	}
	for _, e := range lifecycleEntities(state, pb.EntityKind_STAND_BLOCK) {
		if p.Blocks[e.Key] == nil {
			change.Changes = append(change.Changes, candidateDelete(e.Key, e, pb.EntityKind_STAND_BLOCK))
		}
	}
	episode := p.Episodes[fmt.Sprintf("%d:%s", seed.Id, key)]
	oldEpisode, err := state.LookupWorkflow(episodeID)
	if err != nil {
		return nil, err
	}
	prior := ""
	if oldEpisode != nil && oldEpisode.Status == pb.WorkflowRecord_COMPLETED {
		prior = oldEpisode.Step
	}
	if episode != prior {
		status := pb.WorkflowRecord_COMPLETED
		if episode == "" {
			status = pb.WorkflowRecord_SUPERSEDED
			episode = prior
		}
		change.Workflows = append(change.Workflows, &pb.WorkflowRecord{WorkflowId: episodeID, Source: request.Aggregate, Destination: request.Aggregate, Step: episode, DerivedCommandId: request.CommandId, Status: status, SourceRevision: &revision})
	}
	if len(p.Messages) > 1 {
		return nil, fmt.Errorf("lifecycle produced multiple private messages for one callsign")
	}
	if len(p.Messages) == 1 {
		message := p.Messages[0]
		effect := &pb.EffectRecord{CommandId: request.CommandId, TargetCid: target, OwnerEpoch: state.Owner.Epoch, Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(p.Now.Add(effectDispatchWindow))}
		if stage {
			secret, err := c.Secrets.StagePrivateMessage(effect.CommandId, target, message.Callsign, message.Text)
			if err != nil {
				return nil, err
			}
			effect.Payload = &pb.EffectRecord_PrivateMessage{PrivateMessage: secret}
		}
		change.Effects = append(change.Effects, effect)
	}
	// STAND plugin actions have a distinct Task 17 outcome. A durable workflow
	// links the changed strip revision to the immutable master CID and action.
	for callsign, stand := range p.StandWrites {
		if state.Master == nil || state.Master.Cid == "" {
			continue
		}
		strip := state.Indexes[pb.EntityKind_STRIP][callsign]
		if strip == nil {
			continue
		}
		rev := strip.Revision
		for _, update := range change.Changes {
			if update.Key == callsign && update.GetUpsert().GetStrip() != nil {
				rev = update.Revision
			}
		}
		workflowID := lifecycleID(request.CommandId, "stand/"+callsign)
		effectID := lifecycleID(workflowID, "effect")
		value := stand
		if value == "" {
			value = "-"
		}
		change.Workflows = append(change.Workflows, &pb.WorkflowRecord{WorkflowId: workflowID, Source: request.Aggregate, Destination: request.Aggregate, Step: "vatsim-stand/" + callsign + "/" + value + "/" + state.Master.Cid, DerivedCommandId: effectID, Status: pb.WorkflowRecord_PENDING, SourceRevision: &rev})
	}
	sortCandidateChanges(change.Changes)
	sort.Slice(change.Workflows, func(i, j int) bool { return change.Workflows[i].WorkflowId < change.Workflows[j].WorkflowId })
	return change, nil
}

func (c *VatsimLifecycleCandidate) deliveryCID(state *cluster.Aggregate, id int32) string {
	presence, err := c.Writer.Projection.PresenceSnapshot(id)
	if err != nil {
		return ""
	}
	nodes := map[string]bool{}
	for _, o := range presence {
		if n := o.Value.GetNode(); n != nil && n.Ready {
			nodes[n.NodeId] = true
		}
	}
	candidates := []string{}
	for _, o := range presence {
		client := o.Value.GetClient()
		if client != nil && client.Kind == pb.ClientPresence_EUROSCOPE && !client.Observer && nodes[client.NodeId] {
			position := client.Position
			delivery := position == "DEL"
			for _, e := range lifecycleEntities(state, pb.EntityKind_SECTOR_OWNER) {
				sector := e.GetValue().GetSectorOwner()
				if sector.Sector == "DEL" && (sector.ControllerCid == client.Cid || sector.Position == position) {
					delivery = true
				}
			}
			if delivery {
				candidates = append(candidates, client.Cid)
			}
		}
	}
	sort.Strings(candidates)
	if len(candidates) == 0 {
		if state.Master != nil {
			for _, o := range presence {
				client := o.Value.GetClient()
				if client != nil && client.Cid == state.Master.Cid && client.Kind == pb.ClientPresence_EUROSCOPE && nodes[client.NodeId] {
					return client.Cid
				}
			}
		}
		return ""
	}
	return candidates[0]
}

func (c *VatsimLifecycleCandidate) resumeStandEffects(ctx context.Context, id int32) error {
	state, err := c.Writer.Projection.ReadLifecyclePlanning(sessionRef(id))
	if err != nil {
		return err
	}
	ids := []string{}
	for key, w := range state.Workflows {
		if w.Status == pb.WorkflowRecord_PENDING && strings.HasPrefix(w.Step, "vatsim-stand/") {
			ids = append(ids, key)
		}
	}
	sort.Strings(ids)
	for _, key := range ids {
		intent := state.Workflows[key]
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: intent.DerivedCommandId, Aggregate: sessionRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-stand-effect"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: proto.Clone(intent).(*pb.WorkflowRecord)}}}}}
		writer := c.Writer
		writer.Plan = func(_ context.Context, _ *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			w := current.Workflows[key]
			if w == nil || !proto.Equal(w, intent) {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("stand intent changed")
			}
			tokens := strings.Split(w.Step, "/")
			if len(tokens) != 4 {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid stand intent")
			}
			strip := current.Indexes[pb.EntityKind_STRIP][tokens[1]]
			copy := proto.Clone(w).(*pb.WorkflowRecord)
			copy.Status = pb.WorkflowRecord_COMPLETED
			change := &pb.DomainChange{Workflows: []*pb.WorkflowRecord{copy}}
			stand := tokens[2]
			if stand == "-" {
				stand = ""
			}
			if strip == nil || w.SourceRevision == nil || strip.Revision != *w.SourceRevision || strip.GetValue().GetStrip().Stand != stand {
				copy.Status = pb.WorkflowRecord_SUPERSEDED
				return change, pb.CommandReply_COMMITTED, 0, nil
			}
			change.Effects = []*pb.EffectRecord{{CommandId: w.DerivedCommandId, TargetCid: tokens[3], OwnerEpoch: current.Owner.Epoch, Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(c.clock().Add(effectDispatchWindow)), Payload: &pb.EffectRecord_SetFlightPlan{SetFlightPlan: &pb.SetFlightPlanEffect{Callsign: tokens[1], Field: "STAND", Value: stand}}}}
			return change, pb.CommandReply_COMMITTED, 0, nil
		}
		if err = lifecycleReply(writer.Execute(ctx, request)); err != nil {
			return err
		}
	}
	return nil
}

func lifecycleTime(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	value := t.AsTime()
	return &value
}
func lifecycleTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
func lifecycleClock(t *timestamppb.Timestamp) *string {
	if t == nil {
		return nil
	}
	s := t.AsTime().UTC().Format("150405")
	return &s
}
func lifecycleModelStrip(s *pb.Strip, id int32) *models.Strip {
	m := &models.Strip{ID: int32(s.Id), Version: int32(s.Revision), Session: id, Callsign: s.Callsign, Origin: s.Departure, Destination: s.Destination, AircraftType: &s.AircraftType, Stand: &s.Stand, Bay: s.Bay, State: &s.GroundState, EngineType: s.EngineType, StartReq: s.StartRequested, PositionLatitude: s.VatsimLatitude, PositionLongitude: s.VatsimLongitude, PositionAltitude: s.PositionAltitudeFeet, VatsimCID: &s.VatsimCid, VatsimRevision: &s.VatsimPlanRevision, VatsimSeenAt: lifecycleTime(s.VatsimSeenAt), CdmData: &models.CdmData{Tobt: lifecycleClock(s.Tobt), Tsat: lifecycleClock(s.Tsat), Aldt: lifecycleClock(s.Aldt)}}
	if !s.VatsimOnly {
		m.EuroscopeSeenAt = &time.Time{}
	}
	if s.Eldt != nil {
		m.ArrivalETA = &models.ArrivalETA{Time: s.Eldt.AsTime(), Source: "AMAN"}
	}
	return m
}
func lifecycleModelAssignment(a *pb.StandAssignment, id int32, identity int64) *models.StandAssignment {
	return &models.StandAssignment{ID: identity, SessionID: id, Callsign: a.Callsign, Stand: a.Stand, Direction: a.Direction, Stage: a.Stage, Source: a.Source, RuleID: a.RuleId, Tier: a.Tier, MatchedVariant: a.MatchedVariant, ConflictReason: a.ConflictReason, ObservedStand: a.ObservedStand, ETA: lifecycleTime(a.Eta), ETASource: a.EtaSource, AssignedAt: lifecycleTime(a.AssignedAt), ExpiresAt: lifecycleTime(a.ExpiresAt), ProjectedReleaseAt: lifecycleTime(a.ProjectedReleaseAt), Manual: a.Manual, Acknowledged: a.Acknowledged, AcknowledgedAt: lifecycleTime(a.AcknowledgedAt), AcknowledgedBy: a.AcknowledgedBy, VatsimCID: a.VatsimCid, VatsimRevision: a.VatsimRevision, Version: int32(a.Revision), CreatedAt: a.CreatedAt.AsTime(), UpdatedAt: a.UpdatedAt.AsTime()}
}
func (c *VatsimLifecycleCandidate) lifecycleAssignment(a *models.StandAssignment, old *pb.EntitySnapshot, airport string) *pb.StandAssignment {
	value := &pb.StandAssignment{Callsign: a.Callsign, Stand: a.Stand, Direction: a.Direction, Stage: a.Stage, Source: a.Source, Actor: "vatsim-lifecycle", Confirmed: true, RuleId: a.RuleID, Tier: a.Tier, MatchedVariant: a.MatchedVariant, ConflictReason: a.ConflictReason, ObservedStand: a.ObservedStand, Eta: lifecycleTimestamp(a.ETA), EtaSource: a.ETASource, AssignedAt: lifecycleTimestamp(a.AssignedAt), ExpiresAt: lifecycleTimestamp(a.ExpiresAt), ProjectedReleaseAt: lifecycleTimestamp(a.ProjectedReleaseAt), Manual: a.Manual, Acknowledged: a.Acknowledged, AcknowledgedAt: lifecycleTimestamp(a.AcknowledgedAt), AcknowledgedBy: a.AcknowledgedBy, VatsimCid: a.VatsimCID, VatsimRevision: a.VatsimRevision, CreatedAt: timestamppb.New(a.CreatedAt), UpdatedAt: timestamppb.New(a.UpdatedAt), Revision: standRevision(old)}
	if old != nil {
		value.Actor = old.GetValue().GetStandAssignment().Actor
		value.Confirmed = old.GetValue().GetStandAssignment().Confirmed
	}
	if stand, ok := c.Stands.Stands.Lookup(airport, a.Stand); ok {
		value.BlockedStands = append([]string(nil), stand.Blocks...)
		if a.MatchedVariant != nil {
			for _, v := range stand.Variants {
				if fmt.Sprintf("%s:%s:%d", airport, stand.Name, v.Line) == *a.MatchedVariant {
					value.BlockedStands = append([]string(nil), v.Blocks...)
					break
				}
			}
		}
		sort.Strings(value.BlockedStands)
	}
	return value
}

// lifecycleEntities borrows records from the caller's detached planning snapshot.
// The planner treats them as immutable; mutation is confined to its model copies.
// Keeping sorted order preserves stable assignment IDs and allocation tie breaks.
func lifecycleEntities(state *cluster.Aggregate, kind pb.EntityKind) []*pb.EntitySnapshot {
	keys := make([]string, 0, len(state.Indexes[kind]))
	for key := range state.Indexes[kind] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entities := make([]*pb.EntitySnapshot, 0, len(keys))
	for _, key := range keys {
		entities = append(entities, state.Indexes[kind][key])
	}
	return entities
}
