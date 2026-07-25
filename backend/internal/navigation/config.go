// Package navigation configures the independently deployable navigation-data
// source used by flight-facing features and AMAN.
package navigation

import (
	"fmt"
	"strings"
)

const SourceAIRACNet = "airacnet"

// Config identifies the navigation provider and the airport terminal
// configuration it materializes. Its zero value deliberately disables the
// navigation source without affecting AMAN's rollout setting.
type Config struct {
	Source               string
	TerminalGeometryPath string
}

func (c Config) Normalize() Config {
	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	c.TerminalGeometryPath = strings.TrimSpace(c.TerminalGeometryPath)
	return c
}

func (c Config) Enabled() bool { return c.Normalize().Source != "" }

func (c Config) Validate() error {
	c = c.Normalize()
	if c.Source == "" {
		if c.TerminalGeometryPath != "" {
			return fmt.Errorf("navigation terminal geometry path requires a navigation source")
		}
		return nil
	}
	if c.Source != SourceAIRACNet {
		return fmt.Errorf("navigation source %q is unsupported", c.Source)
	}
	if c.TerminalGeometryPath == "" {
		return fmt.Errorf("navigation terminal geometry path is required when navigation is enabled")
	}
	return nil
}
