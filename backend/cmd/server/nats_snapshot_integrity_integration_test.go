package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// Qualify the production snapshot reader against the pinned client's original
// upload path. Every upload has an immutable namespace, distinct chunks and an
// independent ACL connection. No failed read is retried or accepted as valid.
func TestServerNATSSnapshotMultiChunkIntegrity(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 fixture")
	}
	runSnapshotMultiChunkIntegrity(t, false)
}

// Keep the unmodified vendor read workload available as a strict diagnostic.
// Its known early end-of-delivery bug is not the production snapshot read path.
func TestServerNATSSnapshotVendorMultiChunkDiagnostic(t *testing.T) {
	if os.Getenv("NATS_TASK22_VENDOR_OBJECT_DIAGNOSTIC") != "1" {
		t.Skip("requires explicit vendor object-reader diagnostic")
	}
	runSnapshotMultiChunkIntegrity(t, true)
}

func runSnapshotMultiChunkIntegrity(t *testing.T, vendor bool) {
	t.Helper()
	f := newEntrypointFixture(t, true)
	const workers, attempts = 2, 24
	failures := make(chan error, workers)
	var jobs sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		jobs.Add(1)
		go func(worker int) {
			defer jobs.Done()
			config := f.projection.Config
			config.URLs = []string{config.URLs[worker%len(config.URLs)]}
			connection, err := natsresources.Connect(config)
			if err != nil {
				failures <- err
				return
			}
			defer connection.Close()
			js, err := connection.JetStream(nats.MaxWait(config.RequestTimeout))
			if err != nil {
				failures <- err
				return
			}
			objects, err := js.ObjectStore(config.Names.Objects)
			if err != nil {
				failures <- err
				return
			}
			readerProjection, err := cluster.NewProjection(connection, config)
			if err != nil {
				failures <- err
				return
			}
			for attempt := 0; attempt < attempts; attempt++ {
				if f.ctx.Err() != nil {
					failures <- f.ctx.Err()
					return
				}
				size := 708208
				if attempt%2 != 0 {
					size = 986409
				}
				data := make([]byte, size)
				for i := range data {
					data[i] = byte((i*31 + (i/(128*1024))*73 + worker*17 + attempt) % 251)
				}
				name := "snapshot-integrity/" + uuid.NewString()
				info, err := objects.PutBytes(name, data, nats.Context(f.ctx))
				if err != nil {
					failures <- fmt.Errorf("upload worker=%d attempt=%d name=%s: %w", worker, attempt, name, err)
					return
				}
				expectedDigest := sha256.Sum256(data)
				expectedChunks := uint32((len(data) + 128*1024 - 1) / (128 * 1024))
				metadataDigest := "SHA-256=" + base64.URLEncoding.EncodeToString(expectedDigest[:])
				if info == nil || info.Size != uint64(len(data)) || info.Chunks != expectedChunks || info.NUID == "" || info.Digest != metadataDigest {
					failures <- fmt.Errorf("upload metadata name=%s expected_size=%d info=%+v", name, len(data), info)
					return
				}
				var actual []byte
				var readErr, closeErr error
				if vendor {
					result, err := objects.Get(name, nats.Context(f.ctx))
					if err != nil {
						failures <- fmt.Errorf("vendor get name=%s nuid=%s chunks=%d size=%d digest=%s: %w", name, info.NUID, info.Chunks, info.Size, info.Digest, err)
						return
					}
					actual, readErr = io.ReadAll(result)
					closeErr = result.Close()
				} else {
					actual, readErr = readerProjection.Snapshots.Objects.GetBytes(name, nats.Context(f.ctx))
				}
				actualDigest := sha256.Sum256(actual)
				if readErr != nil || closeErr != nil || actualDigest != expectedDigest || len(actual) != len(data) {
					failures <- fmt.Errorf("read vendor=%t name=%s nuid=%s chunks=%d size=%d metadata_digest=%s expected_sha256=%x actual_size=%d actual_sha256=%x read_error=%v close_error=%v", vendor, name, info.NUID, info.Chunks, info.Size, info.Digest, expectedDigest, len(actual), actualDigest, readErr, closeErr)
					return
				}
			}
		}(worker)
	}
	done := make(chan struct{})
	go func() { jobs.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(90 * time.Second):
		t.Fatal("bounded immutable snapshot stress exceeded 90 seconds")
	}
	close(failures)
	for err := range failures {
		t.Logf("object stream/broker evidence follows integrity failure: %v", err)
		stream, streamErr := f.projection.JS.StreamInfo("OBJ_" + f.projection.Config.Names.Objects)
		if streamErr == nil {
			t.Logf("object stream state: %+v cluster: %+v", stream.State, stream.Cluster)
		} else {
			t.Logf("object stream metadata failure: %v", streamErr)
		}
		require.NoError(t, err)
	}
}
