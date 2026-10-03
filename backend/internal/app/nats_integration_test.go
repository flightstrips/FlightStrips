package app

import (
	"FlightStrips/internal/cluster"
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/euroscopebinary"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type runtimeAuth struct{}

func (runtimeAuth) Validate(token string) (shared.AuthenticatedUser, error) {
	if token == "invalid" {
		return shared.AuthenticatedUser{}, errors.New("invalid token")
	}
	return shared.NewAuthenticatedUser(token, 1, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())}}), nil
}

type runtimeFixture struct {
	t       *testing.T
	ctx     context.Context
	cfg     Config
	deps    Dependencies
	admin   *nats.Conn
	apps    [2]*App
	servers [2]*httptest.Server
	session int32
	name    string
	closed  [2]bool
}

func newRuntimeFixture(t *testing.T, configure func(*Config, *Dependencies)) *runtimeFixture {
	t.Helper()
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires isolated pinned three-node fixture")
	}
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(filepath.Join(cwd, "../..")))
	err = appconfig.InitConfig()
	_ = os.Chdir(cwd)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	port := 5322
	if value := os.Getenv("NATS_TEST_PORT_BASE"); value != "" {
		port, err = strconv.Atoi(value)
		require.NoError(t, err)
	}
	urls := func(user string) []string {
		out := []string{}
		for i := 0; i < 3; i++ {
			out = append(out, fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+i))
		}
		return out
	}
	resources := natsresources.Config{URLs: urls("bootstrap"), Names: natsresources.RequiredNames, ConnectTimeout: 2 * time.Second, RequestTimeout: 2 * time.Second}
	admin, err := natsresources.Connect(resources)
	require.NoError(t, err)
	t.Cleanup(admin.Close)
	require.NoError(t, natscluster.WaitForQuorum(ctx, admin))
	require.NoError(t, natsresources.Bootstrap(ctx, admin, resources))
	resources.URLs = urls("backend")
	key := filepath.Join(t.TempDir(), "effects.key")
	require.NoError(t, os.WriteFile(key, []byte("01234567890123456789012345678901"), 0600))
	f := &runtimeFixture{t: t, ctx: ctx, admin: admin, name: "TASK20A-" + strings.ToUpper(uuid.NewString()), cfg: Config{Environment: "test", NATS: NATSConfig{Resources: resources, EffectKeyID: "test", EffectKeyFiles: map[string]string{"test": key}, Airports: []string{"EKCH"}}, EnablePDC: true, EnablePilotAPI: true, EnableEFB: true, EnableECFMPAPI: true, EnableTraffic: true}, deps: Dependencies{AuthenticationService: runtimeAuth{}}}
	if configure != nil {
		configure(&f.cfg, &f.deps)
	}
	// Production starts in backend; SAT lazily resolves its reference files
	// through config.GetConfigDir. Keep that same working directory until the
	// applications have joined all workers (these fixtures are not parallel).
	require.NoError(t, os.Chdir(filepath.Join(cwd, "../..")))
	t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
	t.Cleanup(func() {
		// Each test owns its registered session in this disposable cluster.
		// Release the LIVE name through the real lifecycle before closing nodes.
		if f.session != 0 {
			for i, app := range f.apps {
				if app == nil || f.closed[i] {
					continue
				}
				cleanup, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				var deletionErr error
				for cleanup.Err() == nil {
					deletionErr = app.natsRuntime.registry.TombstoneSession(cleanup, f.session)
					if deletionErr == nil {
						deletionErr = app.natsRuntime.registry.FinalizeDeletion(cleanup, f.session)
					}
					if deletionErr == nil {
						break
					}
					select {
					case <-cleanup.Done():
					case <-time.After(50 * time.Millisecond):
					}
				}
				cancel()
				if deletionErr != nil {
					t.Errorf("release fixture session: %v", deletionErr)
				}
				break
			}
		}
		for i := range f.apps {
			if f.apps[i] != nil && !f.closed[i] {
				shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
				err := f.apps[i].Close(shutdown)
				stop()
				if err != nil {
					t.Errorf("close node %d: %v", i, err)
				}
			}
			if f.servers[i] != nil {
				f.servers[i].Close()
			}
		}
	})
	for i := range f.apps {
		f.apps[i], err = BuildNATS(ctx, f.cfg, f.deps)
		require.NoError(t, err)
		f.apps[i].StartWorkers(ctx)
		f.servers[i] = httptest.NewServer(f.apps[i].Handler())
	}
	f.await("two ready applications", func() bool { return f.apps[0].natsRuntime.ready() == nil && f.apps[1].natsRuntime.ready() == nil })
	return f
}
func (f *runtimeFixture) await(label string, fn func() bool) {
	f.t.Helper()
	require.Eventually(f.t, fn, 35*time.Second, 25*time.Millisecond, label)
}
func (f *runtimeFixture) owner(ref *pb.AggregateRef) int {
	f.t.Helper()
	index := -1
	f.await("accepted aggregate owner", func() bool {
		for i, app := range f.apps {
			if !f.closed[i] && app.natsRuntime.owner.CanWrite(ref) {
				index = i
				return true
			}
		}
		return false
	})
	return index
}
func (f *runtimeFixture) state() *cluster.Aggregate {
	f.t.Helper()
	var state *cluster.Aggregate
	f.await("owner projection catches up", func() bool {
		for i, app := range f.apps {
			if f.closed[i] || !app.natsRuntime.owner.CanWrite(sessionNATSRef(f.session)) {
				continue
			}
			value, err := app.natsRuntime.projection.Read(sessionNATSRef(f.session))
			if err == nil {
				state = value
				return true
			}
		}
		return false
	})
	return state
}

