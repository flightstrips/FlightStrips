package cdm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ConfigProvider interface {
	ConfigForAirport(airport string) *CdmAirportConfig
	SetLvo(airport string, active bool)
	SetDelay(delay CdmDelay)
	ClearDelay(airport, runway string)
}

type CdmConfigDefaults struct {
	Rate        int
	RateLvo     int
	TaxiMinutes int
}

type CdmConfigStore struct {
	mu               sync.RWMutex
	configs          map[string]*CdmAirportConfig
	restrictionRates map[string]int // ICAO → overriding departure rate from vIFF API
	rateURL          string
	sidIntervalURL   string
	taxiZoneURL      string
	refreshInterval  time.Duration
	defaults         CdmConfigDefaults
	httpClient       *http.Client
	cdmClient        *Client
}

func NewCdmConfigStore(rateURL, sidIntervalURL, taxiZoneURL string, refreshInterval time.Duration, defaults CdmConfigDefaults, httpClient *http.Client) *CdmConfigStore {
	if refreshInterval <= 0 {
		refreshInterval = time.Minute
	}
	if defaults.Rate <= 0 {
		defaults.Rate = DefaultCDMRate
	}
	if defaults.RateLvo <= 0 {
		defaults.RateLvo = DefaultCDMRateLVO
	}
	if defaults.TaxiMinutes <= 0 {
		defaults.TaxiMinutes = DefaultCDMTaxiMinutes
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &CdmConfigStore{
		configs:          make(map[string]*CdmAirportConfig),
		restrictionRates: make(map[string]int),
		rateURL:          strings.TrimSpace(rateURL),
		sidIntervalURL:   strings.TrimSpace(sidIntervalURL),
		taxiZoneURL:      strings.TrimSpace(taxiZoneURL),
		refreshInterval:  refreshInterval,
		defaults:         defaults,
		httpClient:       httpClient,
	}
}

func (s *CdmConfigStore) SetCdmClient(client *Client) {
	s.cdmClient = client
}

func (s *CdmConfigStore) Start(ctx context.Context) {
	slog.Info("CDM config refresh started",
		slog.String("rate_url", s.rateURL),
		slog.String("sid_interval_url", s.sidIntervalURL),
		slog.String("taxi_zone_url", s.taxiZoneURL),
		slog.Duration("refresh_interval", s.refreshInterval),
	)

	s.refresh(ctx)

	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh(ctx)
		}
	}
}

func (s *CdmConfigStore) ConfigForAirport(airport string) *CdmAirportConfig {
	key := strings.ToUpper(strings.TrimSpace(airport))
	s.mu.RLock()
	defer s.mu.RUnlock()

	config := s.configs[key]
	if config == nil {
		return nil
	}
	clone := config.Clone()
	if rate, ok := s.restrictionRates[key]; ok && rate > 0 {
		clone.DefaultRate = rate
		clone.DefaultRateLvo = rate
	}
	return clone
}

func (s *CdmConfigStore) DefaultConfigForAirport(airport string) *CdmAirportConfig {
	config := NewDefaultAirportConfig(airport)
	config.DefaultRate = s.defaults.Rate
	config.DefaultRateLvo = s.defaults.RateLvo
	config.DefaultTaxiMinutes = s.defaults.TaxiMinutes
	return config
}

func (s *CdmConfigStore) SetLvo(airport string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config := s.ensureConfigLocked(airport)
	config.LvoActive = active
}

func (s *CdmConfigStore) SetDelay(delay CdmDelay) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config := s.ensureConfigLocked(delay.Airport)
	filtered := make([]CdmDelay, 0, len(config.Delays)+1)
	for _, existing := range config.Delays {
		if strings.EqualFold(existing.Runway, delay.Runway) {
			continue
		}
		filtered = append(filtered, existing)
	}
	config.Delays = append(filtered, delay)
}

func (s *CdmConfigStore) ClearDelay(airport, runway string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config := s.ensureConfigLocked(airport)
	filtered := config.Delays[:0]
	for _, delay := range config.Delays {
		if strings.EqualFold(delay.Runway, runway) {
			continue
		}
		filtered = append(filtered, delay)
	}
	config.Delays = append([]CdmDelay(nil), filtered...)
}

func (s *CdmConfigStore) applyDepartureRestrictions(restrictions []DepartureRestriction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restrictionRates = make(map[string]int, len(restrictions))
	for _, r := range restrictions {
		key := strings.ToUpper(strings.TrimSpace(r.Airport))
		if key != "" && r.Rate > 0 {
			s.restrictionRates[key] = r.Rate
		}
	}
}

