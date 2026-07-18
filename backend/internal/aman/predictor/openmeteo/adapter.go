// Package openmeteo adapts the Open-Meteo GFS HTTP API to predictor's small,
// provider-neutral wind-profile contract. Its response DTOs remain private.
package openmeteo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman/predictor"
)

const (
	defaultBaseURL   = "https://api.open-meteo.com/v1/gfs"
	defaultTimeout   = 5 * time.Second
	defaultCacheTTL  = 30 * time.Minute
	maxResponseBytes = 1 << 20
)

type Config struct {
	BaseURL  string
	Client   *http.Client
	Now      func() time.Time
	CacheTTL time.Duration
}
type cacheEntry struct {
	levels                []predictor.WindLevel
	observedAt, expiresAt time.Time
}
type Adapter struct {
	baseURL  string
	client   *http.Client
	now      func() time.Time
	cacheTTL time.Duration
	mu       sync.RWMutex
	cache    map[string]cacheEntry
}

func New(config Config) *Adapter {
	if strings.TrimSpace(config.BaseURL) == "" {
		config.BaseURL = defaultBaseURL
	}
	if config.Client == nil {
		config.Client = &http.Client{Timeout: defaultTimeout}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = defaultCacheTTL
	}
	return &Adapter{baseURL: config.BaseURL, client: config.Client, now: config.Now, cacheTTL: config.CacheTTL, cache: make(map[string]cacheEntry)}
}

// WindProfile caches each grid coordinate plus forecast hour. Returned samples
// are rewritten to the current request instant/coordinate and deep-copied, so
// callers cannot mutate cache state. A failed refresh returns its last profile
// with its original expiry; the predictor then rejects stale data explicitly.
func (a *Adapter) WindProfile(ctx context.Context, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	if len(request.Samples) == 0 {
		return predictor.WindProfile{}, fmt.Errorf("wind request has no samples")
	}
	profile := predictor.WindProfile{SourceID: "open-meteo-gfs", SourceRevision: "gfs"}
	for _, sample := range request.Samples {
		entry, err := a.sample(ctx, sample)
		if err != nil {
			return predictor.WindProfile{}, err
		}
		profile.Samples = append(profile.Samples, predictor.WindSample{Position: sample.Position, At: sample.At, Levels: cloneLevels(entry.levels)})
		if profile.ObservedAt.IsZero() || entry.observedAt.Before(profile.ObservedAt) {
			profile.ObservedAt = entry.observedAt
		}
		if profile.ExpiresAt.IsZero() || entry.expiresAt.Before(profile.ExpiresAt) {
			profile.ExpiresAt = entry.expiresAt
		}
	}
	return profile, nil
}

func (a *Adapter) sample(ctx context.Context, sample predictor.WindSampleRequest) (cacheEntry, error) {
	key, err := cacheKey(sample)
	if err != nil {
		return cacheEntry{}, err
	}
	now := a.now().UTC()
	a.mu.RLock()
	cached, found := a.cache[key]
	a.mu.RUnlock()
	if found && now.Before(cached.expiresAt) {
		return cloneEntry(cached), nil
	}
	levels, err := a.fetchSample(ctx, sample)
	if err != nil {
		if found {
			return cloneEntry(cached), nil
		}
		return cacheEntry{}, err
	}
	entry := cacheEntry{levels: levels, observedAt: now, expiresAt: now.Add(a.cacheTTL)}
	a.mu.Lock()
	a.cache[key] = cloneEntry(entry)
	a.mu.Unlock()
	return cloneEntry(entry), nil
}

func (a *Adapter) fetchSample(ctx context.Context, sample predictor.WindSampleRequest) ([]predictor.WindLevel, error) {
	requestContext, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	u, err := url.Parse(a.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Open-Meteo URL: %w", err)
	}
	q := u.Query()
	q.Set("latitude", strconv.FormatFloat(sample.Position.LatitudeDegrees, 'f', 6, 64))
	q.Set("longitude", strconv.FormatFloat(sample.Position.LongitudeDegrees, 'f', 6, 64))
	q.Set("hourly", "wind_speed_1000hPa,wind_direction_1000hPa,geopotential_height_1000hPa,wind_speed_850hPa,wind_direction_850hPa,geopotential_height_850hPa,wind_speed_700hPa,wind_direction_700hPa,geopotential_height_700hPa,wind_speed_500hPa,wind_direction_500hPa,geopotential_height_500hPa,wind_speed_300hPa,wind_direction_300hPa,geopotential_height_300hPa,wind_speed_250hPa,wind_direction_250hPa,geopotential_height_250hPa,wind_speed_200hPa,wind_direction_200hPa,geopotential_height_200hPa,wind_speed_150hPa,wind_direction_150hPa,geopotential_height_150hPa")
	q.Set("wind_speed_unit", "kn")
	q.Set("timezone", "UTC")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(requestContext, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Open-Meteo request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("Open-Meteo status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("Open-Meteo response exceeds %d bytes", maxResponseBytes)
	}
	var payload gfsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Open-Meteo response: %w", err)
	}
	index, err := payload.Hourly.indexAt(sample.At)
	if err != nil {
		return nil, err
	}
	return payload.Hourly.levels(index)
}