type runtimeSocket struct {
	conn   *websocket.Conn
	mu     sync.Mutex
	frames []*es.Envelope
	err    error
	cid    string
	node   string
}

func (s *runtimeSocket) send(t *testing.T, frame proto.Message) {
	t.Helper()
	data, err := proto.Marshal(frame)
	require.NoError(t, err)
	require.NoError(t, s.conn.WriteMessage(websocket.BinaryMessage, data))
}
func (f *runtimeFixture) socket(node int, cid, callsign, position string, observer ...bool) *runtimeSocket {
	f.t.Helper()
	dialer := websocket.Dialer{Subprotocols: []string{euroscopebinary.Subprotocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(f.servers[node].URL, "http")+"/euroscopeEvents", nil)
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = conn.Close() })
	s := &runtimeSocket{conn: conn, cid: cid, node: f.apps[node].natsRuntime.owner.NodeID}
	s.send(f.t, &es.Envelope{Event: &es.Envelope_Token{Token: &es.TokenEvent{Token: cid, ProtocolRevision: 2}}})
	s.send(f.t, &es.Envelope{Event: &es.Envelope_Login{Login: &es.LoginEvent{Airport: "EKCH", Connection: f.name, Callsign: callsign, Position: position, Observer: len(observer) > 0 && observer[0]}}})
	go func() {
		for {
			kind, data, e := conn.ReadMessage()
			s.mu.Lock()
			if e != nil {
				s.err = e
				s.mu.Unlock()
				return
			}
			frame := &es.Envelope{}
			if kind != websocket.BinaryMessage || pb.UnmarshalStrict(data, frame) != nil {
				s.err = errors.New("bad server frame")
				s.mu.Unlock()
				return
			}
			s.frames = append(s.frames, frame)
			s.mu.Unlock()
		}
	}()
	f.t.Cleanup(func() {
		if f.t.Failed() {
			s.mu.Lock()
			f.t.Logf("socket CID=%s node=%s reader=%v frames=%d", s.cid, s.node, s.err, len(s.frames))
			s.mu.Unlock()
		}
	})
	f.await("real socket creates shared session", func() bool {
		entries, e := f.apps[node].natsRuntime.registry.ActiveSessions(f.ctx)
		if e != nil {
			return false
		}
		for _, entry := range entries {
			if entry.Name == f.name {
				f.session = entry.Id
				return true
			}
		}
		return false
	})
	f.await("socket authenticates and elects master", func() bool {
		state := f.state()
		return state.Indexes[pb.EntityKind_CONTROLLER][cid] != nil && state.Master != nil
	})
	return s
}
func (f *runtimeFixture) send(s *runtimeSocket, event *es.Envelope) {
	state := f.state()
	event.CommandId = uuid.NewString()
	event.SessionId = f.session
	event.OwnerEpoch = state.Owner.Epoch
	event.MasterEpoch = state.Master.Epoch
	s.send(f.t, event)
}
func (f *runtimeFixture) sync(s *runtimeSocket, strips ...*es.Strip) {
	f.t.Helper()
	f.await("current socket elected", func() bool {
		state := f.state()
		_, entries, err := f.apps[f.owner(state.Ref)].natsRuntime.projection.ObservationSnapshot(f.session)
		if err != nil {
			return false
		}
		for _, entry := range entries {
			client := entry.Value.GetClient()
			if client != nil && client.Cid == s.cid && client.NodeId == s.node && client.ConnectionId == state.Master.GetConnectionId() {
				return true
			}
		}
		return false
	})
	f.send(s, &es.Envelope{Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: strips, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}, Sids: []*es.SidEntry{{Name: "ODN1C", Runway: "22R"}}}}})
	f.await("shared operational sync", func() bool {
		for i, app := range f.apps {
			if f.closed[i] {
				continue
			}
			sync, e := app.natsRuntime.projection.OperationalSync(sessionNATSRef(f.session))
			if e != nil || sync == nil {
				return false
			}
		}
		return true
	})
}
func (f *runtimeFixture) get(node int, path string, status int) map[string]any {
	f.t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.servers[node].URL+path, nil)
	require.NoError(f.t, err)
	req.Header.Set("Authorization", "Bearer 111111")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(f.t, err)
	defer resp.Body.Close()
	require.Equal(f.t, status, resp.StatusCode)
	var value map[string]any
	require.NoError(f.t, json.NewDecoder(resp.Body).Decode(&value))
	return value
}