func (s *CdmConfigStore) refresh(ctx context.Context) {
	if s.cdmClient != nil && s.cdmClient.isValid {
		restrictions, err := s.cdmClient.GetDepartureRestrictions(ctx)
		if err != nil {
			slog.Warn("CDM departure restrictions refresh failed", slog.Any("error", err))
		} else {
			s.applyDepartureRestrictions(restrictions)
		}
	}

	if s.rateURL != "" {
		rates, err := s.fetchRates(ctx, s.rateURL)
		if err != nil {
			slog.Warn("CDM rate refresh failed", slog.Any("error", err))
		} else if len(rates) > 0 {
			s.mu.Lock()
			s.mergeRatesLocked(rates)
			s.mu.Unlock()
		}
	}

	if s.sidIntervalURL != "" {
		intervals, err := s.fetchSidIntervals(ctx, s.sidIntervalURL)
		if err != nil {
			slog.Warn("CDM sid interval refresh failed", slog.Any("error", err))
		} else if len(intervals) > 0 {
			s.mu.Lock()
			s.mergeSidIntervalsLocked(intervals)
			s.mu.Unlock()
		}
	}

	if s.taxiZoneURL != "" {
		zones, err := s.fetchTaxiZones(ctx, s.taxiZoneURL)
		if err != nil {
			slog.Warn("CDM taxi zone refresh failed", slog.Any("error", err))
		} else if len(zones) > 0 {
			s.mu.Lock()
			s.mergeTaxiZonesLocked(zones)
			s.mu.Unlock()
		}
	}
}

func (s *CdmConfigStore) fetchRates(ctx context.Context, url string) ([]CdmRate, error) {
	data, err := s.fetchBytes(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseRateData(data)
}

func (s *CdmConfigStore) fetchSidIntervals(ctx context.Context, url string) ([]CdmSidInterval, error) {
	data, err := s.fetchBytes(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseSidIntervalData(data)
}

func (s *CdmConfigStore) fetchTaxiZones(ctx context.Context, url string) ([]CdmTaxiZone, error) {
	data, err := s.fetchBytes(ctx, url)
	if err != nil {
		return nil, err
	}
	return parseTaxiZoneData(data)
}

func (s *CdmConfigStore) fetchBytes(ctx context.Context, uri string) ([]byte, error) {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
		if err != nil {
			return nil, err
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("unexpected status code %d for %s", resp.StatusCode, uri)
		}

		return io.ReadAll(resp.Body)
	}

	return os.ReadFile(uri)
}

// SeedAirportConfig pre-populates airport defaults from the YAML config before the first
// external refresh. This ensures rate, LVO rate, and deice config are available immediately
// at startup even before any external URIs are fetched.
func (s *CdmConfigStore) SeedAirportConfig(airport string, rate, rateLvo int, deice CdmDeiceConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config := s.ensureConfigLocked(airport)
	if rate > 0 {
		config.DefaultRate = rate
	}
	if rateLvo > 0 {
		config.DefaultRateLvo = rateLvo
	}
	config.DeiceConfig = deice
}

func (s *CdmConfigStore) ensureConfigLocked(airport string) *CdmAirportConfig {
	key := normalizeToken(airport)
	config := s.configs[key]
	if config == nil {
		config = s.DefaultConfigForAirport(key)
		s.configs[key] = config
	}
	return config
}

func (s *CdmConfigStore) mergeRatesLocked(rates []CdmRate) {
	grouped := make(map[string][]CdmRate)
	for _, rate := range rates {
		key := strings.ToUpper(strings.TrimSpace(rate.Airport))
		rate.Airport = key
		grouped[key] = append(grouped[key], rate)
	}

	for airport, airportRates := range grouped {
		config := s.ensureConfigLocked(airport)
		config.Rates = append([]CdmRate(nil), airportRates...)
	}
}

func (s *CdmConfigStore) mergeSidIntervalsLocked(intervals []CdmSidInterval) {
	grouped := make(map[string][]CdmSidInterval)
	for _, interval := range intervals {
		key := strings.ToUpper(strings.TrimSpace(interval.Airport))
		interval.Airport = key
		grouped[key] = append(grouped[key], interval)
	}

	for airport, airportIntervals := range grouped {
		config := s.ensureConfigLocked(airport)
		config.SidIntervals = append([]CdmSidInterval(nil), airportIntervals...)
	}
}

func (s *CdmConfigStore) mergeTaxiZonesLocked(zones []CdmTaxiZone) {
	grouped := make(map[string][]CdmTaxiZone)
	for _, zone := range zones {
		key := strings.ToUpper(strings.TrimSpace(zone.Airport))
		zone.Airport = key
		grouped[key] = append(grouped[key], zone)
	}

	for airport, airportZones := range grouped {
		config := s.ensureConfigLocked(airport)
		config.TaxiZones = append([]CdmTaxiZone(nil), airportZones...)
	}
}

func parseRateData(data []byte) ([]CdmRate, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var rates []CdmRate
		if err := json.Unmarshal(data, &rates); err != nil {
			return nil, err
		}
		return rates, nil
	}

	var rates []CdmRate
	for _, line := range sourceLines(trimmed) {
		parts := strings.Split(line, ":")
		if len(parts) != 9 {
			continue
		}

		rate := CdmRate{
			Airport:      strings.ToUpper(strings.TrimSpace(parts[0])),
			ArrRwyYes:    splitList(parts[2]),
			ArrRwyNo:     splitList(parts[3]),
			DepRwyYes:    splitList(parts[5]),
			DepRwyNo:     splitList(parts[6]),
			DependentRwy: splitList(parts[7]),
		}

		for _, entry := range splitList(parts[8]) {
			values := strings.Split(entry, "_")
			if len(values) == 0 {
				continue
			}
			rate.Rates = append(rate.Rates, strings.TrimSpace(values[0]))
			if len(values) > 1 {
				rate.RatesLvo = append(rate.RatesLvo, strings.TrimSpace(values[1]))
			}
		}

		rates = append(rates, rate)
	}
	return rates, nil
}