func cacheKey(sample predictor.WindSampleRequest) (string, error) {
	if sample.At.IsZero() || sample.At.Location() != time.UTC || !finite(sample.Position.LatitudeDegrees) || !finite(sample.Position.LongitudeDegrees) || sample.Position.LatitudeDegrees < -90 || sample.Position.LatitudeDegrees > 90 || sample.Position.LongitudeDegrees < -180 || sample.Position.LongitudeDegrees > 180 || !finite(sample.AltitudeFeet) || sample.AltitudeFeet < 0 {
		return "", fmt.Errorf("wind sample is invalid")
	}
	return fmt.Sprintf("%.4f:%.4f:%s", sample.Position.LatitudeDegrees, sample.Position.LongitudeDegrees, sample.At.UTC().Truncate(time.Hour).Format(time.RFC3339)), nil
}
func cloneEntry(value cacheEntry) cacheEntry { value.levels = cloneLevels(value.levels); return value }
func cloneLevels(value []predictor.WindLevel) []predictor.WindLevel {
	return append([]predictor.WindLevel(nil), value...)
}

// gfsResponse is deliberately adapter-private: vendor JSON never reaches the predictor contract.
type gfsResponse struct {
	Hourly gfsHourly `json:"hourly"`
}
type gfsHourly struct {
	Time          []string   `json:"time"`
	Speed1000     []*float64 `json:"wind_speed_1000hPa"`
	Direction1000 []*float64 `json:"wind_direction_1000hPa"`
	Height1000    []*float64 `json:"geopotential_height_1000hPa"`
	Speed850      []*float64 `json:"wind_speed_850hPa"`
	Direction850  []*float64 `json:"wind_direction_850hPa"`
	Height850     []*float64 `json:"geopotential_height_850hPa"`
	Speed700      []*float64 `json:"wind_speed_700hPa"`
	Direction700  []*float64 `json:"wind_direction_700hPa"`
	Height700     []*float64 `json:"geopotential_height_700hPa"`
	Speed500      []*float64 `json:"wind_speed_500hPa"`
	Direction500  []*float64 `json:"wind_direction_500hPa"`
	Height500     []*float64 `json:"geopotential_height_500hPa"`
	Speed300      []*float64 `json:"wind_speed_300hPa"`
	Direction300  []*float64 `json:"wind_direction_300hPa"`
	Height300     []*float64 `json:"geopotential_height_300hPa"`
	Speed250      []*float64 `json:"wind_speed_250hPa"`
	Direction250  []*float64 `json:"wind_direction_250hPa"`
	Height250     []*float64 `json:"geopotential_height_250hPa"`
	Speed200      []*float64 `json:"wind_speed_200hPa"`
	Direction200  []*float64 `json:"wind_direction_200hPa"`
	Height200     []*float64 `json:"geopotential_height_200hPa"`
	Speed150      []*float64 `json:"wind_speed_150hPa"`
	Direction150  []*float64 `json:"wind_direction_150hPa"`
	Height150     []*float64 `json:"geopotential_height_150hPa"`
}

func (h gfsHourly) indexAt(at time.Time) (int, error) {
	lengths := []int{len(h.Time), len(h.Speed1000), len(h.Direction1000), len(h.Height1000), len(h.Speed850), len(h.Direction850), len(h.Height850), len(h.Speed700), len(h.Direction700), len(h.Height700), len(h.Speed500), len(h.Direction500), len(h.Height500), len(h.Speed300), len(h.Direction300), len(h.Height300), len(h.Speed250), len(h.Direction250), len(h.Height250), len(h.Speed200), len(h.Direction200), len(h.Height200), len(h.Speed150), len(h.Direction150), len(h.Height150)}
	for _, length := range lengths {
		if length != len(h.Time) || length == 0 {
			return 0, fmt.Errorf("Open-Meteo hourly response is incomplete")
		}
	}
	target := at.UTC().Truncate(time.Hour)
	for i, value := range h.Time {
		parsed, err := time.Parse("2006-01-02T15:04", value)
		if err == nil && parsed.Equal(target) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("Open-Meteo response has no wind at %s", target.Format(time.RFC3339))
}
func (h gfsHourly) levels(i int) ([]predictor.WindLevel, error) {
	values := [][]*float64{{h.Height1000[i], h.Speed1000[i], h.Direction1000[i]}, {h.Height850[i], h.Speed850[i], h.Direction850[i]}, {h.Height700[i], h.Speed700[i], h.Direction700[i]}, {h.Height500[i], h.Speed500[i], h.Direction500[i]}, {h.Height300[i], h.Speed300[i], h.Direction300[i]}, {h.Height250[i], h.Speed250[i], h.Direction250[i]}, {h.Height200[i], h.Speed200[i], h.Direction200[i]}, {h.Height150[i], h.Speed150[i], h.Direction150[i]}}
	levels := make([]predictor.WindLevel, 0, len(values))
	for _, value := range values {
		if value[0] == nil || value[1] == nil || value[2] == nil || !finite(*value[0]) || !finite(*value[1]) || !finite(*value[2]) {
			return nil, fmt.Errorf("Open-Meteo wind level is null or non-finite")
		}
		radians := *value[2] * math.Pi / 180
		levels = append(levels, predictor.WindLevel{AltitudeFeet: *value[0] * 3.28084, EastKnots: -*value[1] * math.Sin(radians), NorthKnots: -*value[1] * math.Cos(radians)})
	}
	return levels, nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

var _ predictor.WindProfileReader = (*Adapter)(nil)
