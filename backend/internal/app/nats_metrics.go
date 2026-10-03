package app

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"github.com/nats-io/nats.go"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type runtimeMetrics struct {
	mu            sync.Mutex
	pubackCount   uint64
	pubackSeconds float64
}
type measuredStore struct {
	cluster.EventStore
	metrics *runtimeMetrics
}

// Preserve the exact committed-receipt capability through the metrics adapter.
// Embedding EventStore alone exposes only Replay and Publish.
func (s measuredStore) Committed(ctx context.Context, sequence uint64) (cluster.AppliedEvent, error) {
	store, ok := s.EventStore.(interface {
		Committed(context.Context, uint64) (cluster.AppliedEvent, error)
	})
	if !ok {
		return cluster.AppliedEvent{}, fmt.Errorf("event store has no committed receipt reader")
	}
	return store.Committed(ctx, sequence)
}

func (s measuredStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	start := time.Now()
	sequence, err := s.EventStore.Publish(ctx, subject, expected, data)
	if err == nil {
		s.metrics.mu.Lock()
		s.metrics.pubackCount++
		s.metrics.pubackSeconds += time.Since(start).Seconds()
		s.metrics.mu.Unlock()
		// Log metadata only: accepted payloads may contain private messages and
		// provider data. Never serialize the event or transport error here.
		event := &pb.StateEvent{}
		if pb.UnmarshalStrict(data, event) == nil {
			masterEpoch := event.GetSessionSynced().GetMasterEpoch()
			if effect := event.GetEffectChanged(); effect != nil {
				masterEpoch = effect.MasterEpoch
			}
			slog.DebugContext(ctx, "NATS state accepted", "command_id", event.GetCommandId(), "sequence", sequence, "owner_epoch", event.OwnerEpoch, "master_epoch", masterEpoch)
		}
	}
	return sequence, err
}
func (r *natsRuntime) metricsHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	r.metrics.mu.Lock()
	count, seconds := r.metrics.pubackCount, r.metrics.pubackSeconds
	r.metrics.mu.Unlock()
	fmt.Fprintf(w, "# TYPE fs_nats_puback_seconds summary\nfs_nats_puback_seconds_sum{resource=\"FS_STATE\"} %g\nfs_nats_puback_seconds_count{resource=\"FS_STATE\"} %d\n", seconds, count)
	r.projection.WriteMetrics(w, r.owner.NodeID)
	ctx, cancel := context.WithTimeout(request.Context(), time.Second)
	defer cancel()
	names := r.projection.Config.Names
	for _, name := range []string{names.State, "KV_" + names.Positions, "KV_" + names.Presence, "KV_" + names.SnapshotIndex, "OBJ_" + names.Objects} {
		if info, err := r.projection.JS.StreamInfo(name, nats.Context(ctx)); err == nil {
			fmt.Fprintf(w, "fs_nats_storage_bytes{resource=%q} %d\n", name, info.State.Bytes)
		}
	}
}
