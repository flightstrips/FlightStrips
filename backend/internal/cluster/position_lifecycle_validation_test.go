package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type lifecycleFenceKV struct {
	*positionKVTest
	statsMu                      sync.Mutex
	getCalls, keysCalls          int
	createEntered, createRelease chan struct{}
}

func (kv *lifecycleFenceKV) Get(key string) (nats.KeyValueEntry, error) {
	kv.statsMu.Lock()
	kv.getCalls++
	kv.statsMu.Unlock()
	return kv.positionKVTest.Get(key)
}
func (kv *lifecycleFenceKV) Keys(...nats.WatchOpt) ([]string, error) {
	kv.statsMu.Lock()
	kv.keysCalls++
	kv.statsMu.Unlock()
	return nil, errors.New("lifecycle must not enumerate remote position keys")
}
func (kv *lifecycleFenceKV) Create(key string, data []byte) (uint64, error) {
	if kv.createEntered != nil {
		close(kv.createEntered)
		<-kv.createRelease
	}
	return kv.positionKVTest.Create(key, data)
}
func TestLifecycleProjectionFenceMustBeConfiguredAndCannotBeRebound(t *testing.T) {
	kv := &lifecycleFenceKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}}
	writer, err := NewPositionWriter(kv, 12, 1, "conn", func(context.Context, int32, uint64, string) error { return nil }, 2, 8)
	require.NoError(t, err)
	callback := false
	_, err = writer.ExecuteLifecycle(context.Background(), nil, func() (*pb.CommandReply, error) { callback = true; return nil, nil })
	require.ErrorContains(t, err, "projection fence is unavailable")
	require.False(t, callback)
	require.Error(t, writer.SetLifecycleFence(nil))
	fence := func(context.Context, int32, uint64, string, []KVPosition) error { return nil }
	require.NoError(t, writer.SetLifecycleFence(fence))
	require.Error(t, writer.SetLifecycleFence(fence))
	require.NoError(t, writer.Close(context.Background()))
	require.Error(t, writer.SetLifecycleFence(fence))
	require.Zero(t, kv.getCalls)
	require.Zero(t, kv.keysCalls)
}
func TestLifecycleProjectionFenceErrorPreventsConditionalPublication(t *testing.T) {
	for _, failure := range []error{errors.New("position watcher behind"), errors.New("unobserved aircraft"), errors.New("position deleted"), context.Canceled} {
		kv := &lifecycleFenceKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}}
		writer, err := NewPositionWriter(kv, 12, 1, "conn", func(context.Context, int32, uint64, string) error { return nil }, 2, 8)
		require.NoError(t, err)
		observations := []KVPosition{{Value: &pb.PositionValue{SessionId: 12, AircraftKey: "SAS123", OwnerEpoch: 1, SourceConnectionId: "conn"}, Revision: 7}}
		require.NoError(t, writer.SetLifecycleFence(func(ctx context.Context, session int32, epoch uint64, connection string, got []KVPosition) error {
			require.Equal(t, int32(12), session)
			require.Equal(t, uint64(1), epoch)
			require.Equal(t, "conn", connection)
			require.Equal(t, observations, got)
			return failure
		}))
		published := false
		_, err = writer.ExecuteLifecycle(context.Background(), observations, func() (*pb.CommandReply, error) { published = true; return nil, nil })
		require.ErrorIs(t, err, failure)
		require.False(t, published)
		require.Zero(t, kv.getCalls, "verified lifecycle must not fetch remote positions")
		require.Zero(t, kv.keysCalls, "verified lifecycle must not enumerate remote positions")
		require.NoError(t, writer.Close(context.Background()))
	}
}
func TestLifecycleProjectionFenceDrainsAndPausesPositionWritesThroughPublication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	kv := &lifecycleFenceKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}, createEntered: make(chan struct{}), createRelease: make(chan struct{})}
	writer, err := NewPositionWriter(kv, 12, 1, "conn", func(context.Context, int32, uint64, string) error { return nil }, 2, 8)
	require.NoError(t, err)
	defer writer.Close(context.Background())
	fenceEntered, releaseFence := make(chan struct{}), make(chan struct{})
	require.NoError(t, writer.SetLifecycleFence(func(context.Context, int32, uint64, string, []KVPosition) error {
		// The first accepted write must finish before verifying the materialization.
		kv.mu.Lock()
		_, written := kv.values["12.SAS123.1"]
		kv.mu.Unlock()
		if !written {
			return errors.New("fence ran before admitted position drained")
		}
		close(fenceEntered)
		<-releaseFence
		return nil
	}))
	first, err := writer.QueuePosition(ctx, "SAS123", &pb.AircraftPosition{Latitude: 55, Longitude: 12}, time.Now())
	require.NoError(t, err)
	<-kv.createEntered
	completed := make(chan error, 1)
	published := false
	go func() {
		_, err := writer.ExecuteLifecycle(ctx, nil, func() (*pb.CommandReply, error) {
			published = true
			return &pb.CommandReply{Status: pb.CommandReply_COMMITTED}, nil
		})
		completed <- err
	}()
	// A blocked accepted KV write is evidence the owner drain has not completed.
	select {
	case <-fenceEntered:
		t.Fatal("fence ran while an admitted KV write was blocked")
	default:
	}
	close(kv.createRelease)
	require.NoError(t, (<-first).Err)
	<-fenceEntered
	second, err := writer.QueuePosition(ctx, "SAS123", &pb.AircraftPosition{Latitude: 56, Longitude: 13}, time.Now())
	require.NoError(t, err)
	select {
	case <-second:
		t.Fatal("new position executed while verified lifecycle fence was paused")
	default:
	}
	close(releaseFence)
	require.NoError(t, <-completed)
	require.True(t, published)
	require.NoError(t, (<-second).Err)
	kv.statsMu.Lock()
	defer kv.statsMu.Unlock()
	require.Equal(t, 1, kv.getCalls, "only initial position admission may Get its starting CAS revision")
	require.Zero(t, kv.keysCalls)
}
