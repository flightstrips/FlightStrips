// Command amanbrowser runs an isolated production application for browser E2E
// tests. Only external HTTP feeds and identity verification are substituted.
// It is deliberately outside cmd/server and never part of a deployed binary.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testtools"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type harness struct {
	mu          sync.Mutex
	epoch       time.Time
	clock       *testtools.Clock
	scenario    string
	feedURL     string
	es          *websocket.Conn
	gainLoss    []*es.AMANGainLossEvent
	rawGainLoss [][]byte
	observer    *websocket.Conn
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Setenv("TEST_MODE", "true")
	os.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", "config/test/ICAO_Aircraft.json")
	if err := config.InitConfig(); err != nil {
		return err
	}
	pool, _, cleanup, err := testdata.OpenTestDB(ctx)
	if err != nil {
		return err
	}
	defer testdata.ShutdownTestDB()
	defer cleanup()
	defer pool.Close()
	if _, err := pool.Exec(ctx, "INSERT INTO airports (name) VALUES ('EKCH') ON CONFLICT DO NOTHING"); err != nil {
		return err
	}
	h := &harness{epoch: time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC), scenario: "healthy", clock: testtools.NewClock()}
	h.clock.Set(h.epoch)
	feeds := httptest.NewServer(http.HandlerFunc(h.feed))
	defer feeds.Close()
	h.feedURL = feeds.URL
	// Fail closed: even accidental newly introduced outbound calls cannot reach
	// live services. Real adapters still make HTTP requests to a local server.
	original := http.DefaultTransport
	http.DefaultTransport = fixtureTransport{upstream: original, fixtureURL: feeds.URL}
	defer func() { http.DefaultTransport = original }()
	application, err := app.Build(ctx, app.Config{
		Environment: "test", EnableVATSIM: true, Navigation: navigation.Config{Source: navigation.SourceAIRACNet, TerminalGeometryPath: navigation.DefaultTerminalGeometryPath},
		AMAN: aman.RuntimeConfig{Mode: aman.ModeAuthoritative, SourceMode: aman.ObservationSourceVATSIM, EnabledAirports: []string{"EKCH"}, ReconciliationInterval: 200 * time.Millisecond, SurveillanceInterval: 100 * time.Millisecond, EnableEuroScopeGainLoseTags: true, EnableHoldingEATWriteback: true},
	}, app.Dependencies{Now: h.clock.Now, DBPool: pool, AuthenticationService: browserIdentity{}, VATSIMStatusURL: feeds.URL + "/status", VATSIMPollInterval: 100 * time.Millisecond})
	if err != nil {
		return err
	}
	defer application.Close(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("/e2e/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		w.WriteHeader(204)
		go stop()
	})
	mux.HandleFunc("/e2e/ready", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{"ready": true}) })
	mux.HandleFunc("/e2e/scenario", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		var request struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch request.Name {
		case "healthy", "spike", "unreliable", "recovery", "outage":
		default:
			http.Error(w, "unknown scenario", 400)
			return
		}
		h.mu.Lock()
		h.scenario = request.Name
		if request.Name == "unreliable" {
			h.clock.Advance(121 * time.Second)
		} else {
			h.clock.Advance(time.Second)
		}
		h.mu.Unlock()
		writeJSON(w, map[string]any{"scenario": request.Name})
	})
	mux.HandleFunc("/e2e/persisted", func(w http.ResponseWriter, r *http.Request) {
		var session int32
		if err := pool.QueryRow(r.Context(), "SELECT id FROM sessions WHERE airport='EKCH' AND name='LIVE'").Scan(&session); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		state, err := postgres.NewAMANRepository(pool).LoadAirportState(aman.WithSession(r.Context(), session), "EKCH")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, state)
	})
	mux.HandleFunc("/e2e/euroscope", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		writeJSON(w, map[string]any{"events": h.gainLoss, "frames": h.rawGainLoss})
	})
	mux.Handle("/", application.Handler())
	address := os.Getenv("AMAN_E2E_ADDR")
	if address == "" {
		address = "127.0.0.1:4180"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	mux.HandleFunc("/e2e/observer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		if h.observer == nil {
			if err := h.connectObserver(ctx, "ws://"+listener.Addr().String()+"/euroscopeEvents"); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		w.WriteHeader(204)
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("server: %v", err)
			stop()
		}
	}()
	application.StartWorkers(ctx)
	if err := h.connectEuroScope(ctx, "ws://"+listener.Addr().String()+"/euroscopeEvents"); err != nil {
		return err
	}
	defer h.es.Close()
	defer func() {
		if h.observer != nil {
			h.observer.Close()
		}
	}()
	log.Printf("AMAN browser E2E ready at http://%s", listener.Addr())
	<-ctx.Done()
	return nil
}

