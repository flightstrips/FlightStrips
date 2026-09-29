package natscluster

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

// WaitForQuorum proves that all three JetStream peers can host a replicated
// file stream. It uses a disposable probe, not production resource metadata.
func WaitForQuorum(ctx context.Context, nc *nats.Conn) error {
	js, err := nc.JetStream(nats.MaxWait(2 * time.Second))
	if err != nil {
		return err
	}
	name := "FS_QUORUM_PROBE_" + uuid.NewString()[:8]
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	created := false
	var lastErr error
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("three-node JetStream quorum: %w (last error: %v)", err, lastErr)
		}
		var info *nats.StreamInfo
		// A restarted fixture already has the required stream; waiting for its
		// replicas avoids trying to create a new stream during metadata replay.
		if existing, existingErr := js.StreamInfo("FS_STATE"); existingErr == nil {
			allReady := ready(existing)
			for _, stream := range []string{"KV_FS_POSITIONS", "KV_FS_PRESENCE", "KV_FS_SNAPSHOT_INDEX", "OBJ_FS_OBJECTS"} {
				peer, peerErr := js.StreamInfo(stream)
				if peerErr == nil && !ready(peer) {
					allReady = false
				}
				if peerErr != nil && !errors.Is(peerErr, nats.ErrStreamNotFound) {
					allReady = false
					lastErr = peerErr
				}
			}
			if allReady {
				return nil
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("required stream replicas not current")
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("three-node JetStream quorum: %w (last error: %v)", ctx.Err(), lastErr)
			case <-ticker.C:
				continue
			}
		} else if !errors.Is(existingErr, nats.ErrStreamNotFound) {
			lastErr = existingErr
			select {
			case <-ctx.Done():
				return fmt.Errorf("three-node JetStream quorum: %w (last error: %v)", ctx.Err(), lastErr)
			case <-ticker.C:
				continue
			}
		}
		if created {
			info, err = js.StreamInfo(name)
		} else {
			info, err = js.AddStream(&nats.StreamConfig{Name: name, Subjects: []string{"fs.quorum.probe." + name}, Storage: nats.FileStorage, Replicas: 3})
			if err == nil {
				created = true
				defer js.DeleteStream(name)
			} else if errors.Is(err, nats.ErrStreamNameAlreadyInUse) {
				created = true
				defer js.DeleteStream(name)
				info, err = js.StreamInfo(name)
			}
		}
		if err == nil && ready(info) {
			return nil
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("three-node JetStream quorum: %w (last error: %v)", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func ready(info *nats.StreamInfo) bool {
	return info != nil && info.Cluster != nil && info.Cluster.Leader != "" && len(info.Cluster.Replicas) == 2 && info.Cluster.Replicas[0].Current && info.Cluster.Replicas[1].Current
}
