package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

func TestViffMasterAndFlightTwoReplicaNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1024 || parsed > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", raw)
		}
		port = parsed
	}
	url := func(user string, offset int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = []string{url("backend", 0), url("backend", 1), url("backend", 2)}
	seed := uuid.New()
	icao := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	sessionID := int32(100000 + int(seed[4])*1000 + int(seed[5]))
	airport, session := airportRef(icao), sessionRef(sessionID)
	type replica struct {
		nc     *nats.Conn
		owner  *OwnerRuntime
		writer Writer
		state  NavigationWeather
		stop   context.CancelFunc
	}
	var nodes [2]replica
	for i := range nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		projection := startProjection(t, ctx, nc, cfg)
		store := NATSStore{JS: projection.JS}
		owner, err := NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range []*pb.AggregateRef{airport, session} {
			if err := owner.Track(ref); err != nil {
				t.Fatal(err)
			}
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		writer := Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}
		nodes[i] = replica{nc: nc, owner: owner, writer: writer, state: NavigationWeather{Writer: writer, Objects: NATSObjects{Store: objects}}, stop: stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(ref *pb.AggregateRef, exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("vIFF owner handoff timed out: %v", ref)
		return -1
	}
	masterOwner := waitOwner(airport, "")
	masterClient := &viffStub{}
	master := ViffWriteSpec{OperationID: uuid.NewString(), Kind: ViffSetMaster, Airport: icao, Position: icao + "_TWR"}
	adapter := func(i int, client ViffClient) ViffWriteAdapter {
		return ViffWriteAdapter{Worker: ExternalCallWorker{Writer: nodes[i].writer}, Client: client}
	}
	if sent, err := adapter(1-masterOwner, masterClient).Run(ctx, master); sent || err == nil || masterClient.calls != 0 {
		t.Fatalf("nonowner master call: %v %v calls=%d", sent, err, masterClient.calls)
	}
	if sent, err := adapter(masterOwner, masterClient).Run(ctx, master); !sent || err != nil || masterClient.calls != 1 {
		t.Fatalf("owner master call: %v %v calls=%d", sent, err, masterClient.calls)
	}
	if sent, err := adapter(masterOwner, masterClient).Run(ctx, master); sent || err != nil || masterClient.calls != 1 {
		t.Fatalf("master replay: %v %v calls=%d", sent, err, masterClient.calls)
	}
	sessionOwner := waitOwner(session, "")
	other := 1 - sessionOwner
	seedViffSession(t, nodes[sessionOwner].writer, sessionID, icao)
	readClient := &viffReadStub{airport: icao}
	read := func(i int) ViffReadAdapter {
		return ViffReadAdapter{State: nodes[i].state, Worker: ExternalCallWorker{Writer: nodes[i].writer}, Client: readClient}
	}
	readDeadline := time.Now().UTC()
	if sent, err := read(other).Flights(ctx, icao, sessionID, readDeadline); sent || err == nil || readClient.flights != 0 {
		t.Fatalf("nonowner vIFF read: %v %v calls=%d", sent, err, readClient.flights)
	}
	if sent, err := read(sessionOwner).Flights(ctx, icao, sessionID, readDeadline); !sent || err != nil || readClient.flights != 1 {
		t.Fatalf("owner vIFF read: %v %v calls=%d projection=%v", sent, err, readClient.flights, nodes[sessionOwner].writer.Projection.Ready())
	}
	for ctx.Err() == nil {
		page, revision, err := read(other).ReadFlights(ctx, sessionID, "")
		if err == nil && page != nil && revision == 1 && len(page.Flights) == 1 && page.Flights[0].CdmData.Tsat == "1220" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if sent, err := read(sessionOwner).Flights(ctx, icao, sessionID, readDeadline); sent || err != nil || readClient.flights != 1 {
		t.Fatalf("vIFF read replay: %v %v calls=%d", sent, err, readClient.flights)
	}
	flight := ViffWriteSpec{OperationID: uuid.NewString(), Kind: ViffDpi, Airport: icao, SessionID: sessionID, Callsign: "SAS123", Value: "OBT/1200/10"}
	flightClient := &viffStub{call: func() error { nodes[sessionOwner].stop(); nodes[sessionOwner].nc.Close(); return nil }}
	if sent, err := adapter(other, flightClient).Run(ctx, flight); sent || err == nil || flightClient.calls != 0 {
		t.Fatalf("nonowner flight call: %v %v calls=%d", sent, err, flightClient.calls)
	}
	if sent, err := adapter(sessionOwner, flightClient).Run(ctx, flight); !sent || err == nil || flightClient.calls != 1 {
		t.Fatalf("post-call owner death: %v %v calls=%d", sent, err, flightClient.calls)
	}
	if waitOwner(session, nodes[sessionOwner].owner.NodeID) != other {
		t.Fatal("wrong session takeover")
	}
	if err := adapter(other, flightClient).Resume(ctx, icao, sessionID); err != nil {
		t.Fatal(err)
	}
	if sent, err := adapter(other, flightClient).Run(ctx, flight); sent || err != nil || flightClient.calls != 1 {
		t.Fatalf("uncertain flight write repeated: %v %v calls=%d", sent, err, flightClient.calls)
	}
	subject, _ := Subject(session)
	state, err := nodes[other].writer.load(ctx, subject, session)
	if err != nil || state.Workflows[flight.OperationID].Status != pb.WorkflowRecord_FAILED || state.Workflows[flight.OperationID].ReasonCode != "CALL_UNCERTAIN" {
		t.Fatalf("session uncertainty missing: %v %v", state.Workflows[flight.OperationID], err)
	}
}
