package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/app"
	"FlightStrips/internal/navigation"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testtools"
	es "FlightStrips/pkg/events/euroscope"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

type amanSuite struct {
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

func newAMANSuite() suite {
	h := &amanSuite{epoch: time.Date(2026, 10, 26, 18, 0, 0, 0, time.UTC), scenario: "healthy", clock: testtools.NewClock()}
	h.clock.Set(h.epoch)
	feeds := httptest.NewServer(http.HandlerFunc(h.feed))

	h.feedURL = feeds.URL
	// Fail closed: even accidental newly introduced outbound calls cannot reach
	// live services. Real adapters still make HTTP requests to a local server.
	original := http.DefaultTransport
	http.DefaultTransport = fixtureTransport{upstream: original, redirects: map[string]string{"airac.net": feeds.URL, "api.open-meteo.com": feeds.URL}}

	configuration := app.Config{
		Environment: "test", EnableVATSIM: true, Navigation: navigation.Config{Source: navigation.SourceAIRACNet, TerminalGeometryPath: navigation.DefaultTerminalGeometryPath},
		AMAN: aman.RuntimeConfig{Mode: aman.ModeAuthoritative, SourceMode: aman.ObservationSourceVATSIM, EnabledAirports: []string{"EKCH"}, ReconciliationInterval: 200 * time.Millisecond, SurveillanceInterval: 100 * time.Millisecond, EnableEuroScopeGainLoseTags: true, EnableHoldingEATWriteback: true},
	}
	dependencies := app.Dependencies{Now: h.clock.Now, VATSIMStatusURL: feeds.URL + "/status", VATSIMPollInterval: 100 * time.Millisecond}
	return suite{config: configuration, dependencies: dependencies,
		close: func() {
			feeds.Close()
			http.DefaultTransport = original
			if h.es != nil {
				h.es.Close()
			}
			if h.observer != nil {
				h.observer.Close()
			}
		},
		register: func(mux *http.ServeMux, pool *pgxpool.Pool, endpoint string) {
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
			mux.HandleFunc("/e2e/observer", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "POST required", 405)
					return
				}
				if h.observer == nil {
					if err := h.connectObserver(r.Context(), endpoint); err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
				}
				w.WriteHeader(204)
			})
		},
		connect: h.connectEuroScope,
	}
}

func (h *amanSuite) connectEuroScope(ctx context.Context, endpoint string) error {
	conn, err := connectFixtureController(ctx, endpoint, services.TestToken,
		&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "131.040", Callsign: "EKDK_FMP", Range: 300},
		&es.SyncEvent{Runways: []*es.Runway{{Name: "22L", Arrival: true}}},
		func(kind es.EventType, payload, data []byte) {
			if kind != es.AMANGainLoss {
				return
			}
			event := &es.AMANGainLossEvent{}
			if err := proto.Unmarshal(payload, event); err != nil {
				log.Printf("invalid ES gain/loss: %v", err)
				return
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			h.gainLoss = append(h.gainLoss, event)
			h.rawGainLoss = append(h.rawGainLoss, append([]byte(nil), data...))
		})
	h.es = conn
	return err
}

func (h *amanSuite) connectObserver(ctx context.Context, endpoint string) error {
	conn, err := connectFixtureController(ctx, endpoint, services.TestFrontendToken,
		&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "131.040", Callsign: "E2E_OBS", Observer: true, Range: 300}, nil, nil)
	h.observer = conn
	return err
}
