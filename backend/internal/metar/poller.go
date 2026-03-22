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
	"time"
)

// FrontendHub is the minimal interface required by the poller to send ATIS updates.
type FrontendHub interface {
	SendAtisUpdate(session int32, metar string, arrAtisCode string, depAtisCode string)
}

// afvAtisEntry holds the fields we need from a single AFV ATIS data entry.
type afvAtisEntry struct {
	Callsign string `json:"callsign"`
	AtisCode string `json:"atis_code"`
}

// atisInfo holds arrival and departure ATIS codes for an airport.
type atisInfo struct {
	arr string
	dep string
}

// Poller fetches METAR data for all active sessions and pushes it to connected frontend clients.
type Poller struct {
	sessionRepo  repository.SessionRepository
	hub          FrontendHub
	interval     time.Duration
	httpClient   *http.Client
	metarBaseURL string
	atisDataURL  string
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
		p.hub.SendAtisUpdate(session.ID, metar, info.arr, info.dep)
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
		switch kind {
		case "arr":
			info.arr = e.AtisCode
		case "dep":
			info.dep = e.AtisCode
		default: // general: fill whichever slots are still empty
			if info.arr == "" {
				info.arr = e.AtisCode
			}
			if info.dep == "" {
				info.dep = e.AtisCode
			}
		}
		result[icao] = info
	}

	return result, nil
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
