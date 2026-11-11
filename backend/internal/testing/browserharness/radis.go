package main

import (
	"FlightStrips/internal/app"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
)

type radisCommand struct {
	Airport     string `json:"airport"`
	Environment string `json:"environment"`
	Token       string `json:"token"`
	Kind        string `json:"kind"`
	Callsign    string `json:"callsign"`
}

// This suite runs app.Build, migrations, both scoped hubs, the CDM/SAT services,
// shared feed clients and the production frontend. Only external identity/feeds
// and the external ES SDK peer are fixtures. It cannot be selected in deployment.
func newRADISSuite() suite {
	var mu sync.Mutex
	// Connection handshakes can deliver command callbacks that acquire mu.
	// Serialize own-peer lifecycle separately so those callbacks can drain.
	var ownMu sync.Mutex
	var clients []*websocket.Conn
	var commands []radisCommand
	epoch := time.Now().UTC()
	var pool *pgxpool.Pool
	var own *websocket.Conn
	var ekchOwn *websocket.Conn
	var endpoint string
	var primarySend func(proto.Message, es.EventType) error
	instance := uuid.NewString()
	ekchInstance := uuid.NewString()
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		switch {
		case r.URL.Path == "/status":
			writeJSON(w, map[string]any{"data": map[string]any{"v3": []string{"http://" + r.Host + "/vatsim"}}})
		case r.URL.Path == "/vatsim":
			pilots := []any{}
			for i := range 24 {
				pilots = append(pilots, map[string]any{"cid": 910000 + i, "callsign": fmt.Sprintf("FIND%02d", i), "latitude": 2, "longitude": 2, "altitude": 9000, "groundspeed": 200, "logon_time": epoch.Add(-time.Hour), "last_updated": epoch, "flight_plan": map[string]any{"flight_rules": "I", "aircraft": "A320/M", "aircraft_short": "A320", "departure": "EKBI", "arrival": "EKYT", "enroute_time": "0130", "revision_id": 1}})
			}
			// Keep the ES snapshot flights online in the external feed too. Otherwise
			// the arrival lifecycle correctly releases their stands as disconnected.
			for i, callsign := range []string{"RADIS1", "RADIS2"} {
				pilots = append(pilots, map[string]any{"cid": 920000 + i, "callsign": callsign, "latitude": 57.09278889, "longitude": 9.84916389, "altitude": 100, "groundspeed": 0, "logon_time": epoch.Add(-time.Hour), "last_updated": epoch, "flight_plan": map[string]any{"flight_rules": "I", "aircraft": "A320/M", "aircraft_short": "A320", "departure": "EKBI", "arrival": "EKYT", "enroute_time": "0130", "revision_id": 1}})
			}
			writeJSON(w, map[string]any{"general": map[string]any{"update_timestamp": now}, "pilots": pilots, "prefiles": []any{}, "controllers": []any{}})
		case strings.Contains(r.URL.Path, "afv-atis"):
			writeJSON(w, []any{map[string]any{"callsign": "EKBI_ATIS", "frequency": "118.000", "atis_code": "B", "text_atis": []string{"Synthetic EKBI weather"}, "last_updated": now}, map[string]any{"callsign": "EKYT_ATIS", "frequency": "119.000", "atis_code": "Y", "text_atis": []string{"Synthetic EKYT weather"}, "last_updated": now}})
		default:
			airport := strings.TrimPrefix(r.URL.Path, "/")
			qnh := "1001"
			if airport == "EKYT" {
				qnh = "1002"
			}
			fmt.Fprintf(w, "%s 061200Z 09005KT CAVOK 10/05 Q%s", airport, qnh)
		}
	}))
	original := http.DefaultTransport
	http.DefaultTransport = fixtureTransport{upstream: original, redirects: map[string]string{"metar.vatsim.net": feed.URL, "data.vatsim.net": feed.URL}}
	connect := func(ctx context.Context, airport, environment, token, callsign, frequency string, snapshot *es.SyncEvent, physical string, expose func(func(proto.Message, es.EventType) error)) (*websocket.Conn, error) {
		return connectFixtureControllerProtocol(ctx, endpoint, token, &es.LoginEvent{Airport: airport, Connection: environment, Position: frequency, Callsign: callsign, Range: 300}, snapshot, nil, physical, func(event *es.Envelope) *es.Envelope {
			kind, payload, err := es.UnmarshalEnvelope(mustProto(event))
			if err != nil {
				return nil
			}
			if event.CommandOrigin == es.CommandOrigin_COMMAND_ORIGIN_HUMAN || kind == es.SendFrequencyMessage {
				mu.Lock()
				commands = append(commands, radisCommand{airport, environment, token, kind.String(), commandCallsign(kind, payload)})
				mu.Unlock()
			}
			switch kind {
			case es.ClearedAltitude, es.SetHeading, es.AssignedSquawk, es.GroundState, es.ClearedFlag, es.Stand:
				if event.ExecutionId != "" {
					return proto.Clone(event).(*es.Envelope)
				}
			case es.SendFrequencyMessage:
				request := event.GetSendFrequencyMessage()
				return &es.Envelope{Context: proto.Clone(event.Context).(*es.MembershipContext), CommandOrigin: event.CommandOrigin, Event: &es.Envelope_FrequencyMessageResult{FrequencyMessageResult: &es.FrequencyMessageResultEvent{RequestId: request.RequestId, Callsign: request.Callsign, Submitted: true, AppliedActions: request.AppliedActions}}}
			}
			return nil
		}, expose)
	}
	reconnect := func(ctx context.Context) (*websocket.Conn, error) {
		return connect(ctx, "EKBI", "LIVE", services.TestFrontendToken, "EKBI_APP", "127.580", nil, instance, nil)
	}
	reconnectEKCH := func(ctx context.Context) (*websocket.Conn, error) {
		return connect(ctx, "EKCH", "LIVE", services.TestFrontendToken, "EKCH_GW_TWR", "118.580", nil, ekchInstance, nil)
	}
	return suite{
		config: app.Config{Environment: "test", AirportFiles: []string{"backend/config/airports/ekch.airport.yaml", "backend/config/airports/ekbi.airport.yaml", "backend/config/airports/ekyt.airport.yaml"}, EnableCDMConfigStore: true, EnableStandAssignment: true, EnableMetar: true, EnableVATSIM: true},
		dependencies: app.Dependencies{VATSIMStatusURL: feed.URL + "/status", VATSIMPollInterval: time.Hour, CoverageSimulated: func(context.Context) ([]euroscope.CoverageStaffingEvidence, error) {
			// The coordinator captures its evaluation time before reading this
			// inventory. Model already-observed data, never future-dated evidence.
			observedAt := time.Now().Add(-time.Second)
			return []euroscope.CoverageStaffingEvidence{{Environment: "SWEATBOX_MA08", Timestamp: observedAt, Inventory: euroscope.CoverageFresh}, {Environment: "PLAYBACK_MA08", Timestamp: observedAt, Inventory: euroscope.CoverageFresh}}, nil
		}},
		close: func() {
			mu.Lock()
			defer mu.Unlock()
			for _, c := range clients {
				c.Close()
			}
			feed.Close()
			http.DefaultTransport = original
		},
		register: func(mux *http.ServeMux, db *pgxpool.Pool, _ string) {
			pool = db
			mux.HandleFunc("GET /e2e/persisted", func(w http.ResponseWriter, r *http.Request) {
				airport := r.URL.Query().Get("airport")
				environment := r.URL.Query().Get("environment")
				if environment == "" {
					environment = "LIVE"
				}
				if airport != "EKCH" && airport != "EKBI" && airport != "EKYT" {
					http.Error(w, "airport required", 400)
					return
				}
				if environment != "LIVE" && environment != "SWEATBOX_MA08" && environment != "PLAYBACK_MA08" {
					http.Error(w, "unknown fixture environment", 400)
					return
				}
				var id int32
				if err := pool.QueryRow(r.Context(), "SELECT id FROM sessions WHERE airport=$1 AND name=$2", airport, environment).Scan(&id); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				rows, err := postgres.NewStripRepository(pool).List(r.Context(), id)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				writeJSON(w, rows)
			})
			mux.HandleFunc("GET /e2e/commands", func(w http.ResponseWriter, r *http.Request) { mu.Lock(); defer mu.Unlock(); writeJSON(w, commands) })
			mux.HandleFunc("POST /e2e/own-plugin/disconnect", func(w http.ResponseWriter, r *http.Request) {
				ownMu.Lock()
				defer ownMu.Unlock()
				selected := &own
				if r.URL.Query().Get("airport") == "EKCH" {
					selected = &ekchOwn
				}
				if *selected != nil {
					(*selected).Close()
					*selected = nil
				}
				w.WriteHeader(204)
			})
			mux.HandleFunc("POST /e2e/own-plugin/reconnect", func(w http.ResponseWriter, r *http.Request) {
				ownMu.Lock()
				defer ownMu.Unlock()
				selected, connectOwn := &own, reconnect
				if r.URL.Query().Get("airport") == "EKCH" {
					selected, connectOwn = &ekchOwn, reconnectEKCH
				}
				if *selected != nil {
					http.Error(w, "already connected", 409)
					return
				}
				next, err := connectOwn(r.Context())
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				*selected = next
				mu.Lock()
				clients = append(clients, next)
				mu.Unlock()
				w.WriteHeader(204)
			})
			mux.HandleFunc("POST /e2e/runways", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				send := primarySend
				mu.Unlock()
				if err := send(&es.RunwayEvent{Runways: []*es.Runway{{Name: "27", Departure: true, Arrival: true}}}, es.RunwayType); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.WriteHeader(204)
			})
		},
		connect: func(ctx context.Context, url string) error {
			endpoint = url
			// Keep the existing EKCH workflow active in the same application as
			// RADIS, including two independently bound controllers and services.
			ekch, err := connect(ctx, "EKCH", "LIVE", services.TestToken, "EKCH_A_TWR", "118.105", stripSnapshot(), uuid.NewString(), nil)
			if err != nil {
				return err
			}
			clients = append(clients, ekch)
			deadlineEKCH := time.NewTimer(15 * time.Second)
			defer deadlineEKCH.Stop()
			tickEKCH := time.NewTicker(20 * time.Millisecond)
			defer tickEKCH.Stop()
			for {
				var count int
				err := pool.QueryRow(ctx, "SELECT count(*) FROM strips JOIN sessions ON sessions.id=strips.session WHERE sessions.airport='EKCH' AND sessions.name='LIVE' AND euroscope_seen_at IS NOT NULL").Scan(&count)
				if err != nil {
					return err
				}
				if count == 6 {
					break
				}
				select {
				case <-tickEKCH.C:
				case <-deadlineEKCH.C:
					return fmt.Errorf("EKCH master snapshot did not persist")
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			ekchOwn, err = reconnectEKCH(ctx)
			if err != nil {
				return err
			}
			clients = append(clients, ekchOwn)
			for _, selected := range []struct{ airport, environment string }{{"EKBI", "LIVE"}, {"EKYT", "LIVE"}, {"EKBI", "SWEATBOX_MA08"}, {"EKBI", "PLAYBACK_MA08"}} {
				airport, environment := selected.airport, selected.environment
				frequency, runway := "119.005", "09"
				latitude, longitude := 55.74032194, 9.15177778
				if airport == "EKYT" {
					frequency, runway = "118.305", "08R"
					latitude, longitude = 57.09278889, 9.84916389
				}
				snapshot := &es.SyncEvent{Controllers: []*es.Controller{{Position: frequency, Callsign: airport + "_TWR"}}, Runways: []*es.Runway{{Name: runway, Departure: true, Arrival: true}}}
				for _, callsign := range []string{"RADIS1", "RADIS2"} {
					snapshot.Strips = append(snapshot.Strips, &es.Strip{Callsign: callsign, Origin: "EKBI", Destination: "EKYT", AircraftType: "A320", AircraftCategory: "M", CommunicationType: "T", HasFp: true, GroundState: es.GroundStateStartup, ClearedAltitude: 3000, RequestedAltitude: 25000, AssignedSquawk: "2401", Route: "DCT TEST", Eobt: time.Now().UTC().Add(time.Hour).Format("1504"), EnrouteDuration: "0130", Runway: runway, Position: &es.Position{Lat: latitude, Lon: longitude, Altitude: 100}})
				}
				var expose func(func(proto.Message, es.EventType) error)
				if airport == "EKBI" && environment == "LIVE" {
					expose = func(send func(proto.Message, es.EventType) error) { mu.Lock(); primarySend = send; mu.Unlock() }
				}
				c, err := connect(ctx, airport, environment, services.TestToken, airport+"_TWR", frequency, snapshot, uuid.NewString(), expose)
				if err != nil {
					return err
				}
				mu.Lock()
				clients = append(clients, c)
				mu.Unlock()
			}
			deadline := time.NewTimer(15 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				var count int
				err := pool.QueryRow(ctx, "SELECT count(*) FROM strips WHERE callsign IN ('RADIS1','RADIS2') AND board_state IS NOT NULL AND euroscope_seen_at IS NOT NULL").Scan(&count)
				if err != nil {
					return err
				}
				if count == 8 {
					break
				}
				select {
				case <-tick.C:
				case <-deadline.C:
					return fmt.Errorf("regional master snapshots did not persist")
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			c, err := reconnect(ctx)
			if err != nil {
				return err
			}
			ownMu.Lock()
			own = c
			ownMu.Unlock()
			mu.Lock()
			clients = append(clients, c)
			mu.Unlock()
			return nil
		},
	}
}
func mustProto(m proto.Message) []byte { data, _ := proto.Marshal(m); return data }
func commandCallsign(kind es.EventType, payload []byte) string {
	switch kind {
	case es.ClearedAltitude:
		v := &es.ClearedAltitudeEvent{}
		_ = proto.Unmarshal(payload, v)
		return v.Callsign
	case es.SendFrequencyMessage:
		v := &es.SendFrequencyMessageEvent{}
		_ = proto.Unmarshal(payload, v)
		return v.Callsign
	case es.Stand:
		v := &es.StandEvent{}
		_ = proto.Unmarshal(payload, v)
		return v.Callsign
	default:
		return ""
	}
}
