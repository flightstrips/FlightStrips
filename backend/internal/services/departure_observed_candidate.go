package services

import (
	"context"
	"fmt"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Non-live sessions derive physical departure occupancy from their own accepted
// radar observations. They never consume another session's VATSIM population.
func (c *VatsimLifecycleCandidate) observedDepartures(ctx context.Context, id int32) error {
	var err error
	if owners := c.Writer.Projection.Async; owners != nil {
		err = owners.Execute(ctx, sessionRef(id), func(turn context.Context) error {
			return c.observedDeparturesAccepted(turn, id)
		})
	} else {
		err = c.observedDeparturesAccepted(ctx, id)
	}
	if err != nil {
		return err
	}
	return c.resumeStandEffects(ctx, id)
}

func (c *VatsimLifecycleCandidate) observedDeparturesAccepted(ctx context.Context, id int32) error {
	positions, _, err := c.Writer.Projection.ObservationSnapshot(id)
	if err != nil {
		return err
	}
	var cached *cluster.Aggregate
	for _, observation := range positions {
		position := observation.Value.GetPosition()
		if observation.Stale || position == nil || position.AltitudeFeet > 1000 {
			continue
		}
		state, err := c.Writer.Projection.ReadLifecyclePlanningCached(sessionRef(id), cached)
		if err != nil {
			return err
		}
		cached = state
		key := observation.Value.AircraftKey
		strip := state.Indexes[pb.EntityKind_STRIP][key].GetValue().GetStrip()
		seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
		if strip == nil || seed == nil || seed.Tombstoned || strip.Departure != seed.Airport {
			continue
		}
		assignment := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT][key].GetValue().GetStandAssignment()
		physical, found := c.Stands.Stands.StandAtPosition(seed.Airport, position.Latitude, position.Longitude)
		if !found && assignment == nil {
			continue // Moving or airborne aircraft have no new stand decision.
		}
		if found && assignment.GetStage() == StageDepartureBlock && assignment.GetStand() == physical.Name && strip.Stand == physical.Name && assignment.GetConflictReason() == "" && strip.GroundState == "" && strip.Bay != "PUSH" && (assignment.ExpiresAt == nil || c.clock().Before(assignment.ExpiresAt.AsTime())) {
			continue // Accepted physical occupancy is unchanged.
		}
		command, _ := cluster.ProviderEventCommandID("euroscope-departure-lifecycle", "euroscope", fmt.Sprintf("%d/%s/%d/%s", id, key, state.Revision, lifecyclePositionTag(positions)))
		page := &pb.VatsimPage{SnapshotAt: timestamppb.New(c.clock())}
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: sessionRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-departure-lifecycle"}, ExpectedEntityRevision: &state.Revision,
			Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_VatsimSessionCursor{VatsimSessionCursor: &pb.VatsimSessionCursor{Provider: "euroscope", SourceRevision: state.Revision, SnapshotAt: page.SnapshotAt}}}}}}}}
		change, err := c.plan(ctx, request, state, key, true, state.Revision, "euroscope", page, positions, false)
		if err != nil {
			return err
		}
		if len(change.Changes) == 0 && len(change.Effects) == 0 && len(change.Workflows) == 0 {
			continue
		}
		writer := c.Writer
		writer.Plan = func(turn context.Context, request *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			if current.Revision != state.Revision {
				return nil, pb.CommandReply_UNAVAILABLE, current.Revision, fmt.Errorf("departure observations changed; rederive")
			}
			change, err := c.plan(turn, request, current, key, true, state.Revision, "euroscope", page, positions, true)
			return change, pb.CommandReply_COMMITTED, 0, err
		}
		if c.Positions == nil || c.Positions(id) == nil {
			continue // Wait for a current synchronized master before acting on radar.
		}
		_, err = c.Positions(id).ExecuteLifecycleContext(ctx, positions, func(turn context.Context) (*pb.CommandReply, error) {
			reply := writer.Execute(turn, request)
			return reply, lifecycleReply(reply)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
