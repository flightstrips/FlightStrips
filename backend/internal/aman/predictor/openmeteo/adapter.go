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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman/predictor"

	"golang.org/x/sync/singleflight"
)

const (
	defaultBaseURL   = "https://api.open-meteo.com/v1/gfs"
	defaultTimeout   = 5 * time.Second
	defaultCacheTTL  = 30 * time.Minute
	maxResponseBytes = 1 << 20

	// GFS pressure-level variables are published on a 0.25° grid. Caching at
	// flight-position precision turns a moving aircraft into a new provider
	// request every reconciliation while returning the same model-cell data.
	gfsPressureGridDegrees = .25

	// Keep AMAN's own traffic safely below Open-Meteo's free-tier limits
	// (600/minute, 5,000/hour, 10,000/day). One request can contain every
	// uncached route sample for a flight.
	maxRequestsPerMinute = 500
	maxRequestsPerHour   = 4000
	maxRequestsPerDay    = 9000
)

type Config struct {
	BaseURL  string
	Client   *http.Client
	Now      func() time.Time
	CacheTTL time.Duration
	Cache    PersistentCache
}

// CachedSample is the durable representation of one grid-coordinate forecast
// profile. Implementations are shared by every backend instance.
type CachedSample struct {
	Key                   string
	Levels                []predictor.WindLevel
	ObservedAt, ExpiresAt time.Time
}

// PersistentCache prevents restarts and horizontally scaled backends from
// resetting weather samples or Open-Meteo quota accounting.
type PersistentCache interface {
	Load(context.Context, []string) ([]CachedSample, error)
	Store(context.Context, []CachedSample) error
	ReserveRequest(context.Context, time.Time) (bool, error)
}
type cacheEntry struct {
	levels                []predictor.WindLevel
	observedAt, expiresAt time.Time
}
type Adapter struct {
	baseURL    string
	client     *http.Client
	now        func() time.Time
	cacheTTL   time.Duration
	persistent PersistentCache
	mu         sync.RWMutex
	cache      map[string]cacheEntry
	requests   []time.Time
	refreshes  singleflight.Group
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
	return &Adapter{baseURL: config.BaseURL, client: config.Client, now: config.Now, cacheTTL: config.CacheTTL, persistent: config.Cache, cache: make(map[string]cacheEntry)}
}

// WindProfile caches each grid coordinate plus forecast hour. Returned samples
// are rewritten to the current request instant/coordinate and deep-copied, so
// callers cannot mutate cache state. A failed refresh returns its last profile
// with its original expiry; the predictor then rejects stale data explicitly.
func (a *Adapter) WindProfile(ctx context.Context, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	if len(request.Samples) == 0 {
		return predictor.WindProfile{}, fmt.Errorf("wind request has no samples")
	}
	now := a.now().UTC()
	keys := make([]string, len(request.Samples))
	entries := make([]cacheEntry, len(request.Samples))
	missing := make(map[string]predictor.WindSampleRequest)
	stale := make(map[string]cacheEntry)
	for i, sample := range request.Samples {
		key, err := cacheKey(sample)
		if err != nil {
			return predictor.WindProfile{}, err
		}
		keys[i] = key
		a.mu.RLock()
		cached, found := a.cache[key]
		a.mu.RUnlock()
		if found && now.Before(cached.expiresAt) {
			entries[i] = cloneEntry(cached)
			continue
		}
		if found {
			stale[key] = cached
		}
		missing[key] = providerGridSample(sample)
	}
	if len(missing) > 0 && a.persistent != nil {
		missingKeys := make([]string, 0, len(missing))
		for key := range missing {
			missingKeys = append(missingKeys, key)
		}
		cached, err := a.persistent.Load(ctx, missingKeys)
		if err != nil {
			return predictor.WindProfile{}, fmt.Errorf("load persisted Open-Meteo cache: %w", err)
		}
		for _, value := range cached {
			// pgx may materialize TIMESTAMPTZ values in the process's local
			// location. The predictor contract requires UTC instants, so restore
			// that invariant when hydrating the durable cache after a restart.
			entry := cacheEntry{levels: cloneLevels(value.Levels), observedAt: value.ObservedAt.UTC(), expiresAt: value.ExpiresAt.UTC()}
			a.mu.Lock()
			a.cache[value.Key] = cloneEntry(entry)
			a.mu.Unlock()
			if now.Before(entry.expiresAt) {
				for i, key := range keys {
					if key == value.Key {
						entries[i] = cloneEntry(entry)
					}
				}
				delete(missing, value.Key)
			} else {
				stale[value.Key] = entry
			}
		}
	}
	if len(missing) > 0 {
		requested := make([]predictor.WindSampleRequest, 0, len(missing))
		for _, sample := range missing {
			requested = append(requested, sample)
		}
		slices.SortFunc(requested, func(left, right predictor.WindSampleRequest) int {
			leftKey, _ := cacheKey(left)
			rightKey, _ := cacheKey(right)
			return strings.Compare(leftKey, rightKey)
		})
		refreshKey := strings.Join(missingKeys(requested), ",")
		value, err, _ := a.refreshes.Do(refreshKey, func() (any, error) {
			levels, fetchErr := a.fetchSamples(ctx, requested)
			if fetchErr != nil {
				return nil, fetchErr
			}
			refreshed := make(map[string]cacheEntry, len(requested))
			persisted := make([]CachedSample, 0, len(requested))
			for i, sample := range requested {
				key, _ := cacheKey(sample)
				entry := cacheEntry{levels: levels[i], observedAt: now, expiresAt: now.Add(a.cacheTTL)}
				a.mu.Lock()
				a.cache[key] = cloneEntry(entry)
				a.mu.Unlock()
				refreshed[key] = entry
				persisted = append(persisted, CachedSample{Key: key, Levels: cloneLevels(entry.levels), ObservedAt: entry.observedAt, ExpiresAt: entry.expiresAt})
			}
			if a.persistent != nil {
				if storeErr := a.persistent.Store(ctx, persisted); storeErr != nil {
					return nil, fmt.Errorf("store Open-Meteo cache: %w", storeErr)
				}
			}
			return refreshed, nil
		})
		if err != nil {
			for i, key := range keys {
				if !entries[i].observedAt.IsZero() {
					continue
				}
				cached, found := stale[key]
				if !found {
					return predictor.WindProfile{}, err
				}
				entries[i] = cloneEntry(cached)
			}
		} else {
			refreshed := value.(map[string]cacheEntry)
			for i, key := range keys {
				if entries[i].observedAt.IsZero() {
					entries[i] = cloneEntry(refreshed[key])
				}
			}
		}
	}
	profile := predictor.WindProfile{SourceID: "open-meteo-gfs", SourceRevision: "gfs"}
	for i, sample := range request.Samples {
		entry := entries[i]
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

func (a *Adapter) fetchSamples(ctx context.Context, samples []predictor.WindSampleRequest) ([][]predictor.WindLevel, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("wind fetch has no samples")
	}
	if err := a.reserveRequest(ctx, a.now().UTC()); err != nil {
		return nil, err
	}
	requestContext, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	u, err := url.Parse(a.baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse Open-Meteo URL: %w", err)
	}
	q := u.Query()
	latitudes, longitudes := make([]string, len(samples)), make([]string, len(samples))
	for i, sample := range samples {
		latitudes[i] = strconv.FormatFloat(sample.Position.LatitudeDegrees, 'f', 6, 64)
		longitudes[i] = strconv.FormatFloat(sample.Position.LongitudeDegrees, 'f', 6, 64)
	}
	q.Set("latitude", strings.Join(latitudes, ","))
	q.Set("longitude", strings.Join(longitudes, ","))
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
	payloads := []gfsResponse{}
	if len(samples) == 1 {
		var payload gfsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode Open-Meteo response: %w", err)
		}
		payloads = []gfsResponse{payload}
	} else if err := json.Unmarshal(body, &payloads); err != nil {
		return nil, fmt.Errorf("decode Open-Meteo response: %w", err)
	}
	if len(payloads) != len(samples) {
		return nil, fmt.Errorf("Open-Meteo returned %d profiles for %d samples", len(payloads), len(samples))
	}
	levels := make([][]predictor.WindLevel, len(samples))
	for i, payload := range payloads {
		index, err := payload.Hourly.indexAt(samples[i].At)
		if err != nil {
			return nil, err
		}
		levels[i], err = payload.Hourly.levels(index)
		if err != nil {
			return nil, err
		}
	}
	return levels, nil
}

// reserveRequest records one outgoing provider call only when all rolling
// budget windows remain below our deliberately conservative limits.
func (a *Adapter) reserveRequest(ctx context.Context, now time.Time) error {
	if now.IsZero() || now.Location() != time.UTC {
		return fmt.Errorf("Open-Meteo request time is invalid")
	}
	if a.persistent != nil {
		allowed, err := a.persistent.ReserveRequest(ctx, now)
		if err != nil {
			return fmt.Errorf("reserve persisted Open-Meteo request: %w", err)
		}
		if !allowed {
			return fmt.Errorf("Open-Meteo request budget exhausted")
		}
		return nil
	}
	dayAgo, hourAgo, minuteAgo := now.Add(-24*time.Hour), now.Add(-time.Hour), now.Add(-time.Minute)
	a.mu.Lock()
	defer a.mu.Unlock()
	first := 0
	for first < len(a.requests) && a.requests[first].Before(dayAgo) {
		first++
	}
	a.requests = append([]time.Time(nil), a.requests[first:]...)
	minute, hour := 0, 0
	for _, at := range a.requests {
		if !at.Before(minuteAgo) {
			minute++
		}
		if !at.Before(hourAgo) {
			hour++
		}
	}
	if len(a.requests) >= maxRequestsPerDay || hour >= maxRequestsPerHour || minute >= maxRequestsPerMinute {
		return fmt.Errorf("Open-Meteo request budget exhausted")
	}
	a.requests = append(a.requests, now)
	return nil
}

func cacheKey(sample predictor.WindSampleRequest) (string, error) {
	if sample.At.IsZero() || sample.At.Location() != time.UTC || !finite(sample.Position.LatitudeDegrees) || !finite(sample.Position.LongitudeDegrees) || sample.Position.LatitudeDegrees < -90 || sample.Position.LatitudeDegrees > 90 || sample.Position.LongitudeDegrees < -180 || sample.Position.LongitudeDegrees > 180 || !finite(sample.AltitudeFeet) || sample.AltitudeFeet < 0 {
		return "", fmt.Errorf("wind sample is invalid")
	}
	sample = providerGridSample(sample)
	return fmt.Sprintf("%.2f:%.2f:%s", sample.Position.LatitudeDegrees, sample.Position.LongitudeDegrees, sample.At.UTC().Truncate(time.Hour).Format(time.RFC3339)), nil
}

func providerGridSample(sample predictor.WindSampleRequest) predictor.WindSampleRequest {
	sample.Position.LatitudeDegrees = providerGridCoordinate(sample.Position.LatitudeDegrees)
	sample.Position.LongitudeDegrees = providerGridCoordinate(sample.Position.LongitudeDegrees)
	return sample
}

func providerGridCoordinate(value float64) float64 {
	normalized := math.Round(value/gfsPressureGridDegrees) * gfsPressureGridDegrees
	if normalized == 0 { // Avoid separate cache keys for -0 and 0.
		return 0
	}
	return normalized
}

func missingKeys(samples []predictor.WindSampleRequest) []string {
	keys := make([]string, 0, len(samples))
	for _, sample := range samples {
		key, _ := cacheKey(sample)
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
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
