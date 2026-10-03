package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"strconv"
	"strings"
)

// Prune removes snapshot payloads older than both retained verified pointers.
// Sequence ordering protects concurrent uploads: an older object cannot replace
// a newer index under publishSnapshotIndex's monotonic CAS contract.
func (s SnapshotStore) Prune(ctx context.Context, ref *pb.AggregateRef) (int, error) {
	if s.Index == nil || s.Objects == nil {
		return 0, fmt.Errorf("snapshot retention unavailable")
	}
	key, err := snapshotKey(ref)
	if err != nil {
		return 0, err
	}
	entries, err := s.Index.History(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	cutoff := uint64(0)
	keep := map[string]bool{}
	for _, entry := range entries {
		index := &pb.SnapshotIndex{}
		if err := pb.UnmarshalStrict(entry.Value(), index); err != nil {
			return 0, err
		}
		if index.LastStreamSequence == 0 || index.ObjectName == "" {
			return 0, fmt.Errorf("invalid snapshot retention index")
		}
		keep[index.ObjectName] = true
		if cutoff == 0 || index.LastStreamSequence < cutoff {
			cutoff = index.LastStreamSequence
		}
	}
	if cutoff == 0 {
		return 0, nil
	}
	infos, err := s.Objects.List(nats.Context(ctx))
	if errors.Is(err, nats.ErrNoObjectsFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	prefix := "snapshot/" + strings.ReplaceAll(key, ".", "/") + "/"
	deleted := 0
	for _, info := range infos {
		if ctx.Err() != nil {
			return deleted, ctx.Err()
		}
		if info == nil || info.Deleted || keep[info.Name] || !strings.HasPrefix(info.Name, prefix) {
			continue
		}
		tail := strings.TrimPrefix(info.Name, prefix)
		sequence, err := strconv.ParseUint(strings.Split(tail, "/")[0], 10, 64)
		if err != nil || sequence >= cutoff {
			continue
		}
		if err := s.Objects.Delete(info.Name); err != nil && !errors.Is(err, nats.ErrObjectNotFound) {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
