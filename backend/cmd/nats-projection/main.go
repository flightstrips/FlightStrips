// Command nats-projection is an isolated test runtime for the preparatory
// NATS projection. The production PostgreSQL application never starts it.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
)

func main() {
	cfg, err := natsresources.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	nc, err := natsresources.Connect(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer nc.Close()
	p, err := cluster.NewProjection(nc, cfg)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		if err := p.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("projection stopped: %v", err)
		}
	}()
	listen := os.Getenv("NATS_PROJECTION_LISTEN")
	if listen == "" {
		listen = "127.0.0.1:8091"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", p.Readyz)
	server := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
