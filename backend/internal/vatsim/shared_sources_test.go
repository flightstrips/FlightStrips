package vatsim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSharedRefreshCoalescesAcrossConsumers(t *testing.T) {
	for _, radios := range []bool{false, true} {
		t.Run(map[bool]string{false: "network", true: "radios"}[radios], func(t *testing.T) {
			var requests atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/status" {
					_, _ = w.Write([]byte(`{"data":{"v3":["http://` + r.Host + `/data"]}}`))
					return
				}
				if requests.Add(1) == 1 {
					close(entered)
				}
				<-release
				if radios {
					_, _ = w.Write([]byte(`[{"callsign":"EKDK_CTR","frequency":118105000}]`))
				} else {
					_, _ = w.Write([]byte(`{"pilots":[],"controllers":[{"callsign":"EKDK_CTR","text_atis":["shared"]}]}`))
				}
			}))
			defer server.Close()
			var refresh func(context.Context) error
			if radios {
				refresh = NewTransceiverCache(server.URL, time.Second, server.Client(), nil).refresh
			} else {
				refresh = NewCache(server.URL+"/status", time.Second, server.Client()).refresh
			}
			var wg sync.WaitGroup
			for range 24 {
				wg.Go(func() { require.NoError(t, refresh(t.Context())) })
			}
			<-entered
			// All consumers enter the same in-flight acquisition, including EFB misses.
			time.Sleep(50 * time.Millisecond)
			close(release)
			wg.Wait()
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

func TestStaffingSnapshotIsImmutableAndRetainsLastGoodHealth(t *testing.T) {
	c, payload := newTestCache(t, `{"controllers":[{"callsign":"EKDK_CTR","text_atis":["original"]}]}`)
	require.NoError(t, c.refresh(t.Context()))
	before := c.Snapshot()
	controller, ok := before.ControllerByCallsign("ekdk_ctr")
	require.True(t, ok)
	controller.TextATIS[0] = "mutated"
	payload.Store(`broken`)
	require.Error(t, c.refresh(t.Context()))
	after := c.Snapshot()
	require.Equal(t, before.Timestamp, after.Timestamp)
	require.Error(t, after.LastRefreshError)
	require.NoError(t, before.LastRefreshError)
	require.Equal(t, "original", after.Controllers()[0].TextATIS[0])
}

func TestRadioSnapshotRetainsGenerationAndHealth(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`[{"callsign":"EKDK_CTR","frequency":118105000}]`))
	}))
	defer srv.Close()
	c := NewTransceiverCache(srv.URL, time.Second, srv.Client(), nil)
	require.NoError(t, c.refresh(t.Context()))
	before := c.Snapshot()
	before.GetFrequencies("EKDK_CTR")[0] = "bad"
	fail.Store(true)
	require.Error(t, c.refresh(t.Context()))
	after := c.Snapshot()
	require.Equal(t, before.Timestamp, after.Timestamp)
	require.Error(t, after.LastRefreshError)
	require.NoError(t, before.LastRefreshError)
	require.Equal(t, []string{"118.105"}, after.GetFrequencies("EKDK_CTR"))
}

func TestEFBLookupMissesShareAcquisitionCadence(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"data":{"v3":["http://` + r.Host + `/data"]}}`))
			return
		}
		requests.Add(1)
		_, _ = w.Write([]byte(`{"pilots":[]}`))
	}))
	defer upstream.Close()
	c := NewCache(upstream.URL+"/status", time.Hour, upstream.Client())
	for range 40 {
		_, found, err := c.GetCallsignByCID(t.Context(), "123")
		require.NoError(t, err)
		require.False(t, found)
		owned, err := c.VerifyPilotOwnsCallsign(t.Context(), "123", "SAS123")
		require.NoError(t, err)
		require.False(t, owned)
	}
	require.EqualValues(t, 1, requests.Load(), "negative results are shared across EFB tabs and verification consumers")
}

func TestCancellingOneLookupDoesNotCancelSharedAcquisition(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = w.Write([]byte(`{"data":{"v3":["http://` + r.Host + `/data"]}}`))
			return
		}
		requests.Add(1)
		close(entered)
		<-release
		_, _ = w.Write([]byte(`{"pilots":[{"cid":123,"callsign":"SAS123"}]}`))
	}))
	defer upstream.Close()
	c := NewCache(upstream.URL+"/status", time.Hour, upstream.Client())
	ctx, cancel := context.WithCancel(t.Context())
	finished := make(chan error, 1)
	go func() { _, _, err := c.GetCallsignByCID(ctx, "123"); finished <- err }()
	<-entered
	cancel()
	require.ErrorIs(t, <-finished, context.Canceled)
	close(release)
	callsign, found, err := c.GetCallsignByCID(t.Context(), "123")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "SAS123", callsign)
	require.NoError(t, c.Snapshot().LastRefreshError)
	require.EqualValues(t, 1, requests.Load())
}
