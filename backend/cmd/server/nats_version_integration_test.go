package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func buildBaseEntrypoint(t *testing.T, backend string) string {
	t.Helper()
	dir := t.TempDir()
	archive := filepath.Join(dir, "base.tar")
	// Extract committed source without switching this checkout or touching any
	// application/dev configuration. No extra managed worktree is needed.
	command := exec.Command("git", "rev-parse", "db662817")
	command.Dir = filepath.Dir(backend)
	revision, err := command.CombinedOutput()
	require.NoError(t, err, string(revision))
	command = exec.Command("git", "archive", "db662817", "-o", archive)
	command.Dir = filepath.Dir(backend)
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	command = exec.Command("tar", "-xf", archive, "-C", dir)
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binary := filepath.Join(dir, "api-base"+suffix)
	command = exec.Command("go", "build", "-o", binary, "./cmd/server")
	command.Dir = filepath.Join(dir, "backend")
	output, err = command.CombinedOutput()
	require.NoError(t, err, string(output))
	data, err := os.ReadFile(binary)
	require.NoError(t, err)
	t.Logf("OVERLAP_BASE source=%s binary_sha256=%s", revision, fixtureDigest(data))
	return binary
}

func TestServerNATSVersionOverlap(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit Task22 disposable fixture")
	}
	f := newEntrypointFixture(t, true)
	oldBinary := buildBaseEntrypoint(t, f.backend)
	f.apps[0].stop()
	f.apps[0] = startFixtureProcess(t, oldBinary, f.backend, f.env, "-addr", f.addresses[0])
	f.ready()
	name, ref, _ := f.seededSession()
	// Both compiled versions read and write schema v1 during overlap. Each
	// command enters a different node; CAS/owner routing remains authoritative.
	for node := 0; node < 2; node++ {
		front := f.front(node, name)
		id := uuid.NewString()
		revision := f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
		sendEntrypointFrame(t, front, markedCommand(id, "SAS123", revision, node == 0))
		f.await("compatible reader/writer outcome on both binaries", func() bool { return f.outcome(0, id) == "succeeded" && f.outcome(1, id) == "succeeded" })
		t.Logf("OVERLAP_WRITE ingress_node=%d command_id=%s sequence=%d entity_revision=%d", node, id, f.state(ref).Ledger[id].CommittedStreamSequence, f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].Revision)
	}
	js, err := f.nc.JetStream()
	require.NoError(t, err)
	event := &pb.StateEvent{SchemaVersion: 2, EventId: uuid.NewString(), Aggregate: &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "ZZZZ"}}}, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "future-writer"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "future-writer", Epoch: 1}}}
	data, err := proto.Marshal(event)
	require.NoError(t, err)
	subject, err := cluster.Subject(event.Aggregate)
	require.NoError(t, err)
	sequence, err := (cluster.NATSStore{JS: js}).Publish(f.ctx, subject, 0, data)
	require.NoError(t, err)
	f.await("both actual version readers fence unknown writer", func() bool {
		a, _ := entrypointStatus(f.addresses[0], "/readyz", "")
		b, _ := entrypointStatus(f.addresses[1], "/readyz", "")
		return a == 503 && b == 503
	})
	for _, address := range f.addresses {
		status, _ := entrypointStatus(address, "/healthz", "")
		require.Equal(t, 200, status)
		status, _ = entrypointStatus(address, "/frontEndEvents", "")
		require.Equal(t, 503, status)
	}
	t.Logf("OVERLAP_FENCE unknown_schema=2 sequence=%d versions=[db662817,current] readiness=[503,503] at=%s", sequence, time.Now().UTC().Format(time.RFC3339Nano))
}
