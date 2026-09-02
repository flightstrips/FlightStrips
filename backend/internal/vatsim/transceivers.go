package vatsim

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

const defaultTransceiversURL = "https://data.vatsim.net/v3/transceivers-data.json"

type TransceiverCache struct {
	client          *http.Client
	transceiversURL string
	refreshInterval time.Duration
	onUpdate        func(context.Context) error

	mu                    sync.RWMutex
	frequenciesByCallsign map[string][]string
	refreshPending        bool
	lastUpdated           time.Time
}

type transceiverDataClient struct {
	Callsign     string                   `json:"callsign"`
	Frequency    int64                    `json:"frequency,omitempty"`
	Transceivers []transceiverDataStation `json:"transceivers,omitempty"`
}

type transceiverDataStation struct {
	Frequency int64 `json:"frequency"`
}

type transceiverDataPayload struct {
	Transceivers []transceiverDataClient `json:"transceivers"`
}

func NewTransceiverCache(transceiversURL string, refreshInterval time.Duration, client *http.Client, onUpdate func(context.Context) error) *TransceiverCache {
	if strings.TrimSpace(transceiversURL) == "" {
		transceiversURL = defaultTransceiversURL
	}
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}

	return &TransceiverCache{
		client:                client,
		transceiversURL:       transceiversURL,
		refreshInterval:       refreshInterval,
		onUpdate:              onUpdate,
		frequenciesByCallsign: make(map[string][]string),
	}
}

func (c *TransceiverCache) Start(ctx context.Context) {
	if err := c.refresh(ctx); err != nil {
		slog.Warn("transceiver cache: initial refresh failed", slog.Any("error", err))
	}

	ticker := time.NewTicker(c.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.refresh(ctx); err != nil {
				slog.Warn("transceiver cache: refresh failed", slog.Any("error", err))
			}
		}
	}
}

func (c *TransceiverCache) GetFrequencies(callsign string) []string {
	normalizedCallsign := normalizeCallsign(callsign)
	if normalizedCallsign == "" {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return slices.Clone(c.frequenciesByCallsign[normalizedCallsign])
}

func (c *TransceiverCache) refresh(ctx context.Context) error {
	snapshot, err := c.fetch(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	changed := !transceiverSnapshotsEqual(c.frequenciesByCallsign, snapshot)
	shouldNotify := c.onUpdate != nil && (changed || c.refreshPending)
	c.frequenciesByCallsign = snapshot
	c.lastUpdated = time.Now().UTC()
	c.mu.Unlock()

	if !shouldNotify {
		return nil
	}

	if err := c.onUpdate(ctx); err != nil {
		c.mu.Lock()
		c.refreshPending = true
		c.mu.Unlock()
		return fmt.Errorf("transceiver cache update callback: %w", err)
	}

	c.mu.Lock()
	c.refreshPending = false
	c.mu.Unlock()

	return nil
}

func (c *TransceiverCache) fetch(ctx context.Context) (map[string][]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.transceiversURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create transceiver request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch transceivers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch transceivers: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read transceivers: %w", err)
	}

	clients, err := decodeTransceiverData(body)
	if err != nil {
		return nil, fmt.Errorf("decode transceivers: %w", err)
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("transceiver payload contained no clients")
	}

	frequenciesByCallsign := make(map[string]map[string]struct{})
	validFrequencyCount := 0
	for _, client := range clients {
		callsign := normalizeCallsign(client.Callsign)
		if callsign == "" {
			continue
		}

		frequencies := make([]int64, 0, len(client.Transceivers)+1)
		if client.Frequency != 0 {
			frequencies = append(frequencies, client.Frequency)
		}
		for _, transceiver := range client.Transceivers {
			frequencies = append(frequencies, transceiver.Frequency)
		}

		for _, rawFrequency := range frequencies {
			frequency := NormalizeFrequency(fmt.Sprintf("%d", rawFrequency))
			if frequency == "" {
				continue
			}

			if _, ok := frequenciesByCallsign[callsign]; !ok {
				frequenciesByCallsign[callsign] = make(map[string]struct{})
			}
			frequenciesByCallsign[callsign][frequency] = struct{}{}
			validFrequencyCount++
		}
	}

	if validFrequencyCount == 0 {
		return nil, fmt.Errorf("payload contained %d clients but no valid frequencies", len(clients))
	}

	snapshot := make(map[string][]string, len(frequenciesByCallsign))
	for callsign, frequencies := range frequenciesByCallsign {
		normalizedFrequencies := make([]string, 0, len(frequencies))
		for frequency := range frequencies {
			normalizedFrequencies = append(normalizedFrequencies, frequency)
		}
		slices.Sort(normalizedFrequencies)
		snapshot[callsign] = normalizedFrequencies
	}

	return snapshot, nil
}

func decodeTransceiverData(body []byte) ([]transceiverDataClient, error) {
	var clients []transceiverDataClient
	if err := json.Unmarshal(body, &clients); err == nil {
		return clients, nil
	}

	var payload transceiverDataPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Transceivers == nil {
		return nil, fmt.Errorf("unsupported payload shape")
	}

	return payload.Transceivers, nil
}

func transceiverSnapshotsEqual(current, next map[string][]string) bool {
	if len(current) != len(next) {
		return false
	}

	for callsign, frequencies := range current {
		other, ok := next[callsign]
		if !ok || !slices.Equal(frequencies, other) {
			return false
		}
	}

	return true
}
