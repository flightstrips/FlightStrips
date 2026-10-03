package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
)

type positionProfileOwner struct {
	Node         int     `json:"fixture_node"`
	PID          int     `json:"pid"`
	NodeID       string  `json:"node_id"`
	Epoch        uint64  `json:"owner_epoch"`
	SessionID    int32   `json:"session_id"`
	LeaseSeconds float64 `json:"lease_remaining_seconds"`
}

// Both profilers are configured before any session login. Delayed selection
// never restarts the elected owner or changes the workload to obtain a profile.
func profileOwnerPositionLoad(t *testing.T, f *entrypointFixture, delay time.Duration) {
	addresses := make([]string, len(f.apps))
	listeners := make([]net.Listener, len(f.apps))
	for node := range f.apps {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		listeners[node], addresses[node] = listener, listener.Addr().String()
		defer listener.Close()
	}
	for node := range f.apps {
		f.apps[node].stop()
		if err := listeners[node].Close(); err != nil {
			t.Fatal(err)
		}
		env := replaceFixtureEnv(f.env, map[string]string{"ENABLE_PPROF": "true", "PPROF_ADDR": addresses[node], "PPROF_MUTEX_FRACTION": "5", "PPROF_BLOCK_RATE": "1000000"})
		f.apps[node] = startFixtureProcess(t, f.binary, f.backend, env, "-addr", f.addresses[node])
	}
	f.ready()
	binarySHA := hashFixtureBinary(t, f.binary)
	head, err := exec.Command("git", "-C", f.backend, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	diff, err := exec.Command("git", "-C", f.backend, "diff", "--binary", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	diffSHA := sha256.Sum256(diff)
	done := make(chan error, 1)
	go func() {
		time.Sleep(delay)
		owner, err := selectPositionProfileOwner(f)
		if err != nil {
			done <- err
			return
		}
		t.Logf("POSITION_OWNER_PROFILE node=%d pid=%d node_id=%s epoch=%d address=%s delay=%s", owner.Node, owner.PID, owner.NodeID, owner.Epoch, addresses[owner.Node], delay)
		dir := os.Getenv("NATS_TASK23_OUTPUT")
		if err := os.MkdirAll(dir, 0700); err != nil {
			done <- err
			return
		}
		var jobs sync.WaitGroup
		errors := make(chan error, 5)
		for _, artifact := range []struct {
			kind, endpoint string
			offset         time.Duration
		}{
			{"cpu", "profile?seconds=30", 0},
			{"mutex", "mutex?seconds=30", 0},
			{"block", "block?seconds=30", 0},
			{"trace", "trace?seconds=5", 10 * time.Second},
			{"goroutines.txt", "goroutine?debug=2", 10 * time.Second},
		} {
			jobs.Add(1)
			go func(kind, endpoint string, offset time.Duration) {
				defer jobs.Done()
				time.Sleep(offset)
				started := time.Now()
				response, err := (&http.Client{Timeout: 45 * time.Second}).Get("http://" + addresses[owner.Node] + "/debug/pprof/" + endpoint)
				if err != nil {
					errors <- err
					return
				}
				data, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				finished := time.Now()
				if err == nil && response.StatusCode != http.StatusOK {
					err = fmt.Errorf("%s profile HTTP status %d", kind, response.StatusCode)
				}
				if err == nil {
					err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("backend-%d.%s", owner.Node, kind)), data, 0600)
				}
				if err != nil {
					errors <- err
					return
				}
				after, ownerErr := selectPositionProfileOwner(f)
				unchanged := ownerErr == nil && after.NodeID == owner.NodeID && after.Epoch == owner.Epoch && after.PID == owner.PID
				metadata := map[string]any{"diagnostic_profiled": true, "qualification": false, "kind": kind, "owner_before": owner, "owner_after": after, "owner_unchanged": unchanged,
					"started_utc": started.UTC(), "finished_utc": finished.UTC(), "elapsed_ms": float64(finished.Sub(started)) / float64(time.Millisecond), "configured_delay": delay.String(), "pprof_address": addresses[owner.Node],
					"source_head": strings.TrimSpace(string(head)), "tracked_diff_sha256": hex.EncodeToString(diffSHA[:]), "backend_binary_sha256": binarySHA,
					"mutex_profile_fraction": 5, "block_profile_rate_ns": 1000000}
				if ownerErr != nil {
					metadata["owner_after_error"] = ownerErr.Error()
				}
				window, err := json.MarshalIndent(metadata, "", "  ")
				if err == nil {
					err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("backend-%d.%s-window.json", owner.Node, kind)), window, 0600)
				}
				if err == nil && !unchanged {
					err = fmt.Errorf("%s profile owner changed or could not be verified", kind)
				}
				if err != nil {
					errors <- err
				}
			}(artifact.kind, artifact.endpoint, artifact.offset)
		}
		jobs.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	t.Cleanup(func() {
		if err := <-done; err != nil {
			t.Errorf("actual-owner diagnostic profile: %v", err)
		}
	})
}