func TestBuildNATSTwoApplicationsBinaryHTTPAndTakeover(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	callsign := "S" + strings.ToUpper(uuid.NewString()[:6])
	a := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	strip := &es.Strip{Callsign: callsign, Origin: "EKCH", Destination: "EGLL", AircraftType: "A320", AssignedSquawk: "1001", Sid: "ODN1C", Runway: "22R", Route: "ODN DCT", HasFp: true}
	f.sync(a, strip)
	b := f.socket(1, "222222", "EKCH_GND", "121.600")
	_ = b
	f.await("independent replicas agree", func() bool {
		x, e := f.apps[0].natsRuntime.projection.Read(sessionNATSRef(f.session))
		y, e2 := f.apps[1].natsRuntime.projection.Read(sessionNATSRef(f.session))
		return e == nil && e2 == nil && proto.Equal(x.Indexes[pb.EntityKind_STRIP][callsign].GetValue(), y.Indexes[pb.EntityKind_STRIP][callsign].GetValue()) && x.Indexes[pb.EntityKind_STRIP][callsign] != nil
	})
	dialer := websocket.Dialer{Subprotocols: []string{frontendbinary.Subprotocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(f.servers[1].URL, "http")+"/frontEndEvents", nil)
	require.NoError(t, err)
	defer conn.Close()
	data, err := proto.Marshal(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "111111", Airport: "EKCH", SessionName: f.name}}})
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, data))
	_, data, err = conn.ReadMessage()
	require.NoError(t, err)
	initial := &pb.FrontendFrame{}
	require.NoError(t, pb.UnmarshalStrict(data, initial))
	require.NotNil(t, initial.GetInitial())
	id := uuid.NewString()
	rev := f.state().Indexes[pb.EntityKind_STRIP][callsign].Revision
	action := &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: callsign, Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: true}}}}}
	data, err = proto.Marshal(&pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id, ExpectedEntityRevision: &rev, Action: action}}})
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.BinaryMessage, data))
	f.await("browser command reaches other owner", func() bool { return f.state().Ledger[id].GetStatus() == pb.CommandOutcome_SUCCEEDED })
	require.Equal(t, "succeeded", f.get(0, "/api/commands/"+id, http.StatusOK)["status"])
	for i := 0; i < 2; i++ {
		f.get(i, "/readyz", http.StatusOK)
		f.get(i, "/api/pilot/flight?callsign="+callsign, http.StatusOK)
		response, err := http.Get(f.servers[i].URL + "/metrics")
		require.NoError(t, err)
		metrics, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		for _, name := range []string{"fs_projection_lag_seconds", "fs_owner_lease_remaining_seconds", "fs_owner_takeovers_total", "fs_stale_epoch_rejections_total", "fs_snapshot_verify_failures_total", "fs_nats_storage_bytes", "fs_nats_puback_seconds_count"} {
			require.Contains(t, string(metrics), name)
		}
		require.NotContains(t, string(metrics), "local-only")
	}
	old := f.owner(sessionNATSRef(f.session))
	epoch := f.state().Owner.Epoch
	shutdown, stop := context.WithTimeout(context.Background(), 15*time.Second)
	require.NoError(t, f.apps[old].Close(shutdown))
	stop()
	f.closed[old] = true
	f.await("surviving application takes ownership", func() bool {
		r := f.apps[1-old].natsRuntime
		if !r.owner.CanWrite(sessionNATSRef(f.session)) {
			return false
		}
		state, e := r.projection.Read(sessionNATSRef(f.session))
		return e == nil && state.Owner.Epoch > epoch && state.Indexes[pb.EntityKind_STRIP][callsign].Value.GetStrip().Marked
	})
	f.await("surviving application catches up", func() bool { return f.apps[1-old].natsRuntime.ready() == nil })
	f.get(1-old, "/readyz", http.StatusOK)
	require.Equal(t, "succeeded", f.get(1-old, "/api/commands/"+id, http.StatusOK)["status"])
	reply := f.apps[old].natsRuntime.Route(context.Background(), &pb.CommandRequest{CommandId: uuid.NewString(), Aggregate: sessionNATSRef(f.session)})
	require.Equal(t, pb.CommandReply_UNAVAILABLE, reply.Status)
	require.True(t, f.apps[old].natsRuntime.nc.IsClosed())
}

