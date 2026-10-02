package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// derivePositionStrip restores the position-driven movement policy without
// turning every surveillance observation into an FS_STATE strip replacement.
// It retains existing controller fields and only changes movement/landing facts.
func derivePositionStrip(old *pb.Strip, position *pb.AircraftPosition, airport string, at time.Time) *pb.Strip {
	if old == nil || position == nil {
		return nil
	}
	copy := proto.Clone(old).(*pb.Strip)
	model := models.Strip{Origin: old.Departure, Destination: old.Destination, Bay: old.Bay, State: &old.GroundState}
	copy.Bay = shared.GetDepartureBayFromPosition(position.Latitude, position.Longitude, int64(position.AltitudeFeet), model, config.GetAirborneAltitudeAGL(), airport)
	if strings.EqualFold(old.Destination, airport) {
		if copy.Bay == shared.BAY_UNKNOWN {
			copy.Bay = shared.BAY_ARR_HIDDEN
		}
		onGround := int64(position.AltitudeFeet) < int64(shared.AirportElevation)+config.GetLandingAltitudeAGL()
		region, inRunway := config.GetRunwayRegionForPosition(position.Latitude, position.Longitude)
		if inRunway && old.Runway != "" && !slices.Contains(region.Runways, strings.ToUpper(old.Runway)) {
			inRunway = false
		}
		if old.Aldt == nil && old.Runway != "" && (copy.Bay == shared.BAY_ARR_HIDDEN || copy.Bay == shared.BAY_AIRBORNE) {
			if final, ok := config.GetFinalApproachRegionForRunway(old.Runway, position.Latitude, position.Longitude); ok {
				distance := shared.GetDistance(position.Latitude, position.Longitude, final.ThresholdLat, final.ThresholdLon)
				if int64(position.AltitudeFeet) <= final.FinalApproachAltitudeCeiling(distance, int64(shared.AirportElevation)) {
					copy.Bay = shared.BAY_FINAL
				}
			}
		}
		if inRunway && onGround && old.Aldt == nil {
			copy.Aldt = timestamppb.New(at.UTC())
		}
		if !inRunway && onGround && old.Aldt != nil {
			switch copy.Bay {
			case shared.BAY_FINAL, shared.BAY_RWY_ARR, shared.BAY_ARR_HIDDEN:
				copy.Bay = shared.BAY_TWY_ARR
			}
		}
	}
	if proto.Equal(copy, old) {
		return nil
	}
	return copy
}

func (c *DeadlineCandidate) derivePosition(ctx context.Context, id int32, w *cluster.PositionWriter, key string, revision uint64, position *pb.AircraftPosition) error {
	ref := candidateRef(id)
	strip, err := c.Router.Projection.ReadEntity(ref, pb.EntityKind_STRIP, key)
	if err != nil {
		return err
	}
	if strip == nil {
		return fmt.Errorf("position strip unavailable")
	}
	session, err := c.Router.Projection.ReadEntity(ref, pb.EntityKind_SESSION, fmt.Sprint(id))
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("position session unavailable")
	}
	at := c.clock()
	if derivePositionStrip(strip.Value.GetStrip(), position, session.Value.GetSession().Airport, at) == nil {
		return nil
	}
	command, _ := cluster.ProviderEventCommandID("euroscope-position", w.Connection, fmt.Sprintf("%d/%s/%d/%d", id, key, w.OwnerEpoch, revision))
	// The source KV identity is encoded in the command identity. Planning reads
	// fresh domain fields on every subject-CAS retry; no stale strip replacement
	// or historical command copy can overwrite a concurrent controller action.
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-position"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: key}}}}}}}}
	writer := c.Router.Writer
	writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if err := w.Authority(ctx, id, w.OwnerEpoch, w.Connection); err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		old := state.Indexes[pb.EntityKind_STRIP][key]
		seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
		if old == nil || seed == nil {
			return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("position-derived state unavailable")
		}
		next := derivePositionStrip(old.Value.GetStrip(), position, seed.Airport, at)
		if next == nil {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
		}
		planned := proto.Clone(req).(*pb.CommandRequest)
		planned.ExpectedEntityRevision = &old.Revision
		planned.GetSystem().GetUpdateEntity().Value = &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: next}}
		return cluster.PlanStrip(ctx, planned, state)
	}
	_, err = w.ExecuteDerivedContext(ctx, key, revision, func(runCtx context.Context) (*pb.CommandReply, error) {
		reply := writer.Execute(runCtx, req)
		return reply, candidateReply(reply)
	})
	return err
}
