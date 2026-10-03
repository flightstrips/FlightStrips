package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"testing"
)

type syncTestReader struct {
	state *cluster.Aggregate
	reads []string
}

func (r *syncTestReader) ReadEntity(_ *pb.AggregateRef, kind pb.EntityKind, key string) (*pb.EntitySnapshot, error) {
	r.reads = append(r.reads, key)
	return r.state.Indexes[kind][key], nil
}

func TestStripSyncOnlySendsChangedWireViewsAndRemovesObsoleteEntries(t *testing.T) {
	state := cluster.NewAggregate(candidateRef(1))
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{}
	for _, callsign := range []string{"SAS1", "SAS2"} {
		entity := &pb.EntitySnapshot{Key: callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Callsign: callsign, HasFlightPlan: true, Bay: "NOT_CLEARED", AssignedSquawk: "1001"}}}}
		state.Indexes[pb.EntityKind_STRIP][callsign] = entity
		state.Entities[callsign] = entity
	}
	sync := &socketStripSync{}
	require.Len(t, sync.initial(state).GetBackendSync().Strips, 2)
	reader := &syncTestReader{state: state}
	delta := &pb.FrontendDelta{Aggregate: state.Ref, Changes: []*pb.EntityChange{{Key: "SAS1", Operation: &pb.EntityChange_Upsert{Upsert: state.Indexes[pb.EntityKind_STRIP]["SAS1"].Value}}}}
	frame, err := sync.delta(reader, delta)
	require.NoError(t, err)
	require.Nil(t, frame, "unchanged assignments, including position-only changes, don't replay")
	for _, callsign := range reader.reads {
		require.Equal(t, "SAS1", callsign, "never read unrelated aircraft")
	}
	state.Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip().AssignedSquawk = "1002"
	frame, err = sync.delta(reader, delta)
	require.NoError(t, err)
	require.Len(t, frame.GetBackendSync().Strips, 1)
	require.Equal(t, "SAS1", frame.GetBackendSync().Strips[0].Callsign)
	require.Equal(t, "1002", frame.GetBackendSync().Strips[0].AssignedSquawk)
	state.Indexes[pb.EntityKind_CDM_STATE] = map[string]*pb.EntitySnapshot{"SAS1": {Value: &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: &pb.CdmState{Callsign: "SAS1", Deice: "REMOTE"}}}}}
	delta.Changes = []*pb.EntityChange{{Key: "SAS1", Operation: &pb.EntityChange_Upsert{Upsert: state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value}}}
	frame, err = sync.delta(reader, delta)
	require.NoError(t, err)
	require.Equal(t, "REMOTE", frame.GetBackendSync().Strips[0].Cdm.DeiceType)
	delta.Changes = []*pb.EntityChange{{Key: "SAS1", Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_STRIP}}}}
	_, err = sync.delta(reader, delta)
	require.NoError(t, err)
	require.NotContains(t, sync.latest, "SAS1")
	state.Indexes[pb.EntityKind_STRIP]["SAS2"].Value.GetStrip().Bay = "DEP_HIDDEN"
	delta.Changes = []*pb.EntityChange{{Key: "SAS2", Operation: &pb.EntityChange_Upsert{Upsert: state.Indexes[pb.EntityKind_STRIP]["SAS2"].Value}}}
	_, err = sync.delta(reader, delta)
	require.NoError(t, err)
	require.NotContains(t, sync.latest, "SAS2")
}
