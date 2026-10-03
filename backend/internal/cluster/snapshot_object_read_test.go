package cluster

import (
	"bytes"
	"errors"
	"io"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type diagnosticObjectStore struct {
	nats.ObjectStore
	result nats.ObjectResult
}

func (s diagnosticObjectStore) Get(string, ...nats.GetObjectOpt) (nats.ObjectResult, error) {
	return s.result, nil
}

type diagnosticObjectResult struct {
	reader      io.Reader
	err         error
	callbackErr error
	closed      bool
}

func (r *diagnosticObjectResult) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if err == io.EOF && r.err != nil {
		return n, r.err
	}
	return n, err
}
func (r *diagnosticObjectResult) Close() error { r.closed = true; return nil }
func (r *diagnosticObjectResult) Error() error {
	if r.callbackErr != nil {
		return r.callbackErr
	}
	return r.err
}
func (r *diagnosticObjectResult) Info() (*nats.ObjectInfo, error) {
	return &nats.ObjectInfo{NUID: "immutable-id", Chunks: 3, Size: 1234, Digest: "SHA-256=expected"}, nil
}

func TestSnapshotObjectReadDiagnosticsPreserveFailureAndDiscardPayload(t *testing.T) {
	secret := []byte("private aircraft payload")
	result := &diagnosticObjectResult{reader: bytes.NewReader(secret), err: nats.ErrDigestMismatch, callbackErr: errors.New("invalid delivery metadata")}
	store := snapshotObjectReader{ObjectStore: diagnosticObjectStore{result: result}}
	data, err := store.GetBytes("snapshot/session/1/10/immutable-object")
	require.Nil(t, data)
	require.ErrorIs(t, err, nats.ErrDigestMismatch)
	require.Contains(t, err.Error(), "nuid=immutable-id chunks=3 expected_size=1234")
	require.Contains(t, err.Error(), "actual_size=24 actual_sha256="+digest(secret))
	require.Contains(t, err.Error(), "result_error=invalid delivery metadata")
	require.NotContains(t, err.Error(), string(secret))
	require.True(t, result.closed)
}

func TestSnapshotObjectReadDiagnosticsSuccessfulAndBounded(t *testing.T) {
	result := &diagnosticObjectResult{reader: bytes.NewReader([]byte("valid"))}
	store := snapshotObjectReader{ObjectStore: diagnosticObjectStore{result: result}}
	data, err := store.GetBytes("immutable")
	require.NoError(t, err)
	require.Equal(t, []byte("valid"), data)
	result = &diagnosticObjectResult{reader: io.LimitReader(zeroSnapshotReader{}, MaxObjectBytes+2)}
	store.ObjectStore = diagnosticObjectStore{result: result}
	data, err = store.GetBytes("oversized")
	require.Nil(t, data)
	require.ErrorIs(t, err, nats.ErrBadObjectMeta)
	require.False(t, errors.Is(err, ErrSnapshotTooLarge), "stored corruption must never be treated as a deferred materialization")
	result.reader = io.LimitReader(zeroSnapshotReader{}, MaxObjectBytes+2)
	_, verifyErr := verifySnapshotObject(store, decodeSnapshotIndexForDiagnostic())
	require.ErrorIs(t, verifyErr, nats.ErrBadObjectMeta)
	require.ErrorIs(t, verifyErr, errInvalidSnapshotObject)
	require.True(t, result.closed)
}

func decodeSnapshotIndexForDiagnostic() *pb.SnapshotIndex {
	return &pb.SnapshotIndex{SchemaVersion: 1, Aggregate: sessionRef(1), ObjectName: "snapshot/session/1/10", LastStreamSequence: 10, LastSubjectSequence: 10}
}

type zeroSnapshotReader struct{}

func (zeroSnapshotReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }
