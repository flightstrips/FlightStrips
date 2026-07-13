package metar

import (
	"FlightStrips/internal/repository"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// FrontendHub is the minimal interface required by the poller to send ATIS updates.
type FrontendHub interface {
	SendAtisUpdate(session int32, metar string, arrAtisCode string, depAtisCode string)
}

// afvAtisEntry holds the fields we need from a single AFV ATIS data entry.
type afvAtisEntry struct {
	Callsign    string   `json:"callsign"`
	Frequency   string   `json:"frequency"`
	AtisCode    string   `json:"atis_code"`
	TextAtis    []string `json:"text_atis"`
	LastUpdated string   `json:"last_updated"`
}

// ATIS is the current VATSIM ATIS information exposed to pilot-facing APIs.
type ATIS struct {
	Callsign    string    `json:"callsign"`
	Code        string    `json:"code"`
	Frequency   string    `json:"frequency"`
	Text        []string  `json:"text"`
	LastUpdated time.Time `json:"last_updated"`
	Stale       bool      `json:"stale"`
}

type atisInfo struct {
	arr *ATIS
	dep *ATIS
}

// Poller fetches METAR data for all active sessions and pushes it to connected frontend clients.
type Poller struct {
	sessionRepo   repository.SessionRepository
	hub           FrontendHub
	interval      time.Duration
	httpClient    *http.Client
	metarBaseURL  string
	atisDataURL   string
	atisMu        sync.RWMutex
	atisCache     map[string]atisInfo
	atisFetchedAt time.Time
}

// NewPoller creates a new METAR poller.
func NewPoller(sessionRepo repository.SessionRepository, hub FrontendHub) *Poller {
	return &Poller{
		sessionRepo:  sessionRepo,
		hub:          hub,
		interval:     2 * time.Minute,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		metarBaseURL: "https://metar.vatsim.net",
		atisDataURL:  "https://data.vatsim.net/v3/afv-atis-data.json",
		atisCache:    make(map[string]atisInfo),
	}
}

// Start runs the polling loop. It fetches immediately, then repeats every interval.
// The loop exits when ctx is cancelled.
func (p *Poller) Start(ctx context.Context) {
	p.poll(ctx)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *Poller) poll(ctx context.Context) {
	sessions, err := p.sessionRepo.List(ctx)
	if err != nil {
		slog.Error("metar poller: failed to list sessions", slog.Any("error", err))
		return
	}

	atisMap, err := p.fetchAllAtisData(ctx)
	if err != nil {
		slog.Warn("metar poller: failed to fetch ATIS data", slog.Any("error", err))
		atisMap = map[string]atisInfo{}
	} else {
		p.atisMu.Lock()
		p.atisCache = atisMap
		p.atisFetchedAt = time.Now().UTC()
		p.atisMu.Unlock()
	}

	for _, session := range sessions {
		airport := session.Airport
		metar, err := p.fetch(ctx, airport)
		if err != nil {
			slog.Warn("metar poller: failed to fetch METAR",
				slog.String("airport", airport),
				slog.Any("error", err),
			)
			continue
		}

		info := atisMap[strings.ToUpper(airport)]
		p.hub.SendAtisUpdate(session.ID, metar, atisCode(info.arr), atisCode(info.dep))
	}
}

func (p *Poller) fetch(ctx context.Context, icao string) (string, error) {
	url := fmt.Sprintf("%s/%s", p.metarBaseURL, icao)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(body)), nil
}

// fetchAllAtisData fetches the full VATSIM AFV ATIS feed and returns a map of
// uppercase ICAO → atisInfo with arrival and departure codes.
// Callsign formats: KJFK_ATIS (general), KJFK_A_ATIS (arrival), KJFK_D_ATIS (departure).
// General ATIS is used for both arr and dep when no specific codes are present.
func (p *Poller) fetchAllAtisData(ctx context.Context) (map[string]atisInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.atisDataURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ATIS feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var entries []afvAtisEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse ATIS feed: %w", err)
	}

	result := make(map[string]atisInfo)
	for _, e := range entries {
		icao, kind := parseAtisCallsign(e.Callsign)
		if icao == "" {
			continue
		}
		info := result[icao]
		updated, _ := time.Parse(time.RFC3339Nano, e.LastUpdated)
		entry := &ATIS{Callsign: e.Callsign, Code: e.AtisCode, Frequency: e.Frequency, Text: e.TextAtis, LastUpdated: updated}
		switch kind {
		case "arr":
			info.arr = entry
		case "dep":
			info.dep = entry
		default: // general: fill whichever slots are still empty
			if info.arr == nil {
				info.arr = entry
			}
			if info.dep == nil {
				info.dep = entry
			}
		}
		result[icao] = info
	}

	return result, nil
}

func atisCode(info *ATIS) string {
	if info == nil {
		return ""
	}
	return info.Code
}

// GetATIS returns the latest cached arrival or departure ATIS for an airport.
func (p *Poller) GetATIS(airport string, departure bool) *ATIS {
	p.atisMu.RLock()
	defer p.atisMu.RUnlock()
	info := p.atisCache[strings.ToUpper(strings.TrimSpace(airport))]
	var result *ATIS
	if departure {
		result = cloneATIS(info.dep)
	} else {
		result = cloneATIS(info.arr)
	}
	if result != nil {
		result.Stale = p.atisFetchedAt.IsZero() || time.Since(p.atisFetchedAt) > p.atisStaleAfter()
	}
	return result
}

func (p *Poller) atisStaleAfter() time.Duration {
	threshold := 2 * p.interval
	if threshold < 5*time.Minute {
		return 5 * time.Minute
	}
	return threshold
}

func cloneATIS(info *ATIS) *ATIS {
	if info == nil {
		return nil
	}
	copy := *info
	copy.Text = append([]string(nil), info.Text...)
	return &copy
}

// parseAtisCallsign parses a VATSIM ATIS callsign and returns the ICAO and kind.
// Kind is "arr", "dep", or "general". Returns empty ICAO for non-ATIS callsigns.
func parseAtisCallsign(callsign string) (icao, kind string) {
	upper := strings.ToUpper(callsign)
	parts := strings.Split(upper, "_")
	n := len(parts)
	if n < 2 || parts[n-1] != "ATIS" {
		return "", ""
	}
	icao = parts[0]
	if n == 3 {
		switch parts[1] {
		case "A":
			kind = "arr"
		case "D":
			kind = "dep"
		default:
			kind = "general"
		}
	} else {
		kind = "general"
	}
	return icao, kind
}