// Identity verification is an external boundary. Roles and observer restrictions
// still come from controllers created through the genuine EuroScope login.
type browserIdentity struct{}

func (browserIdentity) Validate(token string) (shared.AuthenticatedUser, error) {
	var cid string
	switch token {
	case services.TestToken:
		cid = "900001"
	case services.TestFrontendToken:
		cid = "900002"
	default:
		return shared.AuthenticatedUser{}, errors.New("unknown browser E2E identity")
	}
	return shared.NewAuthenticatedUser(cid, 0, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())}}), nil
}

func (h *harness) connectEuroScope(ctx context.Context, url string) error {
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return err
	}
	h.es = conn
	send := func(message proto.Message, kind es.EventType) error {
		data, err := es.MarshalEnvelope(message, kind)
		if err != nil {
			return err
		}
		return conn.WriteMessage(websocket.BinaryMessage, data)
	}
	if err := send(&es.TokenEvent{Token: services.TestToken}, es.Authentication); err != nil {
		return err
	}
	if err := send(&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "131.040", Callsign: "EKDK_FMP", Range: 300}, es.Login); err != nil {
		return err
	}
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			kind, payload, err := es.UnmarshalEnvelope(data)
			if err != nil {
				log.Printf("invalid ES envelope: %v", err)
				continue
			}
			if kind == es.SessionInfo {
				info := &es.SessionInfoEvent{}
				if proto.Unmarshal(payload, info) == nil && info.Role == es.SessionInfoMaster {
					if err := send(&es.SyncEvent{Runways: []*es.Runway{{Name: "22L", Arrival: true}}}, es.Sync); err != nil {
						log.Printf("sync: %v", err)
					}
				}
			}
			if kind == es.AMANGainLoss {
				event := &es.AMANGainLossEvent{}
				if err := proto.Unmarshal(payload, event); err != nil {
					log.Printf("invalid ES gain/loss: %v", err)
					continue
				}
				h.mu.Lock()
				h.gainLoss = append(h.gainLoss, event)
				h.rawGainLoss = append(h.rawGainLoss, append([]byte(nil), data...))
				h.mu.Unlock()
			}
		}
	}()
	return nil
}

func (h *harness) connectObserver(ctx context.Context, url string) error {
	conn, response, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return err
	}
	h.observer = conn
	for _, message := range []struct {
		value proto.Message
		kind  es.EventType
	}{
		{&es.TokenEvent{Token: services.TestFrontendToken}, es.Authentication},
		{&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "131.040", Callsign: "E2E_OBS", Observer: true, Range: 300}, es.Login},
	} {
		data, err := es.MarshalEnvelope(message.value, message.kind)
		if err != nil {
			return err
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			return err
		}
	}
	ready := make(chan struct{})
	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			kind, payload, err := es.UnmarshalEnvelope(data)
			if err == nil && kind == es.SessionInfo {
				event := &es.SessionInfoEvent{}
				if proto.Unmarshal(payload, event) == nil && event.Role == es.SessionInfoObserver {
					select {
					case <-ready:
					default:
						close(ready)
					}
				}
			}
		}
	}()
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	select {
	case <-ready:
		return nil
	case <-timeout.C:
		return errors.New("observer role acknowledgement timed out")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
