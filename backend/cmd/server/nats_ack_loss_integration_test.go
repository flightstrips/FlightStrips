package main

import (
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// This TCP gate loses the actual server-to-backend PubAck, not an injected
// Publish return value. Outbound publishes continue; the independent fixture
// observer proves the server committed before the exact publisher PID dies.
type ackLossGate struct {
	mu          sync.Mutex
	paused      chan struct{}
	blocked     int
	closed      bool
	connections []net.Conn
	listeners   []net.Listener
	jobs        sync.WaitGroup
}
type gatedAckWriter struct {
	target io.Writer
	gate   *ackLossGate
}

func (w gatedAckWriter) Write(data []byte) (int, error) {
	w.gate.mu.Lock()
	pause := w.gate.paused
	if pause != nil {
		w.gate.blocked++
	}
	w.gate.mu.Unlock()
	if pause != nil {
		<-pause
	}
	return w.target.Write(data)
}
func (g *ackLossGate) pause() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paused = make(chan struct{})
	g.blocked = 0
}
func (g *ackLossGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused != nil {
		close(g.paused)
		g.paused = nil
	}
}
func (g *ackLossGate) proxy(t *testing.T, target string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	g.listeners = append(g.listeners, listener)
	g.jobs.Add(1)
	go func() {
		defer g.jobs.Done()
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.DialTimeout("tcp", target, time.Second)
			if err != nil {
				_ = client.Close()
				continue
			}
			g.mu.Lock()
			if g.closed {
				g.mu.Unlock()
				_ = client.Close()
				_ = server.Close()
				return
			}
			g.connections = append(g.connections, client, server)
			g.jobs.Add(2)
			g.mu.Unlock()
			go func() {
				defer g.jobs.Done()
				defer client.Close()
				defer server.Close()
				_, _ = io.Copy(server, client)
			}()
			go func() {
				defer g.jobs.Done()
				defer client.Close()
				defer server.Close()
				_, _ = io.Copy(gatedAckWriter{client, g}, server)
			}()
		}
	}()
	return listener.Addr().String()
}
func (g *ackLossGate) close() {
	g.release()
	g.mu.Lock()
	g.closed = true
	for _, c := range g.connections {
		_ = c.Close()
	}
	for _, l := range g.listeners {
		_ = l.Close()
	}
	g.mu.Unlock()
	g.jobs.Wait()
}

func TestServerNATSPubAckLostBeforeBackendDeath(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 native fixture")
	}
	f := newEntrypointFixture(t, true)
	gate := &ackLossGate{}
	t.Cleanup(gate.close)
	urls := []string{}
	for _, raw := range f.resources.URLs {
		parsed, err := url.Parse(raw)
		require.NoError(t, err)
		parsed.Host = gate.proxy(t, parsed.Host)
		urls = append(urls, parsed.String())
	}
	f.changeEnvironment(map[string]string{"NATS_URLS": strings.Join(urls, ",")})
	name, ref, _ := f.seededSession()
	state := f.state(ref)
	_, presence, err := f.projection.ObservationSnapshot(ref.GetSession().Id)
	require.NoError(t, err)
	ownerNode := -1
	for _, entry := range presence {
		client := entry.Value.GetClient()
		if client != nil && client.Kind == pb.ClientPresence_EUROSCOPE && client.NodeId == state.Owner.NodeId {
			switch client.Cid {
			case "111111":
				ownerNode = 0
			case "222222":
				ownerNode = 1
			}
		}
	}
	require.NotEqual(t, -1, ownerNode)
	front := f.front(ownerNode, name)
	id := uuid.NewString()
	revision := state.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
	command := markedCommand(id, "SAS123", revision, true)
	gate.pause()
	t.Cleanup(gate.release)
	started := time.Now()
	sendEntrypointFrame(t, front, command)
	// Do not wait for a frontend reply: the owner's actual PubAck and projection
	// delivery are held at the TCP boundary, while its outbound publish reached JS.
	f.await("independent observer proves commit with PubAck delivery held", func() bool { return f.state(ref).Ledger[id] != nil })
	gate.mu.Lock()
	blocked := gate.blocked
	gate.mu.Unlock()
	require.Positive(t, blocked)
	require.Less(t, time.Since(started), 2*time.Second, "kill before the ordinary request timeout")
	sequence := f.state(ref).Ledger[id].CommittedStreamSequence
	pid := f.apps[ownerNode].ownedPID
	f.apps[ownerNode].stop()
	gate.release()
	f.await("surviving backend serves committed outcome despite lost PubAck", func() bool { return f.outcome(1-ownerNode, id) == "succeeded" })
	f.restart(ownerNode)
	require.Equal(t, "succeeded", f.outcome(ownerNode, id))
	session := ref.GetSession().Id
	reply := f.route(&pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "111111", SessionId: &session}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_Client{Client: command.GetCommand().Action}})
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
	require.Equal(t, sequence, reply.GetStreamSequence())
	t.Logf("PUBACK_LOSS command_id=%s sequence=%d blocked_writes=%d killed_pid=%d elapsed=%s exact_outcome=SUCCEEDED same_id_sequence=true", id, sequence, blocked, pid, time.Since(started))
}
