package cluster

import (
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestProviderCurrentGenerationOnlyNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		var err error
		port, err = strconv.Atoi(raw)
		require.NoError(t, err)
	}
	urls := func(user string) []string {
		out := []string{}
		for i := 0; i < 3; i++ {
			out = append(out, fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+i))
		}
		return out
	}
	cfg := natsresources.Config{URLs: urls("bootstrap"), ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	defer admin.Close()
	require.NoError(t, natscluster.WaitForQuorum(ctx, admin))
	require.NoError(t, natsresources.Bootstrap(ctx, admin, cfg))
	cfg.URLs = urls("backend")
	seed := uuid.New()
	icao := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	ref := airportRef(icao)
	nc, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	defer nc.Close()
	projection := startProjection(t, ctx, nc, cfg)
	replica := startProjection(t, ctx, nc, cfg)
	owner, err := NewOwnerRuntime(nc, projection, NATSStore{JS: projection.JS})
	require.NoError(t, err)
	require.NoError(t, owner.Track(ref))
	go func() { _ = owner.Run(ctx) }()
	require.Eventually(t, func() bool { return owner.CanWrite(ref) }, 10*time.Second, 25*time.Millisecond)
	objects, err := projection.JS.ObjectStore("FS_OBJECTS")
	require.NoError(t, err)
	cache := NewVerifiedObjectCache(1024 * 1024)
	projection.SetObjectCache(cache)
	replicaCache := NewVerifiedObjectCache(1024 * 1024)
	replica.SetObjectCache(replicaCache)
	nav := NavigationWeather{Writer: Writer{Projection: projection, Store: NATSStore{JS: projection.JS}, Lease: owner, NodeID: owner.NodeID}, Objects: NATSObjects{Store: objects}, Cache: cache}
	at := time.Now().UTC().Truncate(time.Second)
	first, sha, err := nav.PublishProvider(feedPage(at))
	require.NoError(t, err)
	reply, err := nav.PutCheckpointFor(ctx, ref, uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: first, Sha256: sha})
	require.NoError(t, err)
	require.NoError(t, replica.WaitApplied(ctx, reply.GetStreamSequence()))
	_, _, err = nav.CheckpointFor(ctx, ref, "vatsim", "network-data/v3")
	require.NoError(t, err)
	follower := nav
	follower.Writer.Projection = replica
	follower.Cache = replicaCache
	_, _, err = follower.CheckpointFor(ctx, ref, "vatsim", "network-data/v3")
	require.NoError(t, err)
	next, nextSha, err := nav.PublishProvider(feedPage(at.Add(time.Minute)))
	require.NoError(t, err)
	reply, err = nav.PutCheckpointFor(ctx, ref, uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: next, Sha256: nextSha})
	require.NoError(t, err)
	require.NoError(t, replica.WaitApplied(ctx, reply.GetStreamSequence()))
	_, err = objects.GetBytes(first)
	require.ErrorIs(t, err, nats.ErrObjectNotFound)
	require.Nil(t, cache.get(first))
	require.Nil(t, replicaCache.get(first))
	recovered := startProjection(t, ctx, nc, cfg)
	follower.Writer.Projection = recovered
	follower.Cache = nil
	checkpoint, page, err := follower.CheckpointFor(ctx, ref, "vatsim", "network-data/v3")
	require.NoError(t, err)
	require.Equal(t, next, checkpoint.ObjectName)
	require.Equal(t, at.Add(time.Minute), page.GetVatsim().SnapshotAt.AsTime())
}
