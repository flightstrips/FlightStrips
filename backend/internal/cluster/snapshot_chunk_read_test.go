package cluster

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type scriptedSnapshotChunks struct {
	messages []*nats.Msg
	reads    int
}

func (s *scriptedSnapshotChunks) NextMsgWithContext(ctx context.Context) (*nats.Msg, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.reads++
	if len(s.messages) == 0 {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	message := s.messages[0]
	s.messages = s.messages[1:]
	return message, nil
}
func snapshotChunkFixture() (*nats.ObjectInfo, []byte, []*nats.Msg) {
	data := make([]byte, 708208)
	for i := range data {
		data[i] = byte((i*31 + i/(128*1024)) % 251)
	}
	hash := sha256.New()
	_, _ = hash.Write(data)
	info := &nats.ObjectInfo{ObjectMeta: nats.ObjectMeta{Name: "snapshot/session/1/5661", Opts: &nats.ObjectMetaOptions{ChunkSize: 128 * 1024}}, Bucket: "objects", NUID: "immutable", Size: uint64(len(data)), Chunks: 6, Digest: nats.GetObjectDigestValue(hash)}
	var messages []*nats.Msg
	for i := 0; i < 6; i++ {
		end := (i + 1) * 128 * 1024
		if end > len(data) {
			end = len(data)
		}
		messages = append(messages, &nats.Msg{Sub: &nats.Subscription{}, Subject: "$O.objects.C.immutable", Data: append([]byte(nil), data[i*128*1024:end]...), Reply: fmt.Sprintf("$JS.ACK.OBJ_objects.C1.1.%d.%d.%d.0", 100+i, i+1, time.Now().UnixNano())})
	}
	return info, data, messages
}
func TestSnapshotChunkReaderIgnoresPrematurePendingZero(t *testing.T) {
	info, expected, messages := snapshotChunkFixture()
	// Every reply advertises pending=0, including the fifth full chunk that
	// reproduced the pinned Get reader's 655360-byte premature EOF.
	sub := &scriptedSnapshotChunks{messages: messages}
	actual, last, err := readSnapshotChunks(context.Background(), sub, info, "$O.objects.C.immutable", time.Second)
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	require.Equal(t, uint64(105), last)
	require.Equal(t, 6, sub.reads)
	require.NoError(t, checkSnapshotStoredTail(info, last, &nats.RawStreamMsg{Sequence: last}, nil))
}
func TestSnapshotChunkReaderRejectsMissingTailCorruptionAndDuplicates(t *testing.T) {
	for _, kind := range []string{"missing", "bad-hash", "duplicate", "wrong-subject", "wrong-size", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			info, _, messages := snapshotChunkFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "missing":
				messages = messages[:5]
			case "bad-hash":
				messages[5].Data[0] ^= 1
			case "duplicate":
				messages[5].Reply = messages[4].Reply
			case "wrong-subject":
				messages[5].Subject = "$O.objects.C.foreign"
			case "wrong-size":
				messages[5].Data = messages[5].Data[:1]
			case "cancelled":
				cancel()
			}
			actual, _, err := readSnapshotChunks(ctx, &scriptedSnapshotChunks{messages: messages}, info, "$O.objects.C.immutable", 5*time.Millisecond)
			require.Error(t, err)
			if kind == "missing" {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.Len(t, actual, 655360)
			}
			if kind == "bad-hash" {
				require.ErrorIs(t, err, nats.ErrDigestMismatch)
			}
			if kind == "duplicate" {
				require.ErrorIs(t, err, nats.ErrBadObjectMeta)
			}
			if kind == "cancelled" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}
func TestSnapshotStoredTailRejectsExtraChunkAndPreservesTransportError(t *testing.T) {
	info, _, _ := snapshotChunkFixture()
	require.ErrorIs(t, checkSnapshotStoredTail(info, 105, &nats.RawStreamMsg{Sequence: 106}, nil), nats.ErrBadObjectMeta)
	require.ErrorIs(t, checkSnapshotStoredTail(info, 105, nil, nats.ErrMsgNotFound), nats.ErrBadObjectMeta)
	require.ErrorIs(t, checkSnapshotStoredTail(info, 105, nil, nats.ErrTimeout), nats.ErrTimeout)
}