func parseSidIntervalData(data []byte) ([]CdmSidInterval, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var intervals []CdmSidInterval
		if err := json.Unmarshal(data, &intervals); err != nil {
			return nil, err
		}
		return intervals, nil
	}

	var intervals []CdmSidInterval
	for _, line := range sourceLines(trimmed) {
		parts := strings.Split(line, ",")
		if len(parts) != 5 {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if err != nil {
			return nil, err
		}
		intervals = append(intervals, CdmSidInterval{
			Airport: strings.ToUpper(strings.TrimSpace(parts[0])),
			Runway:  strings.TrimSpace(parts[1]),
			Sid1:    strings.TrimSpace(parts[2]),
			Sid2:    strings.TrimSpace(parts[3]),
			Value:   value,
		})
	}
	return intervals, nil
}

func parseTaxiZoneData(data []byte) ([]CdmTaxiZone, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}

	if strings.HasPrefix(trimmed, "[") {
		var zones []CdmTaxiZone
		if err := json.Unmarshal(data, &zones); err != nil {
			return nil, err
		}
		return zones, nil
	}

	var zones []CdmTaxiZone
	for _, line := range sourceLines(trimmed) {
		parts := strings.Split(line, ":")
		if len(parts) != 11 && len(parts) != 12 {
			continue
		}

		minutes, err := strconv.Atoi(strings.TrimSpace(parts[10]))
		if err != nil {
			return nil, err
		}

		polygon := make([]CdmTaxiPoint, 0, 4)
		for i := 2; i < 10; i += 2 {
			lat, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
			if err != nil {
				return nil, err
			}
			lon, err := strconv.ParseFloat(strings.TrimSpace(parts[i+1]), 64)
			if err != nil {
				return nil, err
			}
			polygon = append(polygon, CdmTaxiPoint{Lat: lat, Lon: lon})
		}

		zone := CdmTaxiZone{
			Airport: strings.ToUpper(strings.TrimSpace(parts[0])),
			Runway:  strings.TrimSpace(parts[1]),
			Minutes: minutes,
			Polygon: polygon,
		}
		if len(parts) == 12 {
			for _, remoteValue := range splitList(parts[11]) {
				if remoteValue == "" {
					continue
				}
				minutes, err := strconv.Atoi(remoteValue)
				if err != nil {
					return nil, err
				}
				zone.RemoteTaxiMinutes = append(zone.RemoteTaxiMinutes, minutes)
			}
		}
		zones = append(zones, zone)
	}
	return zones, nil
}

func sourceLines(value string) []string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func splitList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		return []string{trimmed}
	}
	return result
}
