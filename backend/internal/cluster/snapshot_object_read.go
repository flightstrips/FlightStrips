package cluster

import (
	"fmt"
	"io"
	"time"

	"github.com/nats-io/nats.go"
)

// snapshotObjectReader retains the pinned client's object digest validation and
// discards every failed read. Metadata and byte counts distinguish a corrupt
// stored object from a truncated or duplicated consumer delivery without logging
// any snapshot payload. Abstract unit stores keep their own GetBytes behavior.
type snapshotObjectReader struct {
	nats.ObjectStore
	chunks  snapshotChunkBroker
	bucket  string
	timeout time.Duration
}

func (s snapshotObjectReader) GetBytes(name string, opts ...nats.GetObjectOpt) ([]byte, error) {
	if s.chunks != nil {
		return s.readImmutableChunks(name, opts...)
	}
	// Keep the underlying client's per-read deadline and caller context. A new
	// total deadline would reject large healthy objects that keep making progress.
	result, err := s.ObjectStore.Get(name, opts...)
	if err != nil {
		return nil, fmt.Errorf("snapshot object %s lookup: %w", name, err)
	}
	info, _ := result.Info()
	data, readErr := io.ReadAll(io.LimitReader(result, MaxObjectBytes+1))
	closeErr := result.Close()
	if readErr == nil && len(data) > MaxObjectBytes {
		// This is invalid stored content, not a legitimate oversized snapshot
		// materialization that the checkpoint scheduler may safely defer.
		readErr = fmt.Errorf("stored snapshot exceeds size bound: %w", nats.ErrBadObjectMeta)
	}
	if readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		if info == nil {
			return nil, fmt.Errorf("snapshot object %s read actual_size=%d actual_sha256=%s result_error=%v: %w", name, len(data), digest(data), result.Error(), readErr)
		}
		return nil, fmt.Errorf("snapshot object %s read nuid=%s chunks=%d expected_size=%d metadata_digest=%s actual_size=%d actual_sha256=%s result_error=%v: %w", name, info.NUID, info.Chunks, info.Size, info.Digest, len(data), digest(data), result.Error(), readErr)
	}
	return data, nil
}
