package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPositionCursorProofRequiresAppliedOrderedGeneration(t *testing.T) {
	now := time.Now()
	sub := &nats.Subscription{}
	fixture := func() (*Projection, *nats.StreamInfo, *nats.ConsumerInfo) {
		return &Projection{positionCursor: positionCursor{sub: sub, consumer: "C1", created: now, appliedConsumer: 2, appliedStream: 10, retained: map[string]uint64{"a": 8, "b": 10}}}, &nats.StreamInfo{Created: now, Config: nats.StreamConfig{MaxMsgsPerSubject: 1, MaxMsgs: -1, MaxBytes: -1}, State: nats.StreamState{LastSeq: 10, Msgs: 2}}, &nats.ConsumerInfo{Name: "C1", Delivered: nats.SequenceInfo{Consumer: 2, Stream: 10}}
	}
	for _, name := range []string{"compacted gaps", "unapplied delivery", "scan behind", "postcapture applied", "generation reset", "silent purge", "stream recreated", "TTL changed", "empty purged tail"} {
		t.Run(name, func(t *testing.T) {
			p, info, ci := fixture()
			wantReady := false
			wantError := false
			switch name {
			case "compacted gaps":
				wantReady = true
			case "unapplied delivery":
				ci.Delivered.Consumer = 3
			case "scan behind":
				ci.Delivered.Stream = 9
			case "postcapture applied":
				p.positionCursor.appliedStream = 11
			case "generation reset":
				ci.Name = "C2"
				wantError = true
			case "silent purge":
				info.State.Msgs = 1
				wantError = true
			case "stream recreated":
				info.Created = now.Add(time.Second)
				wantError = true
			case "TTL changed":
				info.Config.MaxAge = time.Second
				wantError = true
			case "empty purged tail":
				p.positionCursor.appliedConsumer = 0
				p.positionCursor.appliedStream = 0
				p.positionCursor.retained = map[string]uint64{}
				info.State.Msgs = 0
				ci.Delivered.Consumer = 0
				wantReady = true
			}
			caught, err := p.verifyPositionCursorLocked(sub, info, ci)
			require.Equal(t, wantReady, caught)
			if wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestLifecycleRAMSetIncludesNewNeighboursAndTombstones(t *testing.T) {
	item := func(aircraft, conn string, revision uint64) KVPosition {
		return KVPosition{Value: &pb.PositionValue{SessionId: 1, OwnerEpoch: 2, AircraftKey: aircraft, SourceConnectionId: conn}, Revision: revision}
	}
	p := &Projection{positions: map[string]KVPosition{"1.SAS1.2": item("SAS1", "master", 7)}}
	tagged := []KVPosition{item("SAS1", "master", 7)}
	require.NoError(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
	p.positions["1.SAS2.2"] = item("SAS2", "master", 9)
	require.Error(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
	tagged = append(tagged, p.positions["1.SAS2.2"])
	require.NoError(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
	p.positions["1.SAS3.2"] = item("SAS3", "foreign", 11)
	require.NoError(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
	delete(p.positions, "1.SAS1.2")
	require.Error(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
	p.positions["1.SAS1.2"] = item("SAS1", "foreign", 7)
	require.Error(t, p.compareLifecyclePositionsLocked(1, 2, "master", tagged))
}
func TestSnapshotStorageFailureWakesPositionAndStateWaiters(t *testing.T) {
	state := make(chan struct{})
	cursor := make(chan struct{})
	key := make(chan struct{})
	p := &Projection{lastSnapshot: map[string]time.Time{}, stateChanged: state, positionCursor: positionCursor{changed: cursor}, positionWaiters: map[string]*positionWaitNotification{"1.SAS1.2": {changed: key}}}
	p.finishSnapshot("session.1", errors.New("storage unavailable"))
	for _, ch := range []chan struct{}{state, cursor, key} {
		select {
		case <-ch:
		default:
			t.Fatal("health failure did not wake waiter")
		}
	}
}
