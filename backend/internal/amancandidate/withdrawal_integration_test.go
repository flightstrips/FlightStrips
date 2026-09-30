package amancandidate

import (
	"FlightStrips/internal/aman"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

func currentIntent(t *testing.T, w *Worker, airport string) *pb.WorkflowRecord {
	t.Helper()
	state, err := w.options.Sessions.Read(t.Context(), airportRef(airport))
	require.NoError(t, err)
	board, err := w.options.State.Read(t.Context(), airport)
	require.NoError(t, err)
	for _, i := range state.Workflows {
		if i.Status == pb.WorkflowRecord_PENDING && i.GetSourceRevision() == board.Airport.Revision {
			return i
		}
	}
	t.Fatal("no current accepted AMAN intent")
	return nil
}

func TestOperationalHoldingWithdrawalNATS(t *testing.T) {
	for _, edited := range []bool{false, true} {
		name := "owned-eat"
		if edited {
			name = "later-controller-eat"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(t)
			w := f.nodes[f.owner(t, airportRef(f.airport), "")].worker
			committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveVatsim(f.ctx, f.airport, "SAS123"), nil }))
			f.hold(t, w)
			step, err := w.BuildStep(f.ctx, currentIntent(t, w, f.airport))
			require.NoError(t, err)
			committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.options.Sessions.Execute(f.ctx, step), nil }))
			session, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
			require.NoError(t, err)
			s := proto.Clone(session.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()).(*pb.Strip)
			require.NotNil(t, s.AmanWrittenHoldingEat)
			if edited {
				s.HoldEat = "2359"
				s.Remarks = "later controller EAT"
				f.entity(t, sessionRef(f.session), s.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: s}})
			}
			w.options.Mode = aman.ModeShadow
			f.now = f.now.Add(time.Second)
			committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
			withdrawal, err := w.BuildStep(f.ctx, currentIntent(t, w, f.airport))
			require.NoError(t, err)
			require.Empty(t, withdrawal.GetSystem().GetApplyAmanSession().GetHoldingEat().HoldEat)
			committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.options.Sessions.Execute(f.ctx, withdrawal), nil }))
			session, err = w.options.Sessions.Read(f.ctx, sessionRef(f.session))
			require.NoError(t, err)
			result := session.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()
			require.Nil(t, result.AmanWrittenHoldingEat)
			require.Equal(t, "TESPI", result.Hold)
			if edited {
				require.Equal(t, "2359", result.HoldEat)
				require.Equal(t, "later controller EAT", result.Remarks)
				require.Len(t, session.Effects, 1)
			} else {
				require.Empty(t, result.HoldEat)
				require.Len(t, session.Effects, 2)
				require.Empty(t, session.Effects[withdrawal.CommandId].GetAmanHoldingEat().Eat)
			}
			before := result.Revision
			committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.options.Sessions.Execute(f.ctx, withdrawal), nil }))
			session, err = w.options.Sessions.Read(f.ctx, sessionRef(f.session))
			require.NoError(t, err)
			require.Equal(t, before, session.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision)
		})
	}
}
