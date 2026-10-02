package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

// Exercise the pinned client's original upload and ordered-consumer read path.
// Every upload has an immutable namespace, multiple distinct chunks and an
// independent reader. No failed read is retried or accepted as valid.
func TestServerNATSSnapshotMultiChunkIntegrity(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	const workers, attempts, size = 2, 24, 768 * 1024
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
			for attempt := 0; attempt < attempts; attempt++ {
				if f.ctx.Err() != nil {
					failures <- f.ctx.Err()
					return
				}
				data := make([]byte, size+attempt)
				for i := range data {
					data[i] = byte((i*31 + (i/(128*1024))*73 + worker*17 + attempt) % 251)
				}
				name := "snapshot-integrity/" + uuid.NewString()
				info, err := objects.PutBytes(name, data, nats.Context(f.ctx))
				if err != nil {
					failures <- fmt.Errorf("upload worker=%d attempt=%d name=%s: %w", worker, attempt, name, err)
					return
				}
				result, err := objects.Get(name, nats.Context(f.ctx))
				if err != nil {
					failures <- fmt.Errorf("get name=%s nuid=%s chunks=%d size=%d digest=%s: %w", name, info.NUID, info.Chunks, info.Size, info.Digest, err)
					return
				}
				actual, readErr := io.ReadAll(result)
				closeErr := result.Close()
				actualDigest, expectedDigest := sha256.Sum256(actual), sha256.Sum256(data)
				if readErr != nil || closeErr != nil || actualDigest != expectedDigest || len(actual) != len(data) {
					failures <- fmt.Errorf("read name=%s nuid=%s chunks=%d size=%d metadata_digest=%s expected_sha256=%x actual_size=%d actual_sha256=%x read_error=%v close_error=%v", name, info.NUID, info.Chunks, info.Size, info.Digest, expectedDigest, len(actual), actualDigest, readErr, closeErr)
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
