package cluster

import (
	"context"
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
	timeout time.Duration
}

func (s snapshotObjectReader) GetBytes(name string, opts ...nats.GetObjectOpt) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	options := append([]nats.GetObjectOpt{nats.Context(ctx)}, opts...)
	result, err := s.ObjectStore.Get(name, options...)
	if err != nil {
		return nil, fmt.Errorf("snapshot object %s lookup: %w", name, err)
	}
	info, _ := result.Info()
	data, readErr := io.ReadAll(io.LimitReader(result, MaxObjectBytes+1))
	closeErr := result.Close()
	if readErr == nil && len(data) > MaxObjectBytes {
		readErr = ErrSnapshotTooLarge
	}
	if readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		if info == nil {
			return nil, fmt.Errorf("snapshot object %s read actual_size=%d actual_sha256=%s result_error_type=%T: %w", name, len(data), digest(data), result.Error(), readErr)
		}
		return nil, fmt.Errorf("snapshot object %s read nuid=%s chunks=%d expected_size=%d metadata_digest=%s actual_size=%d actual_sha256=%s result_error_type=%T: %w", name, info.NUID, info.Chunks, info.Size, info.Digest, len(data), digest(data), result.Error(), readErr)
	}
	return data, nil
}
