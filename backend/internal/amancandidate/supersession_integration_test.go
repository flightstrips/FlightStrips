package amancandidate

import (
	"FlightStrips/internal/aman"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func TestOperationalSupersededDestinationNATS(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	first := f.owner(t, airportRef(f.airport), "")
	w := f.nodes[first].worker
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveVatsim(f.ctx, f.airport, "SAS123"), nil }))
	f.hold(t, w)
	source, err := w.options.Sessions.Read(f.ctx, airportRef(f.airport))
	require.NoError(t, err)
	require.Len(t, source.Workflows, 1)
	var intent *pb.WorkflowRecord
	for _, i := range source.Workflows {
		intent = i
	}
	step, err := w.BuildStep(f.ctx, intent)
	require.NoError(t, err)
	altered := proto.Clone(step).(*pb.CommandRequest)
	altered.CommandId = step.CommandId
	altered.GetSystem().GetApplyAmanSession().GetHoldingEat().HoldEat = "2359"
	rejected := w.options.Sessions.Execute(f.ctx, altered)
	require.Equal(t, pb.CommandOutcome_FAILED, rejected.GetOutcome().GetStatus())
	require.Equal(t, "INVALID_ARGUMENT", rejected.GetOutcome().ReasonCode)
	// A malformed attempt under the immutable UUID is terminal; use a separate
	// fresh accepted intent for the source-supersession fault scenario.
	f.now = f.now.Add(time.Second)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	source, err = w.options.Sessions.Read(f.ctx, airportRef(f.airport))
	require.NoError(t, err)
	for _, i := range source.Workflows {
		if *i.SourceRevision == source.Indexes[pb.EntityKind_AMAN_AIRPORT][f.airport].GetValue().GetAmanAirport().Revision {
			intent = i
		}
	}
	step, err = w.BuildStep(f.ctx, intent)
	require.NoError(t, err)
	before, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	stripRevision := before.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
	f.now = f.now.Add(time.Minute)
	f.vatsim(t, false)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveMissingVatsim(f.ctx, f.airport, "SAS123"), nil }))
	reply := w.options.Sessions.Execute(f.ctx, step)
	require.Equal(t, pb.CommandOutcome_FAILED, reply.GetOutcome().GetStatus())
	require.Equal(t, "REVISION_CONFLICT", reply.GetOutcome().ReasonCode)
	dead := f.nodes[first].owner.NodeID
	f.nodes[first].stop()
	f.nodes[first].nc.Close()
	second := f.owner(t, airportRef(f.airport), dead)
	f.owner(t, sessionRef(f.session), dead)
	w = f.nodes[second].worker
	require.NoError(t, w.Resume(f.ctx, f.airport))
	source, err = w.options.Sessions.Read(f.ctx, airportRef(f.airport))
	require.NoError(t, err)
	require.Equal(t, pb.WorkflowRecord_SUPERSEDED, source.Workflows[intent.WorkflowId].Status)
	after, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	require.Equal(t, stripRevision, after.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision)
	require.Empty(t, after.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip().HoldEat)
	// Replaying the stale UUID cannot acquire authority from a later board.
	committedReply := w.options.Sessions.Execute(f.ctx, step)
	require.Equal(t, pb.CommandOutcome_FAILED, committedReply.GetOutcome().GetStatus())
}

func TestOperationalRolloutGatesNATS(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	w := f.nodes[f.owner(t, airportRef(f.airport), "")].worker
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveVatsim(f.ctx, f.airport, "SAS123"), nil }))
	f.hold(t, w)
	for _, mode := range []aman.RolloutMode{aman.ModeShadow, aman.ModeReadOnly, aman.ModeDisabled} {
		w.options.Mode = mode
		f.now = f.now.Add(time.Second)
		committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
		b, err := w.options.State.Read(f.ctx, f.airport)
		require.NoError(t, err)
		require.Equal(t, string(mode), b.Airport.ConfiguredMode)
		require.False(t, b.Airport.Authoritative)
		require.Nil(t, b.Flights[0].HoldingEatProjection)
	}
	w.options.Mode = aman.ModeAuthoritative
	f.now = f.now.Add(3 * time.Hour)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	b, err := w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.False(t, b.Airport.Health.Ready)
	require.Nil(t, b.Flights[0].HoldingEatProjection)
}
