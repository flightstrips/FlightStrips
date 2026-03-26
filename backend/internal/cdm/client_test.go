package cdm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAirportMasterByICAO_UsesSharedBaseURLCache(t *testing.T) {
	var airportCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/airport", r.URL.Path)
		airportCalls.Add(1)
		require.NoError(t, json.NewEncoder(w).Encode([]AirportMaster{
			{ICAO: "EKCH", Position: "EKCH_B_GND"},
		}))
	}))
	defer server.Close()

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
		WithAirportMasterCacheTTL(time.Minute),
	)

	ctx := context.Background()

	first, err := client.AirportMasterByICAO(ctx, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "EKCH_B_GND", first.Position)

	second, err := client.AirportMasterByICAO(ctx, "ekch")
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, "EKCH_B_GND", second.Position)

	assert.Equal(t, int32(1), airportCalls.Load(), "expected /airport to be fetched once due to cache")
}

func TestIFPSSetTobt_UsesDpiEndpoint(t *testing.T) {
	var captured url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ifps/dpi", r.URL.Path)
		captured = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := NewClient(
		WithAPIKey("test-key"),
		WithBaseURL(server.URL),
	)

	err := client.IFPSSetTobt(context.Background(), "EIN123", "1030", 12)
	require.NoError(t, err)
	require.NotNil(t, captured)
	assert.Equal(t, "EIN123", captured.Get("callsign"))
	assert.Equal(t, "TOBT/1030/12", captured.Get("value"))
}
