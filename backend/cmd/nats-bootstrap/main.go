package main

import (
	"FlightStrips/internal/natsresources"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := natsresources.ConfigFromEnv()
	if err != nil {
		log.Fatalf("invalid resource configuration (%T)", err)
	}
	timeout := time.Minute
	if value := os.Getenv("NATS_BOOTSTRAP_TIMEOUT"); value != "" {
		timeout, err = time.ParseDuration(value)
		if err != nil || timeout <= 0 {
			log.Fatal("NATS_BOOTSTRAP_TIMEOUT must be a positive duration")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := bootstrap(ctx, cfg); err != nil {
		log.Fatalf("NATS administrator bootstrap failed (%T)", err)
	}
	log.Print("NATS resources verified")
}

// Compose starts brokers concurrently. Retry the idempotent administrator
// operation until quorum forms; existing resource drift is never repaired.
func bootstrap(ctx context.Context, cfg natsresources.Config) error {
	for {
		nc, err := natsresources.Connect(cfg)
		if err == nil {
			err = natsresources.Bootstrap(ctx, nc, cfg)
			nc.Close()
		}
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("bootstrap deadline: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}
