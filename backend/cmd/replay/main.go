package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"FlightStrips/internal/testing/replay"
)

func main() {
	// Parse command line flags
	var (
		sessionFile    = flag.String("session", "", "Path to recorded session JSON file (required)")
		mode           = flag.String("mode", "time", "Replay mode: 'time' for time-based or 'fast' for fast replay")
		speed          = flag.Float64("speed", 1.0, "Speed multiplier for time-based mode (e.g., 10.0 = 10x speed)")
		serverURL      = flag.String("server", "ws://localhost:2994/euroscopeEvents", "WebSocket server URL")
		minDelay       = flag.Int("min-delay", 10, "Minimum delay between events in fast mode (milliseconds)")
		stopOnError    = flag.Bool("stop-on-error", true, "Stop replay on first error")
		verbose        = flag.Bool("verbose", false, "Enable verbose logging")
		listRecordings = flag.Bool("list", false, "List available recorded sessions")
		recordingsPath = flag.String("recordings-path", "recordings", "Path to recordings directory")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "FlightStrips Replay Tool\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Replay at real-time speed:\n")
		fmt.Fprintf(os.Stderr, "  %s -session recordings/EKCH_LIVE_20260214_123456.json\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Replay at 10x speed:\n")
		fmt.Fprintf(os.Stderr, "  %s -session recordings/EKCH_LIVE_20260214_123456.json -speed 10.0\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # Fast replay (minimal delays):\n")
		fmt.Fprintf(os.Stderr, "  %s -session recordings/EKCH_LIVE_20260214_123456.json -mode fast\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # List available recordings:\n")
		fmt.Fprintf(os.Stderr, "  %s -list\n\n", os.Args[0])
	}

	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Handle -list flag
	if *listRecordings {
		if err := listRecordedSessions(*recordingsPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error listing recordings: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Validate required flags
	if *sessionFile == "" {
		fmt.Fprintf(os.Stderr, "Error: -session flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Validate mode
	var replayMode replay.ReplayMode
	switch *mode {
	case "time":
		replayMode = replay.ModeTimeBased
	case "fast":
		replayMode = replay.ModeFast
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid mode '%s' (must be 'time' or 'fast')\n", *mode)
		os.Exit(1)
	}

	// Create config
	config := replay.Config{
		SessionFile:     *sessionFile,
		Mode:            replayMode,
		SpeedMultiplier: *speed,
		ServerURL:       *serverURL,
		MinEventDelay:   time.Duration(*minDelay) * time.Millisecond,
		StopOnError:     *stopOnError,
		Verbose:         *verbose,
	}

	// Create replayer
	replayer, err := replay.NewReplayerWithoutAssertions(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating replayer: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		slog.Info("Received interrupt signal, stopping replay...")
		cancel()
	}()

	// Run replay
	if err := replayer.Replay(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Replay failed: %v\n", err)
		os.Exit(1)
	}

	// Print final stats
	stats := replayer.GetStats()
	if stats.EventsFailed > 0 {
		os.Exit(1)
	}
}

func listRecordedSessions(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read recordings directory: %w", err)
	}

	fmt.Printf("Recorded sessions in %s:\n\n", path)

	found := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if len(entry.Name()) > 5 && entry.Name()[len(entry.Name())-5:] == ".json" {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			fmt.Printf("  %s\n", entry.Name())
			fmt.Printf("    Size: %d bytes\n", info.Size())
			fmt.Printf("    Modified: %s\n\n", info.ModTime().Format("2006-01-02 15:04:05"))
			found = true
		}
	}

	if !found {
		fmt.Println("  No recorded sessions found.")
	}

	return nil
}
