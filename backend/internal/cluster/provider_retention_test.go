package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

type cleanProviderObjects struct {
	*memoryObjects
	times  map[string]time.Time
	onRead func(string)
}

func (o *cleanProviderObjects) PutBytes(name string, data []byte) (*nats.ObjectInfo, error) {
	info, err := o.memoryObjects.PutBytes(name, data)
	o.times[name] = time.Now()
	return info, err
}
func (o *cleanProviderObjects) GetBytes(name string) ([]byte, error) {
	if o.onRead != nil {
		o.onRead(name)
	}
	return o.memoryObjects.GetBytes(name)
}
func (o *cleanProviderObjects) Delete(name string) error {
	o.remove(name)
	delete(o.times, name)
	return nil
}
func (o *cleanProviderObjects) List(context.Context) ([]*nats.ObjectInfo, error) {
	out := []*nats.ObjectInfo{}
	for name := range o.values {
		out = append(out, &nats.ObjectInfo{ObjectMeta: nats.ObjectMeta{Name: name}, ModTime: o.times[name]})
	}
	return out, nil
}
func feedPage(at time.Time) *pb.ProviderPage {
	return &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: timestamppb.New(at)}}}
}

func TestProviderCacheOnlyRetainsAcceptedGeneration(t *testing.T) {
	cache := NewVerifiedObjectCache(1024 * 1024)
	page := func() *pb.ObjectValue {
		return &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_ProviderPage{ProviderPage: feedPage(time.Now())}}
	}
	cache.put("staged", page(), 10)
	require.Nil(t, cache.get("staged"))
	cache.acceptProvider("global", "vatsim", "first", 1)
	cache.put("first", page(), 10)
	cache.acceptProvider("global", "vatsim", "next", 2)
	cache.put("next", page(), 10)
	require.Nil(t, cache.get("first"))
	require.NotNil(t, cache.get("next"))
	cache.acceptProvider("global", "vatsim", "first", 1)
	cache.put("first", page(), 10)
	require.Nil(t, cache.get("first"), "overlapping old reader must not repopulate stale data")
	cache.acceptProvider("airport", "other", "next", 1)
	cache.acceptProvider("global", "vatsim", "third", 3)
	require.NotNil(t, cache.get("next"), "another live checkpoint still needs the payload")
	cache.acceptProvider("airport", "other", "", 2)
	require.Nil(t, cache.get("next"))
}

func TestProviderReplacementDeletesBrokerPayloadAndPrunesLegacy(t *testing.T) {
	ctx := context.Background()
	_, objects, nav := navFixture(t)
	cleaner := &cleanProviderObjects{memoryObjects: objects, times: map[string]time.Time{}}
	nav.Objects = cleaner
	nav.Cache = NewVerifiedObjectCache(1024 * 1024)
	at := time.Now().UTC().Truncate(time.Second)
	first, sha, err := nav.PublishProvider(feedPage(at))
	require.NoError(t, err)
	_, err = nav.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: first, Sha256: sha})
	require.NoError(t, err)
	_, _, err = nav.CheckpointFor(ctx, globalRef(), "vatsim", "network-data/v3")
	require.NoError(t, err)
	next, nextSha, err := nav.PublishProvider(feedPage(at.Add(time.Minute)))
	require.NoError(t, err)
	require.NotNil(t, nav.Cache.get(first), "publication must not evict current accepted data")
	_, err = nav.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: next, Sha256: nextSha})
	require.NoError(t, err)
	_, err = objects.GetBytes(first)
	require.ErrorIs(t, err, nats.ErrObjectNotFound)
	require.Nil(t, nav.Cache.get(first))
	// Migrate old names while preserving newer staged uploads and unrelated data.
	legacyValue := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_ProviderPage{ProviderPage: feedPage(at.Add(-time.Minute))}}
	raw, err := proto.Marshal(legacyValue)
	require.NoError(t, err)
	legacy := "provider/vatsim/" + digest(raw)
	_, err = cleaner.PutBytes(legacy, raw)
	require.NoError(t, err)
	cleaner.times[legacy] = cleaner.times[next].Add(-time.Minute)
	staged, _, err := nav.PublishProvider(feedPage(at.Add(2 * time.Minute)))
	require.NoError(t, err)
	cleaner.times[staged] = cleaner.times[next].Add(time.Minute)
	unrelated := "nav/unrelated"
	_, err = cleaner.PutBytes(unrelated, []byte("nav"))
	require.NoError(t, err)
	count, err := nav.PruneProviderPages(ctx, globalRef())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, err = objects.GetBytes(legacy)
	require.ErrorIs(t, err, nats.ErrObjectNotFound)
	for _, name := range []string{next, staged, unrelated} {
		_, err = objects.GetBytes(name)
		require.NoError(t, err, fmt.Sprintf("deleted live/staged/unrelated %s", name))
	}
}

