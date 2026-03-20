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
	SendAtisUpdate(session int32, metar string, atisCode string)
}

// vatsimAtisStation holds the fields we need from a VATSIM ATIS station entry.
type vatsimAtisStation struct {
	AtisCode string `json:"atis_code"`
}

// Poller fetches METAR data for all active sessions and pushes it to connected frontend clients.
type Poller struct {
	sessionRepo  repository.SessionRepository
	hub          FrontendHub
	interval     time.Duration
	httpClient   *http.Client
	metarBaseURL string
	atisBaseURL  string
}

// NewPoller creates a new METAR poller.
func NewPoller(sessionRepo repository.SessionRepository, hub FrontendHub) *Poller {
	return &Poller{
		sessionRepo:  sessionRepo,
		hub:          hub,
		interval:     2 * time.Minute,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		metarBaseURL: "https://metar.vatsim.net",
		atisBaseURL:  "https://api.vatsim.net/v2/atis",
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

		atisCode, err := p.fetchAtisCode(ctx, airport)
		if err != nil {
			slog.Warn("metar poller: failed to fetch ATIS code",
				slog.String("airport", airport),
				slog.Any("error", err),
			)
			atisCode = ""
		}

		p.hub.SendAtisUpdate(session.ID, metar, atisCode)
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

// fetchAtisCode queries the VATSIM API for the current ATIS code letter for the given ICAO.
// Returns an empty string when no ATIS station is online for the airport.
func (p *Poller) fetchAtisCode(ctx context.Context, icao string) (string, error) {
	url := fmt.Sprintf("%s/%s", p.atisBaseURL, icao)
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

	var stations []vatsimAtisStation
	if err := json.Unmarshal(body, &stations); err != nil {
		return "", fmt.Errorf("parse ATIS response: %w", err)
	}

	if len(stations) == 0 {
		return "", nil
	}

	return stations[0].AtisCode, nil
}
