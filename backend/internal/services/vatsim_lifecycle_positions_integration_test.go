package services

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"testing"
	"time"
)

func (h *lifecycleHarness) operationalPositions(index int, cid string) *cluster.PositionWriter {
	h.t.Helper()
	n := h.nodes[index]
	lease, err := cluster.NewSocketPresenceLease(n.projection.Presence, n.owner.NodeID, h.id, cid, "EKCH_DEL", "DEL", false, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		h.t.Fatal(err)
	}
	socketCtx, stop := context.WithCancel(h.ctx)
	h.t.Cleanup(stop)
	go func() { _ = lease.Run(socketCtx) }()
	h.await("operational client", func() bool {
		_, presence, err := n.projection.ObservationSnapshot(h.id)
		if err != nil {
			return false
		}
		for _, o := range presence {
			if c := o.Value.GetClient(); c != nil && c.ConnectionId == lease.Client.ConnectionId {
				return true
			}
		}
		return false
	})
	writer := n.candidate.Writer
	writer.Plan = cluster.MasterElectionPlanner(n.projection, cluster.PlanSessionObservations)
	router := &cluster.CommandRouter{NC: n.nc, Projection: n.projection, Lease: n.owner, Writer: writer}
	election := cluster.MasterElection{Projection: n.projection, Router: router, Lease: n.owner}
	master, err := election.Reconcile(h.ctx, h.id)
	if err != nil {
		h.t.Fatal(err)
	}
	observations := cluster.SessionObservations{Store: cluster.LocalLifecycleStore{Writer: writer}}
	reply, err := observations.RecordSync(h.ctx, h.id, uuid.NewString(), &pb.SessionSync{ConnectionId: master.ConnectionId, MasterEpoch: master.Epoch, CompletedAt: timestamppb.Now()})
	if err != nil || lifecycleReply(reply) != nil {
		h.t.Fatalf("record sync %v %v", reply, err)
	}
	positions, err := cluster.NewPositionWriter(n.projection.Positions, h.id, h.state().Owner.Epoch, master.ConnectionId, n.projection.PositionAuthority(n.owner.NodeID), 2, 64)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = positions.Close(ctx)
	})
	n.candidate.Positions = func(int32) *cluster.PositionWriter { return positions }
	return positions
}
func (h *lifecycleHarness) position(writer *cluster.PositionWriter, key string, lat, lon float64) {
	h.t.Helper()
	var revision uint64
	h.await("admitted position report", func() bool {
		result, err := writer.QueuePosition(h.ctx, key, &pb.AircraftPosition{Latitude: lat, Longitude: lon, AltitudeFeet: 0}, time.Now())
		if err != nil {
			if !lifecycleTransient(err) {
				h.t.Fatal(err)
			}
			return false
		}
		written := <-result
		if written.Err != nil {
			if !lifecycleTransient(written.Err) {
				h.t.Fatal(written.Err)
			}
			return false
		}
		revision = written.Revision
		return true
	})
	n := h.nodes[h.owner(sessionRef(h.id))]
	h.await("tagged position", func() bool {
		values, _, err := n.projection.ObservationSnapshot(h.id)
		if err != nil {
			return false
		}
		for _, o := range values {
			if o.Value.AircraftKey == key && o.Revision == revision && !o.Stale {
				return true
			}
		}
		return false
	})
}

func TestVatsimLifecycleTwoReplicaSharedPositionsAndStandEffectRecovery(t *testing.T) {
	h := newLifecycleHarness(t)
	dep := lifecycleFlight("SAS123", "EKCH", "EDDF", "online")
	h.put(dep)
	h.callbacks()
	h.edit("SAS123", func(s *pb.Strip) { s.VatsimOnly = false; s.Bay = "NOT_CLEARED"; s.GroundState = "STAND" })
	first := h.owner(sessionRef(h.id))
	n := h.nodes[first]
	positions := h.operationalPositions(first, "777777")
	stand, _ := n.candidate.Stands.Stands.Lookup("EKCH", "A1")
	// Provider says away from the airport; accepted shared EuroScope position
	// establishes occupancy. The real callback must prefer that observation.
	h.position(positions, "SAS123", stand.Latitude, stand.Longitude)
	captured, _, err := n.projection.ObservationSnapshot(h.id)
	if err != nil {
		t.Fatal(err)
	}
	h.position(positions, "UNASSIGNED", 50, 8)
	ran := false
	if _, err = positions.ExecuteLifecycle(h.ctx, captured, func() (*pb.CommandReply, error) {
		ran = true
		return &pb.CommandReply{Status: pb.CommandReply_COMMITTED}, nil
	}); err == nil || ran {
		t.Fatal("barrier accepted a newly admitted aircraft absent from the input")
	}
	fault := &lifecycleFaultStore{EventStore: n.candidate.Writer.Store, after: true, kill: func() { n.stop(); n.nc.Close() }}
	n.candidate.Writer.Store = fault
	h.await("shared-position transition failure boundary", func() bool {
		err := n.work.Departure(h.ctx, h.id)
		if fault.fired {
			return true
		}
		if err != nil && !lifecycleTransient(err) {
			t.Fatal(err)
		}
		return false
	})
	second := 1 - first
	if h.owner(sessionRef(h.id)) != second {
		t.Fatal("wrong takeover")
	}
	state := h.state()
	a := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"].GetValue().GetStandAssignment()
	if a == nil || a.Stage != StageDepartureBlock || a.Stand != "A1" || a.ExpiresAt != nil {
		t.Fatalf("lost shared-position block: %v", a)
	}
	pending := 0
	for _, w := range state.Workflows {
		if w.Status == pb.WorkflowRecord_PENDING && strings.HasPrefix(w.Step, "vatsim-stand/") {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("transition lost durable STAND intent: %d", pending)
	}
	// Takeover has no valid old master sync. Retain the physical booking until
	// the new master has supplied authoritative shared position evidence.
	h.now = h.now.Add(time.Hour)
	h.callbacks()
	state = h.state()
	if state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] == nil {
		t.Fatal("stale takeover position released physical occupancy")
	}
	count := 0
	for _, effect := range state.Effects {
		if payload := effect.GetSetFlightPlan(); payload != nil && payload.Field == "STAND" {
			count++
			if effect.TargetCid != "777777" || payload.Value != "A1" {
				t.Fatal("stand intent retargeted after takeover")
			}
		}
	}
	if count != 1 {
		t.Fatalf("want one durable STAND effect: %d", count)
	}
	newPositions := h.operationalPositions(second, "888888")
	h.position(newPositions, "SAS123", 50, 8)
	h.callbacks()
	state = h.state()
	if state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil {
		t.Fatal("fresh shared position did not release departed stand")
	}
	if state.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip().Stand != "A1" {
		t.Fatal("vacating cleared departure route stand")
	}
	h.put() // Still operational in EuroScope, absent from the provider feed.
	h.position(newPositions, "SAS123", stand.Latitude, stand.Longitude)
	h.callbacks()
	if a := h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"].GetValue().GetStandAssignment(); a == nil || a.Stage != StageDepartureBlock {
		t.Fatal("position-only EuroScope departure was not observed")
	}
	h.edit("SAS123", func(s *pb.Strip) { s.GroundState = "PUSH" })
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil {
		t.Fatal("PUSH did not release departure occupancy")
	}
}
