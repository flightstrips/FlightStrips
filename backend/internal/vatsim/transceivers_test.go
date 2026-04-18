package vatsim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransceiverCacheGetFrequenciesNormalizesAndDeduplicates(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"callsign":"ekch_a_twr","frequency":118105000},
			{"callsign":"EKCH_A_TWR","frequency":118105000},
			{"callsign":"EKCH_A_TWR","frequency":119300000}
		]`))
	}))
	defer server.Close()

	cache := NewTransceiverCache(server.URL, time.Second, server.Client(), nil)

	require.NoError(t, cache.refresh(context.Background()))
	assert.Equal(t, []string{"118.105", "119.300"}, cache.GetFrequencies("EKCH_A_TWR"))
}

func TestTransceiverCacheRefreshInvokesCallbackOnlyWhenSnapshotChanges(t *testing.T) {
	t.Parallel()

	var payload atomic.Value
	payload.Store(`[{"callsign":"EKCH_A_TWR","frequency":118105000}]`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload.Load().(string)))
	}))
	defer server.Close()

	var updates atomic.Int32
	cache := NewTransceiverCache(server.URL, time.Second, server.Client(), func(context.Context) error {
		updates.Add(1)
		return nil
	})

	require.NoError(t, cache.refresh(context.Background()))
	require.Equal(t, int32(1), updates.Load())

	require.NoError(t, cache.refresh(context.Background()))
	require.Equal(t, int32(1), updates.Load())

	payload.Store(`[{"callsign":"EKCH_A_TWR","frequency":119300000}]`)

	require.NoError(t, cache.refresh(context.Background()))
	require.Equal(t, int32(2), updates.Load())
}

func TestTransceiverCacheRefreshRetriesPendingCallbackWithoutSnapshotChange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"callsign":"EKCH_A_TWR","frequency":118105000}]`))
	}))
	defer server.Close()

	var attempts atomic.Int32
	cache := NewTransceiverCache(server.URL, time.Second, server.Client(), func(context.Context) error {
		if attempts.Add(1) == 1 {
			return assert.AnError
		}
		return nil
	})

	err := cache.refresh(context.Background())
	require.ErrorIs(t, err, assert.AnError)

	require.NoError(t, cache.refresh(context.Background()))
	require.Equal(t, int32(2), attempts.Load())
}