func selectPositionProfileOwner(f *entrypointFixture) (positionProfileOwner, error) {
	global, err := f.projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}})
	if err != nil {
		return positionProfileOwner{}, err
	}
	var id int32
	for _, entry := range global.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
		registry := entry.GetValue().GetSessionRegistry()
		if registry.Name == "LIVE" && registry.State == pb.SessionRegistry_ACTIVE {
			id = registry.Id
		}
	}
	if id == 0 {
		return positionProfileOwner{}, fmt.Errorf("profile session is not active")
	}
	state, err := f.projection.Read(sessionFaultRef(id))
	if err != nil || state == nil || state.Owner == nil {
		return positionProfileOwner{}, fmt.Errorf("profile session owner unavailable: %v", err)
	}
	selected := positionProfileOwner{Node: -1}
	for node, address := range f.addresses {
		status, data := entrypointStatus(address, "/metrics", "")
		if status != http.StatusOK {
			return positionProfileOwner{}, fmt.Errorf("profile node metrics unavailable: node=%d status=%d", node, status)
		}
		nodeID, lease, err := positionProfileMetricIdentity(string(data))
		if err != nil {
			return positionProfileOwner{}, err
		}
		if lease > 0 {
			if selected.Node >= 0 || nodeID != state.Owner.NodeId {
				return positionProfileOwner{}, fmt.Errorf("profile owner identity is ambiguous or changed")
			}
			selected = positionProfileOwner{Node: node, PID: f.apps[node].ownedPID, NodeID: nodeID, Epoch: state.Owner.Epoch, SessionID: id, LeaseSeconds: lease}
		}
	}
	if selected.Node < 0 {
		return positionProfileOwner{}, fmt.Errorf("no backend holds a positive session lease")
	}
	return selected, nil
}

func positionProfileMetricIdentity(metrics string) (string, float64, error) {
	var node string
	var lease float64
	for _, line := range strings.Split(metrics, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		const prefix = "fs_projection_behind_messages{node=\""
		if strings.HasPrefix(fields[0], prefix) && strings.HasSuffix(fields[0], "\"}") {
			node = strings.TrimSuffix(strings.TrimPrefix(fields[0], prefix), "\"}")
		}
		if fields[0] == "fs_owner_lease_remaining_seconds{aggregate_kind=\"session\"}" {
			var err error
			lease, err = strconv.ParseFloat(fields[1], 64)
			if err != nil || math.IsNaN(lease) || math.IsInf(lease, 0) || lease < 0 {
				return "", 0, fmt.Errorf("invalid backend session lease metric")
			}
		}
	}
	if node == "" {
		return "", 0, fmt.Errorf("backend metric node identity missing")
	}
	return node, lease, nil
}

func TestPositionProfileMetricIdentity(t *testing.T) {
	for _, lease := range []string{"0", "7.5"} {
		node, remaining, err := positionProfileMetricIdentity("fs_projection_behind_messages{node=\"actual-owner\"} 0\nfs_owner_lease_remaining_seconds{aggregate_kind=\"session\"} " + lease + "\n")
		if err != nil || node != "actual-owner" || strconv.FormatFloat(remaining, 'f', -1, 64) != lease {
			t.Fatalf("incorrect owner metric identity: %q %f %v", node, remaining, err)
		}
	}
	for _, lease := range []string{"invalid", "NaN", "+Inf", "-1"} {
		if _, _, err := positionProfileMetricIdentity("fs_projection_behind_messages{node=\"owner\"} 0\nfs_owner_lease_remaining_seconds{aggregate_kind=\"session\"} " + lease); err == nil {
			t.Fatalf("malformed lease accepted: %q", lease)
		}
	}
	if _, _, err := positionProfileMetricIdentity("fs_owner_lease_remaining_seconds{aggregate_kind=\"session\"} 10"); err == nil {
		t.Fatal("missing node identity accepted")
	}
}
