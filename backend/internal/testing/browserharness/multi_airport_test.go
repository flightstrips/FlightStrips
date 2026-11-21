package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/services"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"net/http/httptest"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}

// A physical peer with two memberships. This deliberately uses the production
// app, migrated database and wire protocol rather than membershipTestHub mocks.
func TestMultiAirportReleaseSocketTakeoverRejoinAndFencing(t *testing.T) {
	original, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir("../../.."))
	defer os.Chdir(original)
	t.Setenv("TEST_MODE", "true")
	aircraft, err := filepath.Abs("config/test/ICAO_Aircraft.json")
	require.NoError(t, err)
	t.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", aircraft)
	require.NoError(t, config.InitEKCHTestConfig())
	selected := newRADISSuite()
	defer selected.close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pool, _, cleanup, err := testdata.OpenTestDB(ctx)
	require.NoError(t, err)
	defer cleanup()
	defer pool.Close()
	_, err = pool.Exec(ctx, "INSERT INTO airports (name) VALUES ('EKCH'),('EKBI'),('EKYT') ON CONFLICT DO NOTHING")
	require.NoError(t, err)
	selected.config.AirportRepositoryRoot = ".."
	selected.dependencies.DBPool = pool
	selected.dependencies.AuthenticationService = browserIdentity{}
	application, err := app.Build(ctx, selected.config, selected.dependencies)
	require.NoError(t, err)
	defer application.Close(context.Background())
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	application.StartWorkers(ctx)
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/euroscopeEvents", nil)
	if response != nil {
		response.Body.Close()
	}
	require.NoError(t, err)
	defer conn.Close()
	instance := uuid.NewString()
	var mu, writes sync.Mutex
	members := map[string]*es.MembershipContext{}
	var transport *es.MembershipContext
	synced := map[string]bool{}
	send := func(envelope *es.Envelope) error {
		data, err := proto.Marshal(envelope)
		if err != nil {
			return err
		}
		writes.Lock()
		defer writes.Unlock()
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	control := func(frequency string, radios ...string) error {
		mu.Lock()
		scope := proto.Clone(transport).(*es.MembershipContext)
		mu.Unlock()
		return send(&es.Envelope{Context: scope, Event: &es.Envelope_Control{Control: &es.ControlEvent{Environment: "LIVE", Callsign: "MA08_CTR", PrimaryFrequency: frequency, TunedFrequencies: radios, TunedFrequenciesKnown: true, EligibleAirports: []string{"EKBI", "EKYT"}, Range: 300}}})
	}
	require.NoError(t, send(&es.Envelope{Event: &es.Envelope_Token{Token: &es.TokenEvent{Token: services.TestToken, InstanceId: instance}}}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event es.Envelope
			if proto.Unmarshal(data, &event) != nil || event.Context == nil {
				continue
			}
			scope := event.Context
			mu.Lock()
			if scope.MembershipId == "" {
				transport = proto.Clone(scope).(*es.MembershipContext)
				mu.Unlock()
				if control("119.005", "119.005", "118.305") != nil {
					return
				}
				continue
			}
			if event.GetMembershipRemoved() != nil {
				delete(members, scope.Airport)
				mu.Unlock()
				continue
			}
			if info := event.GetSessionInfo(); info != nil {
				members[scope.Airport] = proto.Clone(scope).(*es.MembershipContext)
				key := scope.MembershipId
				first := info.Role == es.SessionInfoMaster && !synced[key]
				if first {
					synced[key] = true
				}
				mu.Unlock()
				if first {
					snapshot := &es.SyncEvent{Controllers: []*es.Controller{{Callsign: "MA08_CTR", Position: "119.005"}}, Strips: []*es.Strip{{Callsign: "SAME123", Origin: "EKBI", Destination: "EKYT", AircraftType: "A320", HasFp: true, GroundState: es.GroundStateStartup, AssignedSquawk: "2401", Position: &es.Position{Lat: 55.74032194, Lon: 9.15177778, Altitude: 100}}}}
					probe := proto.Clone(snapshot.Strips[0]).(*es.Strip)
					probe.Callsign = "FENCEPROBE"
					snapshot.Strips = append(snapshot.Strips, probe)
					if send(&es.Envelope{Context: scope, Event: &es.Envelope_Sync{Sync: snapshot}}) != nil {
						return
					}
				}
				continue
			}
			mu.Unlock()
		}
	}()
	defer func() { conn.Close(); <-done }()
	member := func(airport string) *es.MembershipContext {
		mu.Lock()
		defer mu.Unlock()
		if value := members[airport]; value != nil {
			return proto.Clone(value).(*es.MembershipContext)
		}
		return nil
	}
	stateFor := func(airport, callsign string) string {
		var value string
		_ = pool.QueryRow(ctx, "SELECT state FROM strips JOIN sessions ON sessions.id=strips.session WHERE sessions.airport=$1 AND sessions.name='LIVE' AND callsign=$2", airport, callsign).Scan(&value)
		return value
	}
	state := func(airport string) string { return stateFor(airport, "SAME123") }
	require.Eventually(t, func() bool {
		return member("EKBI") != nil && member("EKYT") != nil && state("EKBI") == es.GroundStateStartup && state("EKYT") == es.GroundStateStartup
	}, 15*time.Second, 10*time.Millisecond)
	a, b := member("EKBI"), member("EKYT")
	require.Equal(t, a.TransportGeneration, b.TransportGeneration)
	require.Equal(t, a.InstanceId, b.InstanceId)
	require.NotEqual(t, a.SessionId, b.SessionId)
	ground := func(scope *es.MembershipContext, value string) error {
		return send(&es.Envelope{Context: scope, Event: &es.Envelope_GroundState{GroundState: &es.GroundStateEvent{Callsign: "SAME123", GroundState: value}}})
	}
	// An update to a different strip on the same physical socket proves that
	// preceding fenced writes were processed without overwriting their effects.
	barrier := func(value string) {
		require.NoError(t, send(&es.Envelope{Context: b, Event: &es.Envelope_GroundState{GroundState: &es.GroundStateEvent{Callsign: "FENCEPROBE", GroundState: value}}}))
		require.Eventually(t, func() bool { return stateFor("EKYT", "FENCEPROBE") == value }, 5*time.Second, 10*time.Millisecond)
	}
	foreign := proto.Clone(a).(*es.MembershipContext)
	foreign.SessionId = b.SessionId
	require.NoError(t, ground(foreign, es.GroundStateDepart))
	barrier(es.GroundStateTaxi)
	require.Equal(t, es.GroundStateStartup, state("EKBI"))
	require.Equal(t, es.GroundStateStartup, state("EKYT"))
	require.NoError(t, ground(b, es.GroundStateTaxi))
	require.Eventually(t, func() bool { return state("EKYT") == es.GroundStateTaxi }, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, es.GroundStateStartup, state("EKBI"))
	// A tuned-radio change removes only EKBI; the socket and EKYT incarnation
	// remain usable. Return coverage through that same zero/new-membership path.
	require.NoError(t, control("118.305", "118.305"))
	require.Eventually(t, func() bool { return member("EKBI") == nil }, 5*time.Second, 10*time.Millisecond)
	require.True(t, proto.Equal(b, member("EKYT")))
	require.NoError(t, ground(a, es.GroundStateDepart))
	barrier(es.GroundStateParked)
	require.Equal(t, es.GroundStateStartup, state("EKBI"))
	require.Equal(t, es.GroundStateTaxi, state("EKYT"))
	require.NoError(t, ground(b, es.GroundStateParked))
	require.Eventually(t, func() bool { return state("EKYT") == es.GroundStateParked }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, control("119.005", "119.005", "118.305"))
	require.Eventually(t, func() bool { return member("EKBI") != nil && member("EKBI").MembershipId != a.MembershipId }, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, ground(a, es.GroundStateDepart))
	barrier(es.GroundStateTaxi)
	require.Equal(t, es.GroundStateStartup, state("EKBI"))
	require.Equal(t, es.GroundStateParked, state("EKYT"))
	require.NoError(t, ground(b, es.GroundStateTaxi))
	require.Eventually(t, func() bool { return state("EKYT") == es.GroundStateTaxi }, 5*time.Second, 10*time.Millisecond)
	require.Equal(t, es.GroundStateStartup, state("EKBI"))
}
