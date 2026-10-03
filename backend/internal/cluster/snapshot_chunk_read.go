package cluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type snapshotChunkBroker interface {
	SubscribeSync(string, ...nats.SubOpt) (*nats.Subscription, error)
	GetLastMsg(string, string, ...nats.JSOpt) (*nats.RawStreamMsg, error)
}
type snapshotChunkSubscription interface {
	NextMsgWithContext(context.Context) (*nats.Msg, error)
}

// ACK pending is an advisory consumer count, not an object boundary. The pinned
// ObjectStore.Get stops at pending=0 even when ObjectInfo advertises more chunks.
// Immutable snapshot completion requires every advertised chunk, exact bytes,
// the metadata digest and an exact stored tail. Index/typed validation follows.
func (s snapshotObjectReader) readImmutableChunks(name string, opts ...nats.GetObjectOpt) ([]byte, error) {
	ctx := context.Background()
	for _, option := range opts {
		if option == nil {
			continue
		}
		contextOption, ok := option.(nats.ContextOpt)
		if !ok {
			return nil, fmt.Errorf("unsupported immutable snapshot read option: %w", nats.ErrBadObjectMeta)
		}
		ctx = contextOption.Context
		if ctx == nil {
			ctx = context.Background()
		}
	}
	metadataCtx, cancelMetadata := context.WithTimeout(ctx, s.timeout)
	info, err := s.ObjectStore.GetInfo(name, nats.Context(metadataCtx))
	cancelMetadata()
	if err != nil {
		return nil, fmt.Errorf("snapshot object %s metadata: %w", name, err)
	}
	// Historical metadata may omit the default 128KiB chunk size. Detach the
	// metadata before normalizing it; the object-store namespace stays unchanged.
	if info != nil {
		copy := *info
		info = &copy
		if info.Opts == nil {
			info.Opts = &nats.ObjectMetaOptions{ChunkSize: 128 * 1024}
		} else {
			options := *info.Opts
			info.Opts = &options
			if info.Opts.ChunkSize == 0 {
				info.Opts.ChunkSize = 128 * 1024
			}
		}
	}
	if err = validateSnapshotChunkInfo(info, s.bucket, name); err != nil {
		return nil, err
	}
	stream, subject := "OBJ_"+s.bucket, "$O."+s.bucket+".C."+info.NUID
	var data []byte
	var lastSequence uint64
	if info.Chunks != 0 {
		sub, subscribeErr := subscribeSnapshotChunks(ctx, s.chunks, subject, stream, s.timeout)
		if subscribeErr != nil {
			return nil, subscribeErr
		}
		defer sub.Unsubscribe()
		data, lastSequence, err = readSnapshotChunks(ctx, sub, info, subject, s.timeout)
	} else {
		data, lastSequence, err = readSnapshotChunks(ctx, nil, info, subject, s.timeout)
	}
	// GetLastMsg also supplies payload-free diagnostics when a read stops short.
	// A failed read is never retried or replaced with the stored tail bytes.
	tailCtx, cancelTail := context.WithTimeout(ctx, s.timeout)
	tail, tailErr := s.chunks.GetLastMsg(stream, subject, nats.Context(tailCtx))
	cancelTail()
	if err != nil {
		if tailErr == nil && tail != nil {
			return nil, fmt.Errorf("snapshot object %s nuid=%s chunks=%d expected_size=%d metadata_digest=%s actual_size=%d actual_sha256=%s consumed_tail=%d stored_tail=%d stored_tail_size=%d stored_tail_sha256=%s: %w", name, info.NUID, info.Chunks, info.Size, info.Digest, len(data), digest(data), lastSequence, tail.Sequence, len(tail.Data), digest(tail.Data), err)
		}
		return nil, fmt.Errorf("snapshot object %s nuid=%s chunks=%d expected_size=%d actual_size=%d actual_sha256=%s stored_tail_error=%v: %w", name, info.NUID, info.Chunks, info.Size, len(data), digest(data), tailErr, err)
	}
	if err = checkSnapshotStoredTail(info, lastSequence, tail, tailErr); err != nil {
		return nil, err
	}
	return data, nil
}

