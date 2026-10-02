package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func localPositionValue() *pb.PositionValue {
	return &pb.PositionValue{SchemaVersion: 1, SessionId: 1, AircraftKey: "SAS1", OwnerEpoch: 2, SourceConnectionId: "master", ObservedAt: timestamppb.Now(), Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55, Longitude: 12}}}
}

func TestOwnerLocalPositionReplayKeepsRawFenceIndependent(t *testing.T) {
	p := readyPositionWaitFixture()
	p.Config.Names.Positions = "positions"
	p.positionCursor = positionCursor{consumer: "C1", retained: map[string]uint64{}}
	value := localPositionValue()
	now := time.Now()
	require.NoError(t, p.materializePositionLocked("1.SAS1.2", proto.Clone(value).(*pb.PositionValue), 10, now))
	require.Zero(t, p.positionCursor.appliedStream)
	require.False(t, p.materializedPositionsCaughtLocked(), "old proof cannot cover a newer local receipt")
	require.Empty(t, p.positionCursor.retained)
	replay := func(stream, consumer uint64, op string, v *pb.PositionValue) error {
		msg := nats.NewMsg("$KV.positions.1.SAS1.2")
		msg.Sub = &nats.Subscription{}
		msg.Reply = fmt.Sprintf("$JS.ACK.KV_positions.C1.1.%d.%d.%d.0", stream, consumer, now.UnixNano())
		if op != "" {
			msg.Header.Set("KV-Operation", op)
		} else {
			var err error
			msg.Data, err = proto.Marshal(v)
			require.NoError(t, err)
		}
		return p.applyPositionMessage(msg)
	}
	require.NoError(t, replay(7, 1, "", value))
	require.NoError(t, replay(8, 2, "DEL", nil))
	require.Equal(t, uint64(10), p.positions["1.SAS1.2"].Revision)
	require.Equal(t, uint64(8), p.positionCursor.appliedStream)
	require.Equal(t, uint64(8), p.positionCursor.retained["1.SAS1.2"])
	require.NoError(t, replay(10, 3, "", value))
	require.True(t, p.materializedPositionsCaughtLocked())
	require.Equal(t, now, p.positions["1.SAS1.2"].Observed)
	newer := proto.Clone(value).(*pb.PositionValue)
	newer.SourceConnectionId = "foreign"
	require.NoError(t, replay(11, 4, "", newer))
	require.Equal(t, "foreign", p.positions["1.SAS1.2"].Value.SourceConnectionId)
	require.NoError(t, replay(12, 5, "PURGE", nil))
	require.NotContains(t, p.positions, "1.SAS1.2")
	p.positionCursor.appliedStream = 11
	require.False(t, p.materializedPositionsCaughtLocked(), "deleted-key watermark must also be covered")
	p.positionCursor.appliedStream = 12
	require.NoError(t, p.materializePositionLocked("1.SAS1.2", value, 11, now))
	require.NotContains(t, p.positions, "1.SAS1.2", "old materialization must not resurrect a deleted key")
	require.Error(t, p.materializePositionLocked("1.SAS1.2", value, 12, now))
	require.NoError(t, p.materializePositionLocked("1.SAS1.2", value, 13, now))
	require.Equal(t, uint64(12), p.positionCursor.appliedStream)
}

func TestPositionWriterReportsSuccessOnlyAfterLocalApply(t *testing.T) {
	kv := &positionKVTest{values: map[string]positionKVEntry{}}
	w, err := NewPositionWriter(kv, 1, 2, "master", func(context.Context, int32, uint64, string) error { return nil }, 1, 8)
	require.NoError(t, err)
	defer w.Close(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	require.NoError(t, w.SetCommittedApply(func(_ context.Context, v *pb.PositionValue, revision uint64) error {
		require.Equal(t, uint64(1), revision)
		require.Equal(t, "SAS1", v.AircraftKey)
		close(entered)
		<-release
		return errors.New("authority changed after commit")
	}))
	result, err := w.QueuePosition(context.Background(), "SAS1", localPositionValue().GetPosition(), time.Now())
	require.NoError(t, err)
	<-entered
	select {
	case <-result:
		t.Fatal("success escaped before owner-local acceptance")
	default:
	}
	close(release)
	receipt := <-result
	require.ErrorContains(t, receipt.Err, "authority changed")
	require.Equal(t, uint64(1), receipt.Revision)
	require.Equal(t, uint64(1), w.revisions["1.SAS1.2"], "durable CAS predecessor survives local acceptance failure")
	require.Error(t, w.SetCommittedApply(func(context.Context, *pb.PositionValue, uint64) error { return nil }))
}

type committedPositionJSTest struct {
	nats.JetStreamContext
	message  *nats.RawStreamMsg
	info     *nats.StreamInfo
	afterGet func()
}

func (js committedPositionJSTest) GetMsg(string, uint64, ...nats.JSOpt) (*nats.RawStreamMsg, error) {
	if js.afterGet != nil {
		js.afterGet()
	}
	return js.message, nil
}
func (js committedPositionJSTest) StreamInfo(string, ...nats.JSOpt) (*nats.StreamInfo, error) {
	return js.info, nil
}

func TestCommittedPositionRejectsMetadataGenerationAndAuthorityChanges(t *testing.T) {
	for _, name := range []string{"nil metadata", "wrong subject", "wrong bytes", "recreated stream", "consumer reset", "replay unhealthy", "owner changed", "server lease expired"} {
		t.Run(name, func(t *testing.T) {
			p := readyPositionWaitFixture()
			created := time.Now().Add(-time.Minute)
			p.positionCursor = positionCursor{created: created, sub: &nats.Subscription{}, consumer: "C1"}
			p.Config.Names.Positions = "positions"
			value := localPositionValue()
			data, err := proto.Marshal(value)
			require.NoError(t, err)
			msg := &nats.RawStreamMsg{Subject: "$KV.positions.1.SAS1.2", Sequence: 7, Data: data, Time: time.Now()}
			js := committedPositionJSTest{message: msg, info: &nats.StreamInfo{Created: created}}
			want := "metadata mismatch"
			switch name {
			case "nil metadata":
				js.message = nil
			case "wrong subject":
				msg.Subject += "wrong"
			case "wrong bytes":
				altered := proto.Clone(value).(*pb.PositionValue)
				altered.SourceConnectionId = "foreign"
				msg.Data, err = proto.Marshal(altered)
				require.NoError(t, err)
				want = "value mismatch"
			case "recreated stream":
				js.info.Created = created.Add(time.Second)
				want = "stream generation mismatch"
			case "consumer reset":
				js.afterGet = func() { p.positionCursor.sub = &nats.Subscription{}; p.positionCursor.consumer = "C2" }
				want = "generation changed"
			case "replay unhealthy":
				js.afterGet = func() { p.positionReady = false }
				want = "replay incomplete"
			case "owner changed":
				want = "owner/master changed"
			case "server lease expired":
				p.states["fs.v1.state.session.1"] = &Aggregate{Owner: &pb.OwnerTerm{NodeId: "node", Epoch: 2, LeaseUntil: timestamppb.New(time.Now().Add(time.Minute))}, Master: &pb.MasterTerm{OwnerEpoch: 2, ConnectionId: "master"}}
				msg.Time = time.Now().Add(2 * time.Minute)
				want = "owner/master changed"
			}
			p.JS = js
			require.ErrorContains(t, p.ApplyCommittedPosition("node")(context.Background(), value, 7), want)
			require.Empty(t, p.positions)
			require.Empty(t, p.positionRevision)
			require.Zero(t, p.positionCursor.appliedStream)
		})
	}
}
