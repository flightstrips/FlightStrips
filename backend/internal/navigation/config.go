// Package navigation configures the independently deployable navigation-data
// source used by flight-facing features and AMAN.
package navigation

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	SourceAIRACNet              = "airacnet"
	SourceVIFF                  = "viff"
	SourceSector                = "sector"
	DefaultTerminalGeometryPath = "config/aman/ekch-terminal-2611.json"
)

// Config identifies the navigation provider and the airport terminal
// configuration it materializes. Its zero value deliberately disables the
// navigation source without affecting AMAN's rollout setting.
type Config struct {
	// Now is shared with the application replay clock; nil uses real time.
	Now                  func() time.Time
	Source               string
	TerminalGeometryPath string
	// SectorFilePath enables local SCT fallback for an online source, or is the
	// required input when Source is sector. The user's file is never bundled.
	SectorFilePath string
	// vIFF uses one shared backend key (optional for public arrivals), never a
	// controller's subscription. Overrides also support isolated replay tests.
	VIFFBaseURL string
	VIFFAPIKey  string
	HTTPClient  *http.Client
}

func (c Config) Normalize() Config {
	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	c.TerminalGeometryPath = strings.TrimSpace(c.TerminalGeometryPath)
	c.SectorFilePath = strings.TrimSpace(c.SectorFilePath)
	if c.Source != "" && c.TerminalGeometryPath == "" {
		c.TerminalGeometryPath = DefaultTerminalGeometryPath
	}
	return c
}

func (c Config) Enabled() bool { return c.Normalize().Source != "" }

func (c Config) Validate() error {
	c = c.Normalize()
	if c.Source == "" {
		if c.SectorFilePath != "" {
			return fmt.Errorf("navigation sector file requires a navigation source")
		}
		if c.TerminalGeometryPath != "" {
			return fmt.Errorf("navigation terminal geometry path requires a navigation source")
		}
		return nil
	}
	if c.Source != SourceAIRACNet && c.Source != SourceVIFF && c.Source != SourceSector {
		return fmt.Errorf("navigation source %q is unsupported", c.Source)
	}
	if c.Source == SourceSector && c.SectorFilePath == "" {
		return fmt.Errorf("sector navigation source requires NAVIGATION_SECTOR_FILE")
	}
	if c.TerminalGeometryPath == "" {
		return fmt.Errorf("navigation terminal geometry path is required when navigation is enabled")
	}
	return nil
}
