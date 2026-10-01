package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/euroscopebinary"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type processLog struct {
	sync.Mutex
	bytes.Buffer
}

func (b *processLog) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
func (b *processLog) text() string { b.Lock(); defer b.Unlock(); return b.Buffer.String() }

type fixtureProcess struct {
	command *exec.Cmd
	log     *processLog
	done    chan error
	stopped bool
}

func startFixtureProcess(t *testing.T, binary, cwd string, env []string, args ...string) *fixtureProcess {
	t.Helper()
	p := &fixtureProcess{command: exec.Command(binary, args...), log: &processLog{}, done: make(chan error, 1)}
	p.command.Dir, p.command.Env = cwd, env
	p.command.Stdout, p.command.Stderr = p.log, p.log
	require.NoError(t, p.command.Start())
	go func() { p.done <- p.command.Wait() }()
	t.Cleanup(func() {
		p.stop()
		if t.Failed() {
			t.Log(p.log.text())
		}
	})
	return p
}
func (p *fixtureProcess) stop() {
	if !p.stopped {
		p.stopped = true
		_ = p.command.Process.Kill()
		<-p.done
	}
}
func entrypointAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, e)
	defer l.Close()
	return l.Addr().String()
}
func fixtureEnv(values map[string]string) []string {
	var env []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, exists := values[key]; !exists {
			env = append(env, item)
		}
	}
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}
func entrypointStatus(address, path, token string) (int, []byte) {
	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, "http://"+address+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, data
}
func sendEntrypointFrame(t *testing.T, c *websocket.Conn, v proto.Message) {
	t.Helper()
	data, e := proto.Marshal(v)
	require.NoError(t, e)
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, data))
}

