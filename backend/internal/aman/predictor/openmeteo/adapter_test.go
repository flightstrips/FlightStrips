package openmeteo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman/predictor"
	"github.com/stretchr/testify/require"
)

func TestAdapterMapsPrivateVendorPayloadAndCachesDeepCopy(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "kn", r.URL.Query().Get("wind_speed_unit"))
		require.Contains(t, r.URL.Query().Get("hourly"), "geopotential_height_300hPa")
		if calls > 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(gfsPayload()))
	}))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL, Now: func() time.Time { return now }})
	request := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}, At: now.Add(10 * time.Minute), AltitudeFeet: 10000}}}
	first, err := adapter.WindProfile(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "open-meteo-gfs", first.SourceID)
	require.Len(t, first.Samples[0].Levels, 8)
	require.InDelta(t, 20, first.Samples[0].Levels[0].EastKnots, .001)
	first.Samples[0].Levels[0].EastKnots = 999
	request.Samples[0].At = request.Samples[0].At.Add(20 * time.Second)
	cached, err := adapter.WindProfile(context.Background(), request)
	require.NoError(t, err)
	require.InDelta(t, 20, cached.Samples[0].Levels[0].EastKnots, .001)
	require.Equal(t, 1, calls, "same coordinate and forecast hour reuses cache")
}

func TestAdapterUsesGeopotentialHeightAtHighAltitude(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(gfsPayload())) }))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL, Now: func() time.Time { return now }})
	profile, err := adapter.WindProfile(context.Background(), predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{At: now.Add(5 * time.Minute), AltitudeFeet: 38000}}})
	require.NoError(t, err)
	east, _, ok := interpolate(profile.Samples[0].Levels, 38000)
	require.True(t, ok)
	require.Greater(t, east, 0.0)
}

func TestAdapterRejectsNonSuccessAndIncompletePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL})
	_, err := adapter.WindProfile(context.Background(), predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{}, At: time.Now().UTC(), AltitudeFeet: 1}}})
	require.Error(t, err)
}

func TestAdapterReturnsExpiredCachedProfileAfterProviderFailure(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(gfsPayload()))
		}
	}))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL, Now: func() time.Time { return now }, CacheTTL: time.Minute})
	request := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{At: now.Add(5 * time.Minute)}}}
	first, err := adapter.WindProfile(context.Background(), request)
	require.NoError(t, err)
	now = now.Add(2 * time.Minute)
	status = http.StatusBadGateway
	stale, err := adapter.WindProfile(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, first.ExpiresAt, stale.ExpiresAt)
	require.True(t, stale.ExpiresAt.Before(now))
}

func TestAdapterRejectsOversizedAndMalformedResponses(t *testing.T) {
	for _, body := range []string{string(make([]byte, maxResponseBytes+1)), `{"hourly":{}}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		adapter := New(Config{BaseURL: server.URL})
		_, err := adapter.WindProfile(context.Background(), predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{At: time.Now().UTC()}}})
		require.Error(t, err)
		server.Close()
	}
}

func TestAdapterCacheIsSafeForConcurrentCallers(t *testing.T) {
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(gfsPayload())) }))
	defer server.Close()
	adapter := New(Config{BaseURL: server.URL, Now: func() time.Time { return now }})
	request := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{At: now.Add(5 * time.Minute)}}}
	var group sync.WaitGroup
	errors := make(chan error, 24)
	for range 24 {
		group.Add(1)
		go func() {
			defer group.Done()
			profile, err := adapter.WindProfile(context.Background(), request)
			if err != nil {
				errors <- err
				return
			}
			if len(profile.Samples) != 1 {
				errors <- fmt.Errorf("unexpected sample count")
			}
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

var _ predictor.WindProfileReader = (*Adapter)(nil)

func interpolate(levels []predictor.WindLevel, altitude float64) (float64, float64, bool) {
	for i := 1; i < len(levels); i++ {
		if altitude >= levels[i-1].AltitudeFeet && altitude <= levels[i].AltitudeFeet {
			f := (altitude - levels[i-1].AltitudeFeet) / (levels[i].AltitudeFeet - levels[i-1].AltitudeFeet)
			return levels[i-1].EastKnots + (levels[i].EastKnots-levels[i-1].EastKnots)*f, 0, true
		}
	}
	return 0, 0, false
}
func gfsPayload() string {
	return `{"hourly":{"time":["2026-07-18T12:00"],"wind_speed_1000hPa":[20],"wind_direction_1000hPa":[270],"geopotential_height_1000hPa":[100],"wind_speed_850hPa":[25],"wind_direction_850hPa":[270],"geopotential_height_850hPa":[1500],"wind_speed_700hPa":[30],"wind_direction_700hPa":[270],"geopotential_height_700hPa":[3000],"wind_speed_500hPa":[40],"wind_direction_500hPa":[270],"geopotential_height_500hPa":[5500],"wind_speed_300hPa":[60],"wind_direction_300hPa":[270],"geopotential_height_300hPa":[9500],"wind_speed_250hPa":[70],"wind_direction_250hPa":[270],"geopotential_height_250hPa":[10500],"wind_speed_200hPa":[80],"wind_direction_200hPa":[270],"geopotential_height_200hPa":[12000],"wind_speed_150hPa":[90],"wind_direction_150hPa":[270],"geopotential_height_150hPa":[14000]}}`
}