func subscribeSnapshotChunks(ctx context.Context, broker snapshotChunkBroker, subject, stream string, timeout time.Duration) (*nats.Subscription, error) {
	// The pinned subscription retains Context() and automatically unsubscribes
	// when it expires. Use the configured JS MaxWait for broker setup instead;
	// bound the caller wait separately and clean up any late completed setup.
	setupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type setupResult struct {
		sub *nats.Subscription
		err error
	}
	ready := make(chan setupResult, 1)
	go func() {
		sub, err := broker.SubscribeSync(subject, nats.BindStream(stream), nats.DeliverAll(), nats.OrderedConsumer())
		ready <- setupResult{sub, err}
	}()
	select {
	case result := <-ready:
		if err := setupCtx.Err(); err != nil {
			if result.sub != nil {
				_ = result.sub.Unsubscribe()
			}
			return nil, err
		}
		return result.sub, result.err
	case <-setupCtx.Done():
		go func() {
			result := <-ready
			if result.sub != nil {
				_ = result.sub.Unsubscribe()
			}
		}()
		return nil, setupCtx.Err()
	}
}

func checkSnapshotStoredTail(info *nats.ObjectInfo, last uint64, tail *nats.RawStreamMsg, err error) error {
	if info.Chunks == 0 && errors.Is(err, nats.ErrMsgNotFound) {
		return nil
	}
	if err != nil && !errors.Is(err, nats.ErrMsgNotFound) {
		return fmt.Errorf("snapshot stored tail lookup: %w", err)
	}
	if info.Chunks == 0 || err != nil || tail == nil || tail.Sequence != last {
		return fmt.Errorf("snapshot stored chunk tail mismatch: %w", nats.ErrBadObjectMeta)
	}
	return nil
}

func validateSnapshotChunkInfo(info *nats.ObjectInfo, bucket, name string) error {
	if info == nil || info.Name != name || info.Bucket != bucket || info.NUID == "" || strings.ContainsAny(info.NUID, ".>* \t\r\n") || info.Deleted || info.Opts == nil || info.Opts.Link != nil || info.Opts.ChunkSize == 0 || info.Size > MaxObjectBytes {
		return fmt.Errorf("invalid immutable snapshot object metadata: %w", nats.ErrBadObjectMeta)
	}
	if uint64(info.Chunks) != (info.Size+uint64(info.Opts.ChunkSize)-1)/uint64(info.Opts.ChunkSize) {
		return fmt.Errorf("snapshot chunk count mismatch: %w", nats.ErrBadObjectMeta)
	}
	return nil
}

func readSnapshotChunks(ctx context.Context, sub snapshotChunkSubscription, info *nats.ObjectInfo, subject string, timeout time.Duration) ([]byte, uint64, error) {
	data := make([]byte, 0, int(info.Size))
	var previous uint64
	for chunk := uint32(0); chunk < info.Chunks; chunk++ {
		// Match ObjectResult.Read: a supplied deadline remains authoritative;
		// otherwise each progressing read receives the configured JS wait.
		readCtx, cancel := ctx, func() {}
		if _, suppliedDeadline := ctx.Deadline(); !suppliedDeadline {
			readCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		message, err := sub.NextMsgWithContext(readCtx)
		cancel()
		if err != nil {
			return data, previous, err
		}
		metadata, err := message.Metadata()
		if err != nil {
			return data, previous, err
		}
		remaining := info.Size - uint64(len(data))
		expectedSize := uint64(info.Opts.ChunkSize)
		if remaining < expectedSize {
			expectedSize = remaining
		}
		if message.Subject != subject || metadata.Sequence.Stream <= previous || uint64(len(message.Data)) != expectedSize {
			return data, previous, fmt.Errorf("snapshot chunk identity, order or size invalid: %w", nats.ErrBadObjectMeta)
		}
		previous = metadata.Sequence.Stream
		data = append(data, message.Data...)
	}
	expectedDigest, err := nats.DecodeObjectDigest(info.Digest)
	actualDigest := sha256.Sum256(data)
	if err != nil || uint64(len(data)) != info.Size || !bytes.Equal(expectedDigest, actualDigest[:]) {
		return data, previous, nats.ErrDigestMismatch
	}
	return data, previous, nil
}
