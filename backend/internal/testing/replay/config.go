package replay

import (
	"time"
)

// ReplayMode defines how the replay timing works
type ReplayMode string

const (
	// ModeFast replays events as quickly as possible with minimal delay
	ModeFast ReplayMode = "fast"
	// ModeTimeBased replays events with realistic timing based on speed multiplier
	ModeTimeBased ReplayMode = "time"
)

// Config holds configuration for replay
type Config struct {
	// SessionFile is the path to the recorded session JSON file
	SessionFile string

	// Mode determines replay timing behavior (fast or time-based)
	Mode ReplayMode

	// SpeedMultiplier is used in time-based mode (1.0 = real-time, 10.0 = 10x speed)
	// Ignored in fast mode
	SpeedMultiplier float64

	// ServerURL is the WebSocket endpoint to connect to
	ServerURL string

	// MinEventDelay is the minimum delay between events in fast mode (milliseconds)
	MinEventDelay time.Duration

	// StopOnError determines if replay should stop on first error or continue
	StopOnError bool

	// Verbose enables detailed logging
	Verbose bool
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() Config {
	return Config{
		Mode:            ModeTimeBased,
		SpeedMultiplier: 1.0,
		ServerURL:       "ws://localhost:2994/euroscopeEvents",
		MinEventDelay:   10 * time.Millisecond,
		StopOnError:     true,
		Verbose:         false,
	}
}

// Validate checks if the config is valid
func (c *Config) Validate() error {
	if c.SessionFile == "" {
		return ErrInvalidConfig("session file is required")
	}

	if c.Mode != ModeFast && c.Mode != ModeTimeBased {
		return ErrInvalidConfig("mode must be 'fast' or 'time'")
	}

	if c.Mode == ModeTimeBased && c.SpeedMultiplier <= 0 {
		return ErrInvalidConfig("speed multiplier must be positive")
	}

	if c.ServerURL == "" {
		return ErrInvalidConfig("server URL is required")
	}

	return nil
}
