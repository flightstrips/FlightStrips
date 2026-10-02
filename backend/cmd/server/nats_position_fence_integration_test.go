package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Real retained KV metadata must prove the RAM view before lifecycle decisions.
// This fixture owns every broker and both compiled backend processes.
func TestServerNATSPositionMemoryFence(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit Task22 disposable fixture")
	}
	f := newEntrypointFixture(t, true)
	name, ref, _ := f.seededSession()
	state := f.state(ref)
	id, epoch, connection := ref.GetSession().Id, state.Owner.Epoch, state.Master.ConnectionId
	fence := func(observations []cluster.KVPosition) error {
		ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
		defer cancel()
		return f.projection.FenceLifecyclePositions(ctx, id, epoch, connection, observations)
	}
	snapshot := func() []cluster.KVPosition {
		positions, _, err := f.projection.ObservationSnapshot(id)
		require.NoError(t, err)
		return positions
	}
	put := func(callsign string, tombstone bool) {
		value := &pb.PositionValue{SchemaVersion: 1, SessionId: id, AircraftKey: callsign, OwnerEpoch: epoch, SourceConnectionId: connection, ObservedAt: timestamppb.Now()}
		if tombstone {
			value.Observation = &pb.PositionValue_Tombstone{Tombstone: &pb.PositionTombstone{}}
		} else {
			value.Observation = &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55.63, Longitude: 12.65}}
		}
		data, err := proto.Marshal(value)
		require.NoError(t, err)
		revision, err := f.projection.Positions.Put(fmt.Sprintf("%d.%s.%d", id, callsign, epoch), data)
		require.NoError(t, err)
		require.NoError(t, f.projection.WaitPositionApplied(f.ctx, id, callsign, epoch, revision))
	}
	require.NoError(t, fence(nil), "initial empty ordered projection")
	require.Error(t, f.projection.FenceLifecyclePositions(f.ctx, id, epoch+1, connection, nil), "empty input cannot bypass owner epoch")
	require.Error(t, f.projection.FenceLifecyclePositions(f.ctx, id, epoch, "foreign-master", nil), "empty input cannot bypass master generation")
	put("SAS123", false)
	first := snapshot()
	require.Len(t, first, 1)
	require.NoError(t, fence(first))
	put("SAS124", false)
	require.Error(t, fence(first), "new neighbor must invalidate earlier planning inputs")
	both := snapshot()
	require.Len(t, both, 2)
	require.NoError(t, fence(both))
	put("SAS124", true)
	require.Error(t, fence(both), "disconnect revision must invalidate earlier planning inputs")
	withDisconnect := snapshot()
	require.NoError(t, fence(withDisconnect))
	require.NoError(t, f.projection.Positions.Delete(fmt.Sprintf("%d.SAS124.%d", id, epoch)))
	f.await("physical KV deletion applied", func() bool { return len(snapshot()) == 1 })
	require.Error(t, fence(withDisconnect), "missing tagged observation must fail closed")
	require.NoError(t, fence(snapshot()), "retained delete marker remains part of ordered cursor proof")
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	require.Error(t, f.projection.FenceLifecyclePositions(cancelled, id, epoch, connection, snapshot()))
	require.Error(t, f.projection.FenceLifecyclePositions(f.ctx, id, epoch+1, connection, snapshot()), "foreign owner epoch")
	require.Error(t, f.projection.FenceLifecyclePositions(f.ctx, id, epoch, "foreign-master", snapshot()), "foreign master generation")

	adminConfig := f.resources
	adminConfig.URLs = append([]string(nil), f.resources.URLs...)
	for i, url := range adminConfig.URLs {
		adminConfig.URLs[i] = strings.Replace(url, "backend:backend-local-only", "bootstrap:bootstrap-local-only", 1)
	}
	admin, err := natsresources.Connect(adminConfig)
	require.NoError(t, err)
	defer admin.Close()
	js, err := admin.JetStream()
	require.NoError(t, err)
	before, err := js.StreamInfo("KV_FS_POSITIONS")
	require.NoError(t, err)
	require.NoError(t, js.PurgeStream("KV_FS_POSITIONS", &nats.StreamPurgeRequest{Subject: fmt.Sprintf("$KV.FS_POSITIONS.%d.SAS123.%d", id, epoch)}))
	after, err := js.StreamInfo("KV_FS_POSITIONS")
	require.NoError(t, err)
	require.Equal(t, before.State.LastSeq, after.State.LastSeq, "silent selective purge leaves tail sequence unchanged")
	require.Less(t, after.State.Msgs, before.State.Msgs)
	require.Error(t, fence(snapshot()), "silent retention mutation must not validate stale RAM occupancy")

	// A new incarnation must still bootstrap a legitimately empty retained
	// stream whose historical tail is nonzero, without inventing position data.
	require.NoError(t, js.PurgeStream("KV_FS_POSITIONS"))
	replayed, err := cluster.NewProjection(f.nc, f.resources)
	require.NoError(t, err)
	replayCtx, stop := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	go func() { done <- replayed.Run(replayCtx) }()
	defer func() { stop(); <-done }()
	f.await("empty retained stream replay with historical tail", func() bool { return replayed.Ready() == nil })
	positions, _, err := replayed.ObservationSnapshot(id)
	require.NoError(t, err)
	require.Empty(t, positions)
	require.Error(t, replayed.FenceLifecyclePositions(f.ctx, id, epoch, connection, nil), "new incarnation must reject retained sync")
	globalRef := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	priorGlobal, err := replayed.Read(globalRef)
	require.NoError(t, err)
	f.stopProjection()
	f.projection = replayed
	for _, app := range f.apps {
		app.stop()
	}
	for i := range f.apps {
		f.apps[i] = startFixtureProcess(t, f.binary, f.backend, f.env, "-addr", f.addresses[i])
	}
	f.ready()
	f.await("fresh session and registry owners after backend restart", func() bool {
		session, sessionErr := replayed.Read(ref)
		global, globalErr := replayed.Read(globalRef)
		return sessionErr == nil && globalErr == nil && session.Owner.GetEpoch() > epoch && global.Owner.GetEpoch() > priorGlobal.Owner.GetEpoch()
	})
	plugins := f.concurrentPlugins(name)
	f.await("restarted backend elects current master", func() bool {
		current, err := replayed.Read(ref)
		return err == nil && current.Master.GetCid() != "" && current.Master.OwnerEpoch == current.Owner.Epoch
	})
	f.assertOneMaster(ref, plugins)
	current := f.state(ref)
	master := 0
	if current.Master.Cid == "222222" {
		master = 1
	}
	sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: id, CommandId: uuid.NewString(), OwnerEpoch: current.Owner.Epoch, MasterEpoch: current.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: []*es.Strip{{Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA", AircraftType: "A320", Runway: "22R", AssignedSquawk: "1001", HasFp: true}}, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}}}})
	f.await("fresh sync permits empty memory fence after restart", func() bool {
		return replayed.FenceLifecyclePositions(f.ctx, id, current.Owner.Epoch, current.Master.ConnectionId, nil) == nil
	})
	t.Logf("POSITION_MEMORY_FENCE neighbor_addition_disconnect_delete_purge_cancel_terms=true historical_tail=%d", before.State.LastSeq)
}