// This exercises the compiled cmd/server, its environment loader and real
// authentication, in addition to the comprehensive BuildNATS provider tests.
// NATS_SERVER_BINARY optionally supplies a pinned native fixture owned entirely
// by this test; otherwise the separately bootstrapped Compose fixture is used.
func TestServerNATSBinaryCrossNodeRestartAndReadiness(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires disposable three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	backend, err := filepath.Abs("../..")
	require.NoError(t, err)
	dir := t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binary := filepath.Join(dir, "api"+suffix)
	build := exec.Command("go", "build", "-o", binary, "./cmd/server")
	build.Dir = backend
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	urls := []string{}
	brokers := []*fixtureProcess{}
	native := os.Getenv("NATS_SERVER_BINARY")
	startBroker := func(i int) *fixtureProcess {
		return startFixtureProcess(t, native, backend, os.Environ(), "-c", filepath.Join(dir, fmt.Sprintf("nats-%d.conf", i)))
	}
	if native != "" {
		version, e := exec.Command(native, "--version").Output()
		require.NoError(t, e)
		require.Contains(t, string(version), "v2.15.0")
		clients, routes := []string{}, []string{}
		for i := 0; i < 3; i++ {
			clients = append(clients, entrypointAddress(t))
			routes = append(routes, entrypointAddress(t))
		}
		for i := 0; i < 3; i++ {
			source, e := os.ReadFile(filepath.Join(backend, "testdata", "nats", fmt.Sprintf("nats-%d.conf", i+1)))
			require.NoError(t, e)
			conf := strings.ReplaceAll(string(source), "port: 4222", "listen: "+clients[i])
			conf = strings.ReplaceAll(conf, "port: 6222", "listen: "+routes[i])
			conf = strings.ReplaceAll(conf, "/data/jetstream", filepath.ToSlash(filepath.Join(dir, fmt.Sprintf("data-%d", i))))
			for peer := 0; peer < 3; peer++ {
				conf = strings.ReplaceAll(conf, fmt.Sprintf("nats://nats-%d:6222", peer+1), "nats://"+routes[peer])
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("nats-%d.conf", i)), []byte(conf), 0600))
			urls = append(urls, "nats://bootstrap:bootstrap-local-only@"+clients[i])
			brokers = append(brokers, startBroker(i))
		}
	} else {
		base := 5322
		if v := os.Getenv("NATS_TEST_PORT_BASE"); v != "" {
			base, err = strconv.Atoi(v)
			require.NoError(t, err)
		}
		for i := 0; i < 3; i++ {
			urls = append(urls, fmt.Sprintf("nats://bootstrap:bootstrap-local-only@127.0.0.1:%d", base+i))
		}
	}
	resources := natsresources.Config{URLs: urls, Names: natsresources.RequiredNames, ConnectTimeout: time.Second, RequestTimeout: 2 * time.Second}
	var adminErr error
	require.Eventually(t, func() bool {
		nc, e := natsresources.Connect(resources)
		if e != nil {
			adminErr = e
			return false
		}
		defer nc.Close()
		adminErr = natscluster.WaitForQuorum(ctx, nc)
		if adminErr == nil {
			adminErr = natsresources.Bootstrap(ctx, nc, resources)
		}
		return adminErr == nil
	}, 30*time.Second, 100*time.Millisecond, "administrator bootstrap")
	for i, url := range urls {
		urls[i] = strings.ReplaceAll(url, "bootstrap:bootstrap-local-only", "backend:backend-local-only")
	}
	resources.URLs = urls
	nc, err := natsresources.Connect(resources)
	require.NoError(t, err)
	defer nc.Close()
	projection, err := cluster.NewProjection(nc, resources)
	require.NoError(t, err)
	projectionCtx, stopProjection := context.WithCancel(ctx)
	defer stopProjection()
	projected := make(chan error, 1)
	go func() { projected <- projection.Run(projectionCtx) }()
	defer func() { stopProjection(); <-projected }()
	require.Eventually(t, func() bool { return projection.Ready() == nil }, 15*time.Second, 50*time.Millisecond)
	key := filepath.Join(dir, "effects.key")
	require.NoError(t, os.WriteFile(key, []byte("01234567890123456789012345678901"), 0600))
	jwtKey := []byte("entrypoint-fixture-signing-secret")
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "oct", "kid": "fixture", "alg": "HS256", "k": base64.RawURLEncoding.EncodeToString(jwtKey)}}})
	}))
	defer identity.Close()
	signed := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix(), "aud": "backend-dev", "vatsim/cid": "111111", "vatsim/rating": 5})
	signed.Header["kid"] = "fixture"
	token, err := signed.SignedString(jwtKey)
	require.NoError(t, err)
	env := fixtureEnv(map[string]string{"NATS_URLS": strings.Join(urls, ","), "NATS_EFFECT_ACTIVE_KEY_ID": "v1", "NATS_EFFECT_KEY_FILES": "v1=" + key, "OIDC_AUTHORITY": identity.URL, "OIDC_SIGNING_ALGO": "HS256", "OIDC_AUDIENCE": "backend-dev", "ENVIRONMENT": "test", "OTEL_EXPORTER_OTLP_ENDPOINT": "", "NAVIGATION_SOURCE": "", "NAVIGATION_TERMINAL_GEOMETRY_PATH": "", "AMAN_MODE": "disabled", "ENABLE_TEST_TOOLS": "false", "ENABLE_STAND_ASSIGNMENT": "false", "ENABLE_VATSIM": "false", "ENABLE_VATSIM_TRANSCEIVERS": "false", "ENABLE_METAR": "false", "ENABLE_ECFMP": "false", "CDM_KEY": "", "CDM_KEY_FILE": "", "HOPPIE_LOGON": "", "HOPPIE_LOGON_FILE": ""})
	addresses := []string{entrypointAddress(t), entrypointAddress(t)}
	apps := []*fixtureProcess{startFixtureProcess(t, binary, backend, env, "-addr", addresses[0]), startFixtureProcess(t, binary, backend, env, "-addr", addresses[1])}
	allApps := append([]*fixtureProcess(nil), apps...)
	ready := func() {
		require.Eventually(t, func() bool {
			a, _ := entrypointStatus(addresses[0], "/readyz", "")
			b, _ := entrypointStatus(addresses[1], "/readyz", "")
			return a == 200 && b == 200
		}, 30*time.Second, 100*time.Millisecond, "two compiled server processes ready")
	}
	ready()
	name := "TASK20C-" + strings.ToUpper(uuid.NewString())
	callsign := "S" + strings.ToUpper(uuid.NewString()[:6])
	dial := func(node int, path, protocol string) *websocket.Conn {
		c, _, e := (&websocket.Dialer{Subprotocols: []string{protocol}}).Dial("ws://"+addresses[node]+path, nil)
		require.NoError(t, e)
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	euro := dial(0, "/euroscopeEvents", euroscopebinary.Subprotocol)
	sendEntrypointFrame(t, euro, &es.Envelope{Event: &es.Envelope_Token{Token: &es.TokenEvent{Token: token, ProtocolRevision: 2}}})
	sendEntrypointFrame(t, euro, &es.Envelope{Event: &es.Envelope_Login{Login: &es.LoginEvent{Airport: "EKCH", Connection: name, Callsign: "EKCH_A_TWR", Position: "118.100"}}})
	var state *cluster.Aggregate
	var session int32
	ref := func() *pb.AggregateRef {
		return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: session}}}
	}
	require.Eventually(t, func() bool {
		registry, e := projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}})
		if e != nil {
			return false
		}
		for _, entity := range registry.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
			if s := entity.Value.GetSessionRegistry(); s.Name == name {
				session = s.Id
			}
		}
		if session == 0 {
			return false
		}
		state, e = projection.Read(ref())
		return e == nil && state.Master != nil && state.Master.Cid == "111111"
	}, 30*time.Second, 50*time.Millisecond)
	// Drain outbound plugin frames so real socket delivery never stalls the fixture.
	go func() {
		for {
			if _, _, e := euro.ReadMessage(); e != nil {
				return
			}
		}
	}()
	sendEntrypointFrame(t, euro, &es.Envelope{SessionId: session, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: []*es.Strip{{Callsign: callsign, Origin: "EKCH", Destination: "EGLL", AircraftType: "A320", AssignedSquawk: "1001", Sid: "ODN1C", Runway: "22R", HasFp: true}}, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}}}})
	require.Eventually(t, func() bool {
		s, e := projection.Read(ref())
		if e == nil {
			state = s
		}
		return e == nil && s.Indexes[pb.EntityKind_STRIP][callsign] != nil && s.Sync != nil
	}, 15*time.Second, 50*time.Millisecond)
	front := dial(1, "/frontEndEvents", frontendbinary.Subprotocol)
	sendEntrypointFrame(t, front, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: token, Airport: "EKCH", SessionName: name}}})
	_ = front.SetReadDeadline(time.Now().Add(15 * time.Second))
	kind, data, err := front.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, kind)
	initial := &pb.FrontendFrame{}
	require.NoError(t, pb.UnmarshalStrict(data, initial))
	require.NotNil(t, initial.GetInitial())
	id := uuid.NewString()
	revision := state.Indexes[pb.EntityKind_STRIP][callsign].Revision
	sendEntrypointFrame(t, front, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id, ExpectedEntityRevision: &revision, Action: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: callsign, Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: true}}}}}}}})
	accepted := func(address string) bool {
		status, body := entrypointStatus(address, "/api/commands/"+id, token)
		var outcome map[string]any
		_ = json.Unmarshal(body, &outcome)
		return status == 200 && outcome["status"] == "succeeded"
	}
	for _, address := range addresses {
		require.Eventually(t, func() bool { return accepted(address) }, 15*time.Second, 50*time.Millisecond, "JSON outcome on each process")
	}
	require.Eventually(t, func() bool {
		current, e := projection.Read(ref())
		return e == nil && current.Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip().GetMarked()
	}, 15*time.Second, 50*time.Millisecond, "binary command changes accepted strip state")
	apps[0].stop()
	require.Eventually(t, func() bool { return accepted(addresses[1]) }, 15*time.Second, 50*time.Millisecond, "acknowledged result survives backend death")
	apps[0] = startFixtureProcess(t, binary, backend, env, "-addr", addresses[0])
	allApps = append(allApps, apps[0])
	ready()
	require.True(t, accepted(addresses[0]))
	if native != "" {
		brokers[1].stop()
		brokers[2].stop()
		require.Eventually(t, func() bool {
			a, _ := entrypointStatus(addresses[0], "/readyz", "")
			b, _ := entrypointStatus(addresses[1], "/readyz", "")
			return a == 503 && b == 503
		}, 15*time.Second, 100*time.Millisecond, "quorum loss rejects both processes")
		for _, address := range addresses {
			status, _ := entrypointStatus(address, "/healthz", "")
			require.Equal(t, 200, status)
		}
		brokers[1], brokers[2] = startBroker(1), startBroker(2)
		ready()
		require.True(t, accepted(addresses[1]))
	}
	for _, p := range apps {
		p.stop()
	}
	if native != "" {
		for _, p := range brokers {
			p.stop()
		}
		for i := range brokers {
			brokers[i] = startBroker(i)
		}
		require.Eventually(t, func() bool {
			return natsresources.Verify(ctx, nc, resources) == nil && natscluster.WaitForQuorum(ctx, nc) == nil
		}, 30*time.Second, 100*time.Millisecond, "persisted broker resources ready before backend restart")
	}
	for i := range apps {
		apps[i] = startFixtureProcess(t, binary, backend, env, "-addr", addresses[i])
		allApps = append(allApps, apps[i])
	}
	ready()
	for _, address := range addresses {
		require.True(t, accepted(address), "full restart replays acknowledged result")
	}
	for _, p := range allApps {
		require.NotContains(t, p.log.text(), token)
		require.NotContains(t, p.log.text(), "backend-local-only")
	}
}
