package cluster

import (
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func TestCommittedOwnerReplyBindsDurableOutcome(t *testing.T) {
	_, _, ref := fixture(t)
	request := command(ref, "session", 0)
	hash, err := RequestHash(request)
	if err != nil {
		t.Fatal(err)
	}
	seq, rev := uint64(3), uint64(1)
	outcome := &pb.CommandOutcome{CommandId: request.CommandId, RequestSha256: hash, Actor: proto.Clone(request.Actor).(*pb.Actor), Aggregate: proto.Clone(ref).(*pb.AggregateRef), CommittedStreamSequence: seq, AggregateRevision: rev, Status: pb.CommandOutcome_SUCCEEDED}
	reply := &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_COMMITTED, StreamSequence: &seq, AggregateRevision: &rev, Outcome: outcome}
	if !validCommittedReply(request, hash, reply) {
		t.Fatal("verified owner outcome rejected")
	}
	mutations := map[string]func(*pb.CommandReply){
		"bare puback":        func(r *pb.CommandReply) { r.Outcome = nil },
		"actor":              func(r *pb.CommandReply) { r.Outcome.Actor.Id = "another actor" },
		"request hash":       func(r *pb.CommandReply) { r.Outcome.RequestSha256 = "different" },
		"command":            func(r *pb.CommandReply) { r.Outcome.CommandId = "different" },
		"aggregate":          func(r *pb.CommandReply) { r.Outcome.Aggregate = nil },
		"sequence":           func(r *pb.CommandReply) { r.Outcome.CommittedStreamSequence++ },
		"revision":           func(r *pb.CommandReply) { r.Outcome.AggregateRevision++ },
		"status":             func(r *pb.CommandReply) { r.Status = pb.CommandReply_PENDING },
		"missing checkpoint": func(r *pb.CommandReply) { r.StreamSequence = nil },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := proto.Clone(reply).(*pb.CommandReply)
			mutate(changed)
			if validCommittedReply(request, hash, changed) {
				t.Fatal("unbound owner reply accepted")
			}
		})
	}
	accepted := proto.Clone(reply).(*pb.CommandReply)
	accepted.Status, accepted.Outcome.Status = pb.CommandReply_PENDING, pb.CommandOutcome_ACCEPTED
	if !validCommittedReply(request, hash, accepted) {
		t.Fatal("accepted provider effect rejected")
	}
}
