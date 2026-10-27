// Command browserharness runs the production application against an isolated
// migrated PostgreSQL database. Suites replace external feeds and identity only.
// It is outside cmd/server and never included in deployed binaries.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"FlightStrips/internal/app"
	"FlightStrips/internal/config"
	"FlightStrips/internal/pdc/testdata"

	"github.com/jackc/pgx/v5/pgxpool"
)

type suite struct {
	config       app.Config
	dependencies app.Dependencies
	register     func(*http.ServeMux, *pgxpool.Pool, string)
	connect      func(context.Context, string) error
	close        func()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	name := flag.String("suite", "strips", "fixture suite: strips or aman")
	address := flag.String("address", "127.0.0.1:4180", "local listen address")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Setenv("TEST_MODE", "true")
	os.Setenv("GRPLUGIN_ICAO_AIRCRAFT_JSON", "config/test/ICAO_Aircraft.json")
	if err := config.InitConfig(); err != nil {
		return err
	}
	var selected suite
	switch *name {
	case "aman":
		selected = newAMANSuite()
	case "strips":
		selected = newStripsSuite()
	default:
		return fmt.Errorf("unknown E2E suite %q", *name)
	}
	defer selected.close()
	pool, _, cleanup, err := testdata.OpenTestDB(ctx)
	if err != nil {
		return err
	}
	defer testdata.ShutdownTestDB()
	defer cleanup()
	defer pool.Close()
	// Reference data only. Operational state enters through application protocols.
	if _, err := pool.Exec(ctx, "INSERT INTO airports (name) VALUES ('EKCH') ON CONFLICT DO NOTHING"); err != nil {
		return err
	}
	selected.dependencies.DBPool = pool
	selected.dependencies.AuthenticationService = browserIdentity{}
	application, err := app.Build(ctx, selected.config, selected.dependencies)
	if err != nil {
		return err
	}
	defer application.Close(context.Background())
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/e2e/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		w.WriteHeader(204)
		go stop()
	})
	selected.register(mux, pool, "ws://"+listener.Addr().String()+"/euroscopeEvents")
	mux.Handle("/", application.Handler())
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("server: %v", err)
			stop()
		}
	}()
	application.StartWorkers(ctx)
	if err := selected.connect(ctx, "ws://"+listener.Addr().String()+"/euroscopeEvents"); err != nil {
		return err
	}
	// Advertise readiness only after protocol fixture clients are connected.
	mux.HandleFunc("/e2e/ready", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ready": true, "suite": *name})
	})
	log.Printf("Browser E2E suite %s ready at http://%s", *name, listener.Addr())
	<-ctx.Done()
	return nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}
