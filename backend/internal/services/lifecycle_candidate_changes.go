package services

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"sort"
	"time"
)

func lifecycleArrivalStateEqual(a, b *cluster.Aggregate) bool {
	left, right := a.Indexes[pb.EntityKind_STAND_ASSIGNMENT], b.Indexes[pb.EntityKind_STAND_ASSIGNMENT]
	if len(left) != len(right) {
		return false
	}
	for key, assignment := range left {
		if !proto.Equal(assignment, right[key]) {
			return false
		}
	}
	for key, strip := range a.Indexes[pb.EntityKind_STRIP] {
		if strip.GetValue().GetStrip().Stand != b.Indexes[pb.EntityKind_STRIP][key].GetValue().GetStrip().GetStand() {
			return false
		}
	}
	return true
}

const effectDispatchWindow = 30 * time.Second

func (c *VatsimLifecycleCandidate) clock() time.Time {
	if c.Stands.Now != nil {
		return c.Stands.Now().UTC()
	}
	return time.Now().UTC()
}
func sessionRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func globalRef() *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
}
func lifecycleID(seed, action string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed+"/"+action)).String()
}
func standRevision(old *pb.EntitySnapshot) uint64 {
	if old == nil {
		return 0
	}
	return old.Revision
}
func candidateUpsert(key string, old *pb.EntitySnapshot, value *pb.EntityRecord) *pb.EntityChange {
	return &pb.EntityChange{Key: key, Revision: standRevision(old) + 1, Operation: &pb.EntityChange_Upsert{Upsert: value}}
}
func candidateDelete(key string, old *pb.EntitySnapshot, kind pb.EntityKind) *pb.EntityChange {
	return &pb.EntityChange{Key: key, Revision: standRevision(old) + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: kind}}}
}
func stripChange(old *pb.EntitySnapshot, strip *pb.Strip) *pb.EntityChange {
	strip.Revision = standRevision(old) + 1
	return candidateUpsert(strip.Callsign, old, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}})
}
func equalStripWithoutRevision(a, b *pb.Strip) bool {
	copy := proto.Clone(b).(*pb.Strip)
	copy.Revision = a.Revision
	return proto.Equal(a, copy)
}
func sortCandidateChanges(changes []*pb.EntityChange) {
	kind := func(c *pb.EntityChange) int {
		if c.GetDelete() != nil {
			return int(c.GetDelete().Kind)
		}
		switch c.GetUpsert().Value.(type) {
		case *pb.EntityRecord_Strip:
			return int(pb.EntityKind_STRIP)
		case *pb.EntityRecord_StandAssignment:
			return int(pb.EntityKind_STAND_ASSIGNMENT)
		case *pb.EntityRecord_StandBlock:
			return int(pb.EntityKind_STAND_BLOCK)
		}
		return 0
	}
	sort.Slice(changes, func(i, j int) bool {
		if left, right := kind(changes[i]), kind(changes[j]); left != right {
			return left < right
		}
		return changes[i].Key < changes[j].Key
	})
}
