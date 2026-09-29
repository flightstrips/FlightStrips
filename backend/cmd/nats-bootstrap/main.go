package main

import (
	"FlightStrips/internal/natsresources"
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := natsresources.Bootstrap(ctx, nc, cfg); err != nil {
		log.Fatal(err)
	}
	log.Print("NATS resources verified")
}