func TestBuildNATSAllSocketsReportRunwaysWithoutReplacingMasterConfiguration(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	// An initial runway report may arrive before the first operational sync.
	f.send(master, &es.Envelope{Event: &es.Envelope_Runway{Runway: &es.RunwayEvent{Runways: []*es.Runway{{Name: "22R", Departure: true}}}}})
	f.sync(master)
	slave := f.socket(1, "222222", "EKCH_GND", "121.600")
	observer := f.socket(1, "333333", "EKCH_OBS", "", true)
	f.send(slave, &es.Envelope{Event: &es.Envelope_TrackingControllerChanged{TrackingControllerChanged: &es.TrackingControllerChangedEvent{Callsign: "NOTYETOBSERVED", TrackingController: "EKCH_GND"}}})
	for _, socket := range []*runtimeSocket{slave, observer} {
		f.send(socket, &es.Envelope{Event: &es.Envelope_Runway{Runway: &es.RunwayEvent{Runways: []*es.Runway{{Name: "04L", Departure: true}}}}})
		f.await("non-master receives runway mismatch without disconnection", func() bool {
			socket.mu.Lock()
			defer socket.mu.Unlock()
			require.NoError(t, socket.err)
			for _, frame := range socket.frames {
				if alert := frame.GetRunwayMismatchAlert(); alert != nil {
					require.Equal(t, []string{"22R"}, alert.ExpectedDeparture)
					require.Equal(t, []string{"04L"}, alert.CurrentDeparture)
					return true
				}
			}
			return false
		})
	}
	canonical := f.state().Indexes[pb.EntityKind_SESSION][fmt.Sprint(f.session)].Value.GetSession()
	require.Equal(t, []*pb.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}, canonical.Runways)
	f.send(master, &es.Envelope{Event: &es.Envelope_Runway{Runway: &es.RunwayEvent{Runways: []*es.Runway{{Name: "18", Departure: true}}}}})
	f.await("master change re-evaluates both remote reports", func() bool {
		for _, socket := range []*runtimeSocket{slave, observer} {
			socket.mu.Lock()
			found := false
			for _, frame := range socket.frames {
				if alert := frame.GetRunwayMismatchAlert(); alert != nil && slices.Equal(alert.ExpectedDeparture, []string{"18"}) {
					found = true
				}
			}
			socket.mu.Unlock()
			if !found {
				return false
			}
		}
		return true
	})
}

func TestBuildNATSStripDeltaOnlyReplaysAffectedAircraft(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	f.sync(master, &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001"}, &es.Strip{Callsign: "SAS2", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1002"})
	slave := f.socket(1, "222222", "EKCH_GND", "121.600")
	f.await("initial snapshot received", func() bool {
		slave.mu.Lock()
		defer slave.mu.Unlock()
		for _, frame := range slave.frames {
			if len(frame.GetBackendSync().GetStrips()) == 2 {
				return true
			}
		}
		return false
	})
	slave.mu.Lock()
	slave.frames = nil
	slave.mu.Unlock()
	f.send(master, &es.Envelope{Event: &es.Envelope_AssignedSquawk{AssignedSquawk: &es.AssignedSquawkEvent{Callsign: "SAS1", Squawk: "1234"}}})
	f.await("only affected strip is delivered", func() bool {
		slave.mu.Lock()
		defer slave.mu.Unlock()
		require.NoError(t, slave.err)
		for _, frame := range slave.frames {
			for _, strip := range frame.GetBackendSync().GetStrips() {
				require.Equal(t, "SAS1", strip.Callsign, "unrelated aircraft must never be replayed")
				if strip.AssignedSquawk == "1234" {
					return true
				}
			}
		}
		return false
	})
}

func TestBuildNATSReconnectSyncAcceptsEobtBeforePositionFreshness(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	strip := &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001", Position: &es.Position{Lat: 55.6, Lon: 12.65}}
	f.sync(master, strip)
	require.NoError(t, master.conn.Close())
	next := f.socket(1, "111111", "EKCH_A_TWR", "118.100")
	strip.Eobt = time.Now().UTC().Add(5 * time.Minute).Format("1504")
	f.sync(next, strip)
	f.await("EOBT observation accepted during bootstrap", func() bool {
		value := f.state().Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip().Eobt
		return value != nil && value.AsTime().UTC().Format("1504") == strip.Eobt
	})
	next.mu.Lock()
	defer next.mu.Unlock()
	require.NoError(t, next.err)
}
