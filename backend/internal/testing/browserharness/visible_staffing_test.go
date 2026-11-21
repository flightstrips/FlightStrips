package main

import (
	"context"
	"net/http/httptest"
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
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Production app, migrations and socket dispatch. Only the external SDK and
// identity provider are fixtures; no CoverageSimulated dependency/file is used.
// The public datafeed is disabled for LIVE, SWEATBOX and playback alike.
func TestVisibleControllersBootstrapMultiAirportAndRejoinWithoutDatafeed(t *testing.T) {
	t.Chdir("../../..")
	t.Setenv("TEST_MODE", "true")
	aircraft, err := filepath.Abs("config/test/ICAO_Aircraft.json")
	require.NoError(t, err)
	t.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", aircraft)
	require.NoError(t, config.InitEKCHTestConfig())
	for _, environment := range []string{"LIVE", "SWEATBOX", "PLAYBACK_EXERCISE1"} {
		t.Run(environment, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pool, _, cleanup, err := testdata.OpenTestDB(ctx)
			require.NoError(t, err)
			defer cleanup()
			defer pool.Close()
			_, err = pool.Exec(ctx, "INSERT INTO airports (name) VALUES ('EKCH'),('EKBI'),('EKYT') ON CONFLICT DO NOTHING")
			require.NoError(t, err)
			selected := newRADISSuite()
			defer selected.close()
			selected.config.AirportRepositoryRoot = ".."
			selected.config.EnableVATSIM = false
			selected.config.EnableTransceivers = false
			selected.dependencies.VATSIMStatusURL = "http://127.0.0.1:1/unavailable"
			selected.dependencies.DBPool = pool
			selected.dependencies.AuthenticationService = browserIdentity{}
			application, err := app.Build(ctx, selected.config, selected.dependencies)
			require.NoError(t, err)
			defer application.Close(context.Background())
			server := httptest.NewServer(application.Handler())
			defer server.Close()
			application.StartWorkers(ctx)
			endpoint := "ws" + strings.TrimPrefix(server.URL, "http") + "/euroscopeEvents"
			self := &es.VisibleController{Callsign: "EKDK_CTR", PrimaryFrequency: "136.555"}
			appPeer := &es.VisibleController{Callsign: "EKBI_APP", PrimaryFrequency: "127.580"}
			type peer struct {
				close  func()
				member func(string) *es.MembershipContext
				report func(int64, ...*es.VisibleController) error
			}
			connect := func(env, instance string) peer {
				t.Helper()
				var mu sync.Mutex
				members := map[string]*es.MembershipContext{}
				var send func(proto.Message, es.EventType) error
				snapshot := &es.SyncEvent{Runways: []*es.Runway{{Name: "27", Arrival: true, Departure: true}}, Strips: []*es.Strip{{Callsign: "SBTEST1", Origin: "EKBI", Destination: "EKCH", AircraftType: "A320", HasFp: true, Position: &es.Position{Lat: 55.74032194, Lon: 9.15177778, Altitude: 247}}}}
				conn, err := connectFixtureControllerProtocol(ctx, endpoint, services.TestToken, &es.LoginEvent{Airport: "EKBI", Connection: env, Callsign: "EKDK_CTR", Position: "136.555", Range: 300}, snapshot,
					func(_ es.EventType, _, raw []byte) {
						var event es.Envelope
						if proto.Unmarshal(raw, &event) != nil || event.Context == nil || event.Context.MembershipId == "" {
							return
						}
						mu.Lock()
						defer mu.Unlock()
						if event.GetSessionInfo() != nil {
							members[event.Context.Airport] = proto.Clone(event.Context).(*es.MembershipContext)
						}
						if event.GetMembershipRemoved() != nil && members[event.Context.Airport].GetMembershipId() == event.Context.MembershipId {
							delete(members, event.Context.Airport)
						}
					}, instance, nil, func(fn func(proto.Message, es.EventType) error) { send = fn },
					func(control *es.ControlEvent) {
						control.RelevantAirport = "EKCH"
						control.EligibleAirports = []string{"EKCH", "EKBI"}
					})
				require.NoError(t, err)
				return peer{close: func() { conn.Close() }, member: func(airport string) *es.MembershipContext {
					mu.Lock()
					defer mu.Unlock()
					if members[airport] == nil {
						return nil
					}
					return proto.Clone(members[airport]).(*es.MembershipContext)
				}, report: func(observed int64, controllers ...*es.VisibleController) error {
					return send(&es.ControlEvent{Environment: env, Callsign: "EKDK_CTR", PrimaryFrequency: "136.555", RelevantAirport: "EKCH", EligibleAirports: []string{"EKCH", "EKBI"}, VisibleStaffing: &es.VisibleStaffing{Complete: true, Revision: uint64(observed), ObservationAgeMs: time.Now().UnixMilli() - observed, Controllers: controllers}}, es.Control)
				}}
			}
			instance := uuid.NewString()
			own := connect(environment, instance)
			defer own.close()
			require.Eventually(t, func() bool { return own.member("EKCH") != nil && own.member("EKBI") != nil }, 10*time.Second, 10*time.Millisecond)
			original := own.member("EKBI")
			require.Equal(t, environment, original.Environment)
			var view string
			require.NoError(t, pool.QueryRow(ctx, "SELECT airport FROM sessions WHERE id=$1 AND name=$2", original.SessionId, environment).Scan(&view))
			require.Equal(t, "EKBI", view)
			require.Eventually(t, func() bool {
				var count int
				_ = pool.QueryRow(ctx, "SELECT count(*) FROM strips WHERE session=$1 AND callsign='SBTEST1'", original.SessionId).Scan(&count)
				return count == 1
			}, 5*time.Second, 10*time.Millisecond)

			// A real SDK-visible controller without FlightStrips takes over EKBI.
			require.NoError(t, own.report(time.Now().UnixMilli(), self, appPeer))
			require.Eventually(t, func() bool { return own.member("EKBI") == nil }, 5*time.Second, 10*time.Millisecond)
			require.NotNil(t, own.member("EKCH"))
			require.NoError(t, own.report(time.Now().Add(-time.Minute).UnixMilli(), self))
			require.Never(t, func() bool { return own.member("EKBI") != nil }, 1100*time.Millisecond, 10*time.Millisecond)
			require.NoError(t, own.report(time.Now().UnixMilli(), self))
			require.Eventually(t, func() bool { return own.member("EKBI") != nil }, 10*time.Second, 10*time.Millisecond)
			require.NotEqual(t, original.MembershipId, own.member("EKBI").MembershipId)

			// Another exercise's observations and takeovers cannot affect this one.
			otherEnvironment := "SWEATBOX_OTHER"
			if strings.HasPrefix(environment, "PLAYBACK") {
				otherEnvironment = "PLAYBACK_OTHER"
			}
			other := connect(otherEnvironment, uuid.NewString())
			defer other.close()
			require.NotEqual(t, own.member("EKBI").SessionId, other.member("EKBI").SessionId)
			require.NoError(t, other.report(time.Now().UnixMilli(), self, appPeer))
			require.Eventually(t, func() bool { return other.member("EKBI") == nil }, 5*time.Second, 10*time.Millisecond)
			require.NotNil(t, own.member("EKBI"))
			beforeReconnect := own.member("EKBI")
			own.close()
			reconnected := connect(environment, instance)
			defer reconnected.close()
			require.NotEqual(t, beforeReconnect.TransportGeneration, reconnected.member("EKBI").TransportGeneration)
			require.NotEqual(t, beforeReconnect.MembershipId, reconnected.member("EKBI").MembershipId)
		})
	}
}