func TestProviderReadRefreshesAfterConcurrentReplacement(t *testing.T) {
	ctx := context.Background()
	_, objects, nav := navFixture(t)
	cleaner := &cleanProviderObjects{memoryObjects: objects, times: map[string]time.Time{}}
	nav.Objects = cleaner
	at := time.Now().UTC().Truncate(time.Second)
	first, sha, err := nav.PublishProvider(feedPage(at))
	require.NoError(t, err)
	_, err = nav.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: first, Sha256: sha})
	require.NoError(t, err)
	next, nextSha, err := nav.PublishProvider(feedPage(at.Add(time.Minute)))
	require.NoError(t, err)
	cleaner.onRead = func(name string) {
		if name != first {
			return
		}
		cleaner.onRead = nil
		_, err := nav.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: next, Sha256: nextSha})
		require.NoError(t, err)
	}
	checkpoint, page, revision, err := nav.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	require.NoError(t, err)
	require.Equal(t, next, checkpoint.ObjectName)
	require.Equal(t, uint64(2), revision)
	require.Equal(t, at.Add(time.Minute), page.GetVatsim().SnapshotAt.AsTime())
}

func TestExpiredWeatherCheckpointAndPayloadAreRemoved(t *testing.T) {
	ctx := context.Background()
	_, objects, nav := navFixture(t)
	cleaner := &cleanProviderObjects{memoryObjects: objects, times: map[string]time.Time{}}
	nav.Objects = cleaner
	nav.Cache = NewVerifiedObjectCache(1024 * 1024)
	now := time.Now().UTC().Truncate(time.Second)
	page := &pb.ProviderPage{Provider: "openmeteo", Resource: "old-weather", Parsed: &pb.ProviderPage_OpenMeteo{OpenMeteo: &pb.OpenMeteoPage{SourceId: "gfs", SourceRevision: "v1", ObservedAt: timestamppb.New(now.Add(-2 * time.Hour)), ExpiresAt: timestamppb.New(now.Add(-time.Hour)), Samples: []*pb.OpenMeteoSample{{ForecastAt: timestamppb.New(now), Levels: []*pb.OpenMeteoWindLevel{{AltitudeFeet: 1000}}}}}}}
	name, sha, err := nav.PublishProvider(page)
	require.NoError(t, err)
	_, err = nav.PutCheckpoint(ctx, "EKCH", uuid.NewString(), &pb.ProviderCheckpoint{Provider: page.Provider, Resource: page.Resource, ObjectName: name, Sha256: sha})
	require.NoError(t, err)
	_, _, err = nav.Checkpoint(ctx, "EKCH", page.Provider, page.Resource)
	require.NoError(t, err)
	_, err = nav.PruneProviderPages(ctx, airportRef("EKCH"))
	require.NoError(t, err)
	checkpoint, _, err := nav.Checkpoint(ctx, "EKCH", page.Provider, page.Resource)
	require.NoError(t, err)
	require.Nil(t, checkpoint)
	_, err = objects.GetBytes(name)
	require.ErrorIs(t, err, nats.ErrObjectNotFound)
	require.Nil(t, nav.Cache.get(name))
}
