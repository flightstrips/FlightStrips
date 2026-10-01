package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

type CdmPushbackResult struct {
	Tsat, Ctot          string
	Completed, Verified bool
}

// PushbackResult is a projection read. Waiting for accepted work may be local;
// all attempts, matches and acknowledgement decisions are persisted by owners.
func (s *CdmActionService) PushbackResult(ctx context.Context, id int32, callsign, operation string) (CdmPushbackResult, error) {
	state, err := s.Store.Read(ctx, sessionRef(id))
	if err != nil {
		return CdmPushbackResult{}, err
	}
	data := state.Indexes[pb.EntityKind_CDM_STATE][callsign].GetValue().GetCdmState()
	if data.GetPushback().GetOperationId() != operation {
		return CdmPushbackResult{}, fmt.Errorf("pushback operation superseded or unavailable")
	}
	return CdmPushbackResult{Tsat: cdmValue(cdmClock(data.Tsat)), Ctot: cdmValue(cdmClock(data.Ctot)), Completed: data.Pushback.Completed && data.Recalculation == pb.CdmState_NONE, Verified: data.Pushback.Verified}, nil
}

func (c *CdmCandidate) firePushback(ctx context.Context, id int32, deadline *pb.EntitySnapshot) error {
	state, err := c.Writer.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	seed, err := cdmSeed(state, id)
	if err != nil {
		return err
	}
	d := deadline.GetValue().GetSessionDeadline()
	key := strings.TrimPrefix(d.Id, "cdm-pushback/")
	data := state.Indexes[pb.EntityKind_CDM_STATE][key].GetValue().GetCdmState()
	if data == nil || data.Pushback == nil {
		return fmt.Errorf("pushback verification state unavailable")
	}
	if data.Recalculation != pb.CdmState_NONE || len(data.PendingExports) != 0 {
		return nil
	}
	export, err := state.LookupWorkflow(lifecycleID(data.SourceRevision, "viff/"+key+"/state"))
	if err != nil {
		return err
	}
	proven := export != nil && export.Status == pb.WorkflowRecord_COMPLETED && proto.Equal(data.Tobt, data.Pushback.ExpectedTobt)
	var flights *pb.ViffFlightPage
	var flightRevision uint64
	if proven && c.usesViff(seed) {
		// Each slot is persisted before the read and retains Task 19's UUID.
		_, probeErr := c.Reads.Flight(ctx, seed.Airport, id, key, d.DueAt.AsTime())
		if probeErr != nil {
			if err := c.admission(id); err != nil {
				return err
			}
			proven = false // A failed read cannot establish an acknowledgement.
		}
		state, err = c.Writer.Read(ctx, sessionRef(id))
		if err != nil {
			return err
		}
		if proven {
			flights, flightRevision, err = c.Reads.ReadFlights(ctx, id, key)
			if err != nil || flights == nil {
				return fmt.Errorf("pushback vIFF page unavailable: %v", err)
			}
		}
	}
	page, configRevision, err := c.Config.Read(ctx, seed.Airport)
	if err != nil {
		return err
	}
	config, err := clusterCdmConfig(page, seed)
	if err != nil {
		return err
	}
	positions, _, err := c.Writer.Projection.ObservationSnapshot(id)
	if err != nil {
		return err
	}
	tag := lifecyclePositionTag(positions)
	request := cdmSystem(id, lifecycleID(d.CommandId, fmt.Sprintf("apply/%d/%d/%d/%d/%s", deadline.Revision, state.Revision, configRevision, flightRevision, tag)), state.Revision, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Kind: pb.EntityKind_SESSION_DEADLINE, Key: d.Id}}})
	w := c.Writer
	w.Plan = func(ctx context.Context, req *pb.CommandRequest, current *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		old := current.Indexes[pb.EntityKind_CDM_STATE][key]
		value := old.GetValue().GetCdmState()
		if current.Revision != *req.ExpectedEntityRevision || !proto.Equal(current.Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id], deadline) || value.GetPushback().GetOperationId() != data.Pushback.OperationId {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("pushback inputs changed; rederive")
		}
		change := &pb.DomainChange{}
		if proven {
			change, err = c.plan(ctx, id, current, seed, config, flights, positions, "cdm-sync", req.CommandId, c.clock())
			if err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, 0, err
			}
		}
		copy := proto.Clone(value).(*pb.CdmState)
		for _, e := range change.Changes {
			if e.Key == key && e.GetUpsert().GetCdmState() != nil {
				copy = e.GetUpsert().GetCdmState()
			}
		}
		verification := copy.Pushback
		verification.Attempts++
		matched := false
		for _, row := range flights.GetFlights() {
			if row.Callsign != key {
				continue
			}
			wanted := cdmValue(cdmClock(verification.ExpectedTobt))
			for _, clock := range []string{row.Tobt, row.GetCdmData().GetTobt()} {
				if len(clock) >= 4 && clock[:4] == wanted {
					matched = true
				}
			}
		}
		if matched {
			verification.Matches++
		} else {
			verification.Matches = 0
		}
		verification.Verified = matched && (copy.Ctot != nil || verification.Matches >= 2)
		verification.Completed = !proven || verification.Verified || verification.Attempts >= 3
		filtered := change.Changes[:0]
		for _, e := range change.Changes {
			if e.Key != key || e.GetUpsert().GetCdmState() == nil {
				filtered = append(filtered, e)
			}
		}
		change.Changes = append(filtered, candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: copy}}))
		if verification.Completed {
			change.Changes = append(change.Changes, candidateDelete(d.Id, deadline, pb.EntityKind_SESSION_DEADLINE))
		} else {
			next := cdmDeadline(id, "cdm-pushback", c.clock().Add(500*time.Millisecond), current.Revision+1)
			next.Id = d.Id
			next.CommandId = lifecycleID(verification.OperationId, fmt.Sprintf("pushback/probe/%d", verification.Attempts+1))
			change.Changes = append(change.Changes, candidateUpsert(d.Id, deadline, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: next}}))
		}
		fresh, rev, err := c.Config.Read(ctx, seed.Airport)
		if err != nil || rev != configRevision || !proto.Equal(fresh, page) {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("pushback config changed")
		}
		if proven {
			fresh, rev, err := c.Reads.ReadFlights(ctx, id, key)
			if err != nil || rev != flightRevision || !proto.Equal(fresh, flights) {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("pushback vIFF page changed")
			}
		}
		freshPositions, _, err := c.Writer.Projection.ObservationSnapshot(id)
		if err != nil || lifecyclePositionTag(freshPositions) != tag {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("pushback positions changed")
		}
		cdmSort(change.Changes)
		return change, pb.CommandReply_COMMITTED, 0, nil
	}
	return cdmReply(w.Execute(ctx, request))
}
