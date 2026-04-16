package vatsim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultStatusURL       = "https://status.vatsim.net/status.json"
	defaultRefreshInterval = 30 * time.Second
	defaultHTTPTimeout     = 10 * time.Second
)

type Pilot struct {
	CID      string
	Callsign string
}

type Cache struct {
	client          *http.Client
	statusURL       string
	refreshInterval time.Duration

	mu               sync.RWMutex
	pilotsByCallsign map[string]Pilot
	pilotsByCID      map[string]Pilot
	dataURL          string
	lastUpdated      time.Time
}

type statusResponse struct {
	Data struct {
		V3 []string `json:"v3"`
	} `json:"data"`
}

type networkDataResponse struct {
	Pilots []struct {
		CID      int    `json:"cid"`
		Callsign string `json:"callsign"`
	} `json:"pilots"`
}

func NewCache(statusURL string, refreshInterval time.Duration, client *http.Client) *Cache {
	if strings.TrimSpace(statusURL) == "" {
		statusURL = defaultStatusURL
	}
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &Cache{
		client:           client,
		statusURL:        statusURL,
		refreshInterval:  refreshInterval,
		pilotsByCallsign: make(map[string]Pilot),
		pilotsByCID:      make(map[string]Pilot),
	}
}

func (c *Cache) Start(ctx context.Context) {
	_ = c.refresh(ctx)

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.refresh(ctx)
		}
	}
}

func (c *Cache) VerifyPilotOwnsCallsign(ctx context.Context, cid string, callsign string) (bool, error) {
	normalizedCID := strings.TrimSpace(cid)
	normalizedCallsign := normalizeCallsign(callsign)
	if normalizedCID == "" || normalizedCallsign == "" {
		return false, nil
	}

	if pilot, ok := c.getPilot(normalizedCallsign); ok {
		return pilot.CID == normalizedCID, nil
	}

	if err := c.refresh(ctx); err != nil {
		return false, err
	}

	pilot, ok := c.getPilot(normalizedCallsign)
	if !ok {
		return false, nil
	}
	return pilot.CID == normalizedCID, nil
}

func (c *Cache) GetCallsignByCID(ctx context.Context, cid string) (string, bool, error) {
	normalizedCID := strings.TrimSpace(cid)
	if normalizedCID == "" {
		return "", false, nil
	}

	if pilot, ok := c.getPilotByCID(normalizedCID); ok {
		return pilot.Callsign, true, nil
	}

	if err := c.refresh(ctx); err != nil {
		return "", false, err
	}

	pilot, ok := c.getPilotByCID(normalizedCID)
	if !ok {
		return "", false, nil
	}
	return pilot.Callsign, true, nil
}

func (c *Cache) refresh(ctx context.Context) error {
	dataURL, err := c.resolveDataURL(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dataURL, nil)
	if err != nil {
		return fmt.Errorf("create vatsim data request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch vatsim network data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch vatsim network data: unexpected status %d", resp.StatusCode)
	}

	var payload networkDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode vatsim network data: %w", err)
	}

	pilots := make(map[string]Pilot, len(payload.Pilots))
	pilotsByCID := make(map[string]Pilot, len(payload.Pilots))
	for _, pilot := range payload.Pilots {
		callsign := normalizeCallsign(pilot.Callsign)
		if callsign == "" {
			continue
		}

		entry := Pilot{
			CID:      fmt.Sprintf("%d", pilot.CID),
			Callsign: callsign,
		}
		pilots[callsign] = entry
		pilotsByCID[entry.CID] = entry
	}

	c.mu.Lock()
	c.pilotsByCallsign = pilots
	c.pilotsByCID = pilotsByCID
	c.dataURL = dataURL
	c.lastUpdated = time.Now().UTC()
	c.mu.Unlock()

	return nil
}

func (c *Cache) resolveDataURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.statusURL, nil)
	if err != nil {
		return "", fmt.Errorf("create vatsim status request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch vatsim status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch vatsim status: unexpected status %d", resp.StatusCode)
	}

	var payload statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode vatsim status: %w", err)
	}

	if len(payload.Data.V3) == 0 || strings.TrimSpace(payload.Data.V3[0]) == "" {
		return "", fmt.Errorf("vatsim status did not include a v3 data feed URL")
	}

	return payload.Data.V3[0], nil
}

func (c *Cache) getPilot(callsign string) (Pilot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pilot, ok := c.pilotsByCallsign[callsign]
	return pilot, ok
}

func (c *Cache) getPilotByCID(cid string) (Pilot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	pilot, ok := c.pilotsByCID[cid]
	return pilot, ok
}

func normalizeCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}
