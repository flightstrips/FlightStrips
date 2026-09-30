package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func (f *runtimeFixture) status(node int, path string) int {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(f.servers[node].URL + path)
	if err != nil {
		return 0
	}
	defer response.Body.Close()
	return response.StatusCode
}

type pausedWriter struct {
	destination net.Conn
	gate        *brokerGate
}
type brokerGate struct {
	mu          sync.Mutex
	resume      chan struct{}
	closed      bool
	connections []net.Conn
	listeners   []net.Listener
	jobs        sync.WaitGroup
}

func (g *brokerGate) pause() { g.mu.Lock(); defer g.mu.Unlock(); g.resume = make(chan struct{}) }
func (g *brokerGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.resume != nil {
		close(g.resume)
		g.resume = nil
	}
}
func (w pausedWriter) Write(data []byte) (int, error) {
	w.gate.mu.Lock()
	resume := w.gate.resume
	w.gate.mu.Unlock()
	if resume != nil {
		<-resume
	}
	return w.destination.Write(data)
}
func (g *brokerGate) proxy(t *testing.T, target string) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	g.mu.Lock()
	g.listeners = append(g.listeners, listener)
	g.mu.Unlock()
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
				_, _ = io.Copy(pausedWriter{client, g}, server)
			}()
		}
	}()
	t.Cleanup(func() {
		g.release()
		g.mu.Lock()
		g.closed = true
		for _, connection := range g.connections {
			_ = connection.Close()
		}
		for _, socket := range g.listeners {
			_ = socket.Close()
		}
		g.mu.Unlock()
		_ = listener.Close()
		g.jobs.Wait()
	})
	return listener.Addr().String()
}

func TestBuildNATSReadinessProjectionStallAndRecovery(t *testing.T) {
	gate := &brokerGate{}
	f := newRuntimeFixture(t, func(cfg *Config, _ *Dependencies) {
		for i, raw := range cfg.NATS.Resources.URLs {
			parsed, err := url.Parse(raw)
			require.NoError(t, err)
			parsed.Host = gate.proxy(t, parsed.Host)
			cfg.NATS.Resources.URLs[i] = parsed.String()
		}
	})
	gate.pause()
	t.Cleanup(gate.release)
	f.await("stalled projection cannot admit commands", func() bool { return f.status(0, "/readyz") == 503 && f.status(1, "/readyz") == 503 })
	require.Equal(t, 200, f.status(1, "/healthz"))
	require.Equal(t, 503, f.status(0, "/euroscopeEvents"))
	gate.release()
	f.await("both projections verify and catch up after delivery resumes", func() bool { return f.status(0, "/readyz") == 200 && f.status(1, "/readyz") == 200 })
	for _, app := range f.apps {
		require.Nil(t, app.natsRuntime.workerErr.Load())
	}
}

func TestBuildNATSReadinessResourceDriftAndFailedConstruction(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	js, err := f.admin.JetStream()
	require.NoError(t, err)
	info, err := js.StreamInfo(f.cfg.NATS.Resources.Names.State)
	require.NoError(t, err)
	original := info.Config
	defer func() { _, err := js.UpdateStream(&original); require.NoError(t, err) }()
	drift := original
	drift.MaxMsgSize /= 2
	_, err = js.UpdateStream(&drift)
	require.NoError(t, err)
	f.await("both nodes fail readiness for drift", func() bool { return f.status(0, "/readyz") == 503 && f.status(1, "/readyz") == 503 })
	require.Equal(t, 200, f.status(0, "/healthz"))
	require.Equal(t, 503, f.status(1, "/api/pilot/flight?callsign=SAS101"))
	failing, err := BuildNATS(f.ctx, f.cfg, f.deps)
	require.ErrorContains(t, err, "verify NATS resources")
	require.Nil(t, failing)
	_, err = js.UpdateStream(&original)
	require.NoError(t, err)
	f.await("verified drift recovery", func() bool { return f.status(0, "/readyz") == 200 && f.status(1, "/readyz") == 200 })
	info, err = js.StreamInfo(original.Name)
	require.NoError(t, err)
	consumers := info.State.Consumers
	badKeys := f.cfg
	badKeys.NATS.EffectKeyFiles = map[string]string{"test": "missing-runtime-key"}
	failing, err = BuildNATS(f.ctx, badKeys, f.deps)
	require.Error(t, err)
	require.Nil(t, failing)
	f.await("failed constructor removes replay consumer", func() bool {
		info, e := js.StreamInfo(original.Name)
		return e == nil && info.State.Consumers == consumers
	})
}

// Broker faults require the explicitly named disposable Compose project. The
// label check prevents this test from stopping an unrelated NATS installation.
func TestBuildNATSReadinessQuorumLossAndRecovery(t *testing.T) {
	if os.Getenv("NATS_FAULT_PROJECT") != "fs20a" {
		t.Skip("requires NATS_FAULT_PROJECT=fs20a for the isolated fixture")
	}
	f := newRuntimeFixture(t, nil)
	containers := []string{"fs20a-nats-2-1", "fs20a-nats-3-1"}
	for _, name := range containers {
		out, err := exec.Command("docker", "inspect", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}", name).Output()
		require.NoError(t, err)
		require.Equal(t, "fs20a\n", string(out))
	}
	restart := func() {
		output, err := exec.Command("docker", append([]string{"start"}, containers...)...).CombinedOutput()
		require.NoError(t, err, string(output))
	}
	t.Cleanup(restart)
	output, err := exec.Command("docker", append([]string{"stop", "--time", "1"}, containers...)...).CombinedOutput()
	require.NoError(t, err, string(output))
	f.await("minority cannot admit either application", func() bool { return f.status(0, "/readyz") == 503 && f.status(1, "/readyz") == 503 })
	require.Equal(t, 200, f.status(0, "/healthz"))
	require.Equal(t, 503, f.status(0, "/frontEndEvents"))
	restart()
	f.await("both applications recover after quorum and replay", func() bool { return f.status(0, "/readyz") == 200 && f.status(1, "/readyz") == 200 })
	for _, app := range f.apps {
		require.Nil(t, app.natsRuntime.workerErr.Load())
		require.Equal(t, nats.CONNECTED, app.natsRuntime.nc.Status())
	}
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	require.NoError(t, f.apps[0].Close(shutdown))
	f.closed[0] = true
	require.NoError(t, f.apps[0].Close(shutdown))
	require.Equal(t, 503, f.status(0, "/readyz"))
	require.Equal(t, 503, f.status(0, "/euroscopeEvents"))
}
