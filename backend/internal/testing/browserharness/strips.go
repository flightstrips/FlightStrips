package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"FlightStrips/internal/app"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
	es "FlightStrips/pkg/events/euroscope"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// No AMAN engine, navigation source, VATSIM source or AMAN clock is required.
// Controllers and flights enter through the normal binary EuroScope protocol.
func newStripsSuite() suite {
	var clients []*websocket.Conn
	var database *pgxpool.Pool
	original := http.DefaultTransport
	http.DefaultTransport = fixtureTransport{upstream: original}
	return suite{
		config:       app.Config{Environment: "test"},
		dependencies: app.Dependencies{},
		close: func() {
			for _, client := range clients {
				client.Close()
			}
			http.DefaultTransport = original
		},
		register: func(mux *http.ServeMux, pool *pgxpool.Pool, _ string) {
			database = pool
			mux.HandleFunc("/e2e/persisted", func(w http.ResponseWriter, r *http.Request) {
				var session int32
				if err := pool.QueryRow(r.Context(), "SELECT id FROM sessions WHERE airport='EKCH' AND name='LIVE'").Scan(&session); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				strips, err := postgres.NewStripRepository(pool).List(r.Context(), session)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				writeJSON(w, strips)
			})
		},
		connect: func(ctx context.Context, endpoint string) error {
			// First wait for the elected master's full sync, then connect the second
			// controller. A full sync intentionally removes controllers absent from it.
			primary, err := connectFixtureController(ctx, endpoint, services.TestToken,
				&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.105", Callsign: "EKCH_A_TWR", Range: 300}, stripSnapshot(), nil)
			if err != nil {
				return err
			}
			clients = append(clients, primary)
			deadline := time.NewTimer(15 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				var count int
				err := database.QueryRow(ctx, "SELECT count(*) FROM strips WHERE callsign IN ('SAS301','SAS302','SAS303','SAS404','SAS501','SAS502') AND euroscope_seen_at IS NOT NULL").Scan(&count)
				if err != nil {
					return err
				}
				if count == 6 {
					break
				}
				select {
				case <-tick.C:
				case <-deadline.C:
					return fmt.Errorf("master full sync did not persist six strips")
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			secondary, err := connectFixtureController(ctx, endpoint, services.TestFrontendToken,
				&es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Position: "118.580", Callsign: "EKCH_GW_TWR", Range: 300}, nil, nil)
			if err != nil {
				return err
			}
			clients = append(clients, secondary)
			return nil
		},
	}
}

func stripSnapshot() *es.SyncEvent {
	snapshot := &es.SyncEvent{Controllers: []*es.Controller{{Position: "118.105", Callsign: "EKCH_A_TWR"}}, Runways: []*es.Runway{{Name: "22R", Departure: true}}}
	for index, callsign := range []string{"SAS301", "SAS302", "SAS303", "SAS404", "SAS501", "SAS502"} {
		state := es.GroundStateTaxi
		if index >= 4 {
			state = es.GroundStateDepart
		}
		squawk := fmt.Sprintf("%04d", 2000+index)
		snapshot.Strips = append(snapshot.Strips, &es.Strip{Callsign: callsign, Origin: "EKCH", Destination: "ESSA", Route: "DCT TESPI", AircraftType: "A320", AircraftCategory: "M", FlightRules: "I", HasFp: true, Cleared: true, GroundState: state, Squawk: squawk, AssignedSquawk: squawk, Runway: "22R", ClearedAltitude: 5000, RequestedAltitude: 25000, Position: &es.Position{Lat: 55.6181, Lon: 12.656, Altitude: 17}})
	}
	return snapshot
}
