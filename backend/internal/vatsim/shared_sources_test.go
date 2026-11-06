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
