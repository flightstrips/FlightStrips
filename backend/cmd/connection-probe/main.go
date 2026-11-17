// connection-probe needs no operational token or database credentials. It checks
// HTTP liveness and WebSocket upgrade only, never claims application readiness.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"FlightStrips/internal/connectionprobe"
	"FlightStrips/internal/envconfig"
	"FlightStrips/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var buildVersion = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("Connection probe stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	probe, err := connectionprobe.New(os.Getenv("CONNECTION_PROBE_PUBLIC_URL"), os.Getenv("CONNECTION_PROBE_ORIGIN_ADDRESS"), 5*time.Second)
	if err != nil {
		return err
	}
	if err := envconfig.ApplyFileOverrides("OTEL_EXPORTER_OTLP_HEADERS"); err != nil {
		return err
	}
	tel, err := telemetry.Initialize(ctx, telemetry.Config{ServiceName: "flightstrips-connection-probe", ServiceVersion: buildVersion, Environment: os.Getenv("ENVIRONMENT")})
	if err != nil {
		return err
	}
	telemetry.SetupDualLogger(slog.Default().Handler())
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tel.Shutdown(shutdown); err != nil {
			slog.Warn("Telemetry shutdown failed", "error", err)
		}
	}()
	meter := otel.Meter("FlightStrips/connection-probe")
	success, err := meter.Int64Gauge("connection.probe.success", metric.WithDescription("HTTP or upgrade succeeded; not authenticated application readiness"))
	if err != nil {
		return err
	}
	checked, err := meter.Int64Gauge("connection.probe.checked_at", metric.WithUnit("s"), metric.WithDescription("Unix timestamp of the most recent completed check"))
	if err != nil {
		return err
	}
	duration, err := meter.Float64Histogram("connection.probe.duration", metric.WithUnit("s"))
	if err != nil {
		return err
	}
	previous := map[string]bool{}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		results := make(chan connectionprobe.Result, 4)
		var workers sync.WaitGroup
		for _, target := range []string{"public", "origin"} {
			for _, check := range []string{"http", "websocket"} {
				workers.Add(1)
				go func() { defer workers.Done(); results <- probe.Check(ctx, target, check) }()
			}
		}
		workers.Wait()
		close(results)
		for result := range results {
			attrs := metric.WithAttributes(attribute.String("target", result.Target), attribute.String("check", result.Check))
			value := int64(0)
			if result.Success {
				value = 1
			}
			success.Record(ctx, value, attrs)
			checked.Record(ctx, time.Now().Unix(), attrs)
			duration.Record(ctx, result.Duration.Seconds(), attrs)
			key := result.Target + "/" + result.Check
			before, seen := previous[key]
			if !seen || before != result.Success {
				slog.Info("Connection probe changed", "target", result.Target, "check", result.Check, "success", result.Success,
					"status", result.Status, "duration_ms", result.Duration.Milliseconds(), "cf_ray", result.CFRay,
					"connection_id", result.ConnectionID, "error", result.Error)
			}
			previous[key] = result.Success
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
