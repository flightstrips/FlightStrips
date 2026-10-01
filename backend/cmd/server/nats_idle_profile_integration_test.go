package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Private captured snapshots are used only in disposable resources. The source
// cluster is never connected to by this test. Padding establishes checkpoint
// sequences, not a replacement operational event history or restore proof.
func TestRetainedStateIdleCPU(t *testing.T) {
	dir := os.Getenv("NATS_IDLE_SOURCE_DIR")
	if dir == "" || runtime.GOOS != "windows" {
		t.Skip("opt-in Windows retained-state profiling fixture")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:6060")
	require.NoError(t, err, "refuse to collide with another profiler")
	require.NoError(t, listener.Close())
	f := newEntrypointFixture(t, true)
	for _, app := range f.apps {
		app.stop()
	}
	f.stopProjection()
	cfg := f.resources
	cfg.URLs = append([]string(nil), cfg.URLs...)
	for i, u := range cfg.URLs {
		cfg.URLs[i] = strings.ReplaceAll(u, "backend:backend-local-only", "bootstrap:bootstrap-local-only")
	}
	admin, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	defer admin.Close()
	js, err := admin.JetStream(nats.PublishAsyncMaxPending(512))
	require.NoError(t, err)
	info, err := js.StreamInfo(cfg.Names.State)
	require.NoError(t, err)
	require.NoError(t, js.DeleteStream(cfg.Names.State))
	_, err = js.AddStream(&info.Config)
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(dir, "*.pb"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	snapshots := []*pb.Snapshot{}
	var high uint64
	objects, err := js.ObjectStore(cfg.Names.Objects)
	require.NoError(t, err)
	index, err := js.KeyValue(cfg.Names.SnapshotIndex)
	require.NoError(t, err)
	for _, filename := range files {
		data, err := os.ReadFile(filename)
		require.NoError(t, err)
		snapshot := &pb.Snapshot{}
		require.NoError(t, pb.UnmarshalStrict(data, snapshot))
		require.Greater(t, snapshot.LastStreamSequence, uint64(0))
		if snapshot.LastStreamSequence > high {
			high = snapshot.LastStreamSequence
		}
		snapshots = append(snapshots, snapshot)
		key := strings.TrimSuffix(filepath.Base(filename), ".pb")
		name := fmt.Sprintf("snapshot/%s/%d", strings.ReplaceAll(key, ".", "/"), snapshot.LastStreamSequence)
		data, err = proto.Marshal(&pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Snapshot{Snapshot: snapshot}})
		require.NoError(t, err)
		_, err = objects.PutBytes(name, data)
		require.NoError(t, err)
		entry := &pb.SnapshotIndex{SchemaVersion: 1, Aggregate: snapshot.Aggregate, ObjectName: name, Sha256: fixtureDigest(data), AggregateRevision: snapshot.AggregateRevision, LastStreamSequence: snapshot.LastStreamSequence, LastSubjectSequence: snapshot.LastSubjectSequence}
		encoded, err := proto.Marshal(entry)
		require.NoError(t, err)
		_, err = index.Put(key, encoded)
		require.NoError(t, err)
		store := cluster.SnapshotStore{Index: index, Objects: objects}
		loaded, err := store.Load(snapshot.Aggregate)
		require.NoError(t, err)
		require.Equal(t, snapshot.AggregateRevision, loaded.Revision, "captured checkpoint must load rather than fall back to empty state")
		require.Len(t, loaded.Ledger, len(snapshot.Outcomes))
		t.Logf("IDLE_STATE aggregate=%s entities=%d outcomes=%d workflows=%d sequence=%d", key, len(snapshot.Entities), len(snapshot.Outcomes), len(snapshot.Workflows), snapshot.LastStreamSequence)
	}
	require.Less(t, high, uint64(100000), "bounded diagnostic padding")
	node := uuid.NewString()
	for seq := uint64(1); seq <= high; seq++ {
		selected := snapshots[0]
		for _, snapshot := range snapshots {
			if snapshot.LastStreamSequence > selected.LastStreamSequence {
				selected = snapshot
			}
			if snapshot.LastStreamSequence == seq {
				selected = snapshot
				break
			}
		}
		// The last message per subject must equal its captured CAS checkpoint.
		subject := "fs.v1.state.global"
		if selected.Aggregate.GetAirport() != nil {
			subject = "fs.v1.state.airport." + selected.Aggregate.GetAirport().Icao
		}
		event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: selected.Aggregate, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "idle-padding"}, Fact: &pb.StateEvent_OwnerRenewed{OwnerRenewed: &pb.OwnerTerm{NodeId: node, Epoch: 1}}}
		data, err := proto.Marshal(event)
		require.NoError(t, err)
		_, err = js.PublishAsync(subject, data)
		require.NoError(t, err)
	}
	select {
	case <-js.PublishAsyncComplete():
	case <-time.After(time.Minute):
		t.Fatal("padding PubAck timeout")
	}
	info, err = js.StreamInfo(cfg.Names.State)
	require.NoError(t, err)
	require.Equal(t, high, info.State.Msgs, "all padding messages must have reached the stream")
	require.Equal(t, high, info.State.LastSeq)
	old := buildBaseEntrypoint(t, f.backend)
	current := f.binary
	measurements := map[string]float64{}
	for _, version := range []struct{ name, binary string }{{"base", old}, {"fixed", current}} {
		for i := range f.apps {
			env := replaceFixtureEnv(f.env, map[string]string{"ENABLE_PPROF": strconv.FormatBool(i == 0)})
			f.apps[i] = startFixtureProcess(t, version.binary, f.backend, env, "-addr", f.addresses[i])
		}
		f.ready()
		time.Sleep(5 * time.Second)
		if version.name == "fixed" {
			logHistoryMetrics(t, f, "before")
		}
		before := ownedCPUSeconds(t, f.apps)
		started := time.Now()
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1:6060/debug/pprof/profile?seconds=20", nil)
		require.NoError(t, err)
		response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		profile, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		after := ownedCPUSeconds(t, f.apps)
		elapsed := time.Since(started).Seconds()
		measurements[version.name] = (after - before) / elapsed
		path := filepath.Join(dir, "idle-"+version.name+".cpu")
		require.NoError(t, os.WriteFile(path, profile, 0600))
		t.Logf("IDLE_CPU version=%s combined_cores=%.4f process_cpu_seconds=%.4f elapsed_seconds=%.4f binary_sha256=%s", version.name, measurements[version.name], after-before, elapsed, hashFixtureBinary(t, version.binary))
		if version.name == "fixed" {
			logHistoryMetrics(t, f, "after")
		}
		for _, app := range f.apps {
			app.stop()
		}
	}
	require.Less(t, measurements["fixed"], measurements["base"]*.05, "remove at least 95%% of retained-state idle CPU")
	require.Less(t, measurements["fixed"], 0.05, "two idle backends must use less than five percent of one core on this qualification host")
}

func logHistoryMetrics(t *testing.T, f *entrypointFixture, sample string) {
	t.Helper()
	for node, address := range f.addresses {
		response, err := http.Get("http://" + address + "/metrics")
		require.NoError(t, err)
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "fs_go_heap_alloc_bytes ") || strings.HasPrefix(line, "fs_history_cache_bytes ") || strings.HasPrefix(line, "fs_projection_hot_history_records{") {
				t.Logf("IDLE_MEMORY sample=%s node=%d %s", sample, node, line)
			}
		}
	}
}

func hashFixtureBinary(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return fixtureDigest(data)
}
func ownedCPUSeconds(t *testing.T, apps []*fixtureProcess) float64 {
	t.Helper()
	total := float64(0)
	for _, app := range apps {
		command := exec.Command("powershell", "-NoProfile", "-Command", fmt.Sprintf("(Get-Process -Id %d).CPU.ToString([Globalization.CultureInfo]::InvariantCulture)", app.ownedPID))
		output, err := command.CombinedOutput()
		require.NoError(t, err)
		value, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
		require.NoError(t, err)
		total += value
	}
	return total
}
