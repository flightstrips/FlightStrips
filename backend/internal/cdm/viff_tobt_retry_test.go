package cdm

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTobtUpdateWithinProtectedWindowRefreshesSleepingCdmRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests readyRetryRequests
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.record(req)
			response := "true"
			if req.URL.Query().Get("tobt") == "151000" {
				response = "false"
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})
		service, store, strip := newReadyRetryService(transport)
		defer service.RemoveSession(1)
		service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, store.load())
		synctest.Wait()
		require.Len(t, requests.snapshot(), 1)
		require.NoError(t, service.HandleTobtUpdate(context.Background(), 1, strip.Callsign, "1512", "EKCH_DEL", "ATC"))
		synctest.Wait()
		calls := requests.snapshot()
		require.Len(t, calls, 2, "TOBT update is sent without another CDM recalculation")
		require.Equal(t, "OBT/1512/10", calls[1].query.Get("value"))
		require.False(t, store.load().NeedsLocalRecalculation())
		time.Sleep(5 * time.Second)
		synctest.Wait()
		calls = requests.snapshot()
		require.Len(t, calls, 3, "TOBT acknowledgement must not discard the unacknowledged TSAT/TTOT export")
		require.Equal(t, "151200", calls[2].query.Get("tobt"))
		require.Equal(t, "151000", calls[2].query.Get("tsat"))
		require.Equal(t, "152400", calls[2].query.Get("ttot"))
		tracker := &service.masterViffSync.pushRetries
		tracker.mu.Lock()
		confirmedTobt := tracker.entries[viffPushKey(1, strip.Callsign)].confirmed.Params.Tobt
		tracker.mu.Unlock()
		require.Equal(t, "151200", confirmedTobt, "deduplication tracks the payload actually exported")
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.Len(t, requests.snapshot(), 3)
	})
}

func TestTobtUpdateSerializesWithCdmRetry(t *testing.T) {
	for _, retryInFlight := range []bool{true, false} {
		t.Run(map[bool]string{true: "retry already in flight", false: "TOBT already in flight"}[retryInFlight], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var requests readyRetryRequests
				release := make(chan struct{})
				oldRequests := 0
				transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
					requests.record(req)
					response := "true"
					if req.URL.Query().Get("tobt") == "151000" {
						oldRequests++
						response = "false"
						if retryInFlight && oldRequests > 1 {
							<-release
						}
					}
					if !retryInFlight && req.URL.Path == "/ifps/dpi" {
						<-release
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
				})
				service, store, strip := newReadyRetryService(transport)
				defer service.RemoveSession(1)
				old := store.load()
				service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, old)
				synctest.Wait()
				if retryInFlight {
					time.Sleep(5 * time.Second)
					synctest.Wait()
					require.Len(t, requests.snapshot(), 2)
				}
				require.NoError(t, service.HandleTobtUpdate(context.Background(), 1, strip.Callsign, "1512", "EKCH_DEL", "ATC"))
				synctest.Wait()
				time.Sleep(5 * time.Second)
				synctest.Wait()
				require.Len(t, requests.snapshot(), 2, "TOBT and a CDM retry must not send concurrently")
				// A delayed callback can still carry the obsolete pre-TOBT snapshot.
				service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, old)
				close(release)
				synctest.Wait()
				time.Sleep(30 * time.Second)
				synctest.Wait()
				calls := requests.snapshot()
				require.Equal(t, "151200", calls[len(calls)-1].query.Get("tobt"))
				sentTobt := false
				for _, call := range calls {
					if call.query.Get("value") == "OBT/1512/10" {
						sentTobt = true
					} else if sentTobt && call.path == "/ifps/setCdmData" {
						require.Equal(t, "151200", call.query.Get("tobt"), "no older TOBT may be written after the newer TOBT acknowledgement")
					}
				}
				require.True(t, sentTobt)
				before := len(calls)
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				require.Len(t, requests.snapshot(), before)
			})
		})
	}
}

func TestTobtAcknowledgementRefreshesUnsentInitialCdmExport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests readyRetryRequests
		release := make(chan struct{})
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			requests.record(req)
			if req.URL.Path == "/ifps/dpi" {
				<-release
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("true"))}, nil
		})
		service, store, strip := newReadyRetryService(transport)
		defer service.RemoveSession(1)
		old := store.load()
		require.NoError(t, service.HandleTobtUpdate(context.Background(), 1, strip.Callsign, "1512", "EKCH_DEL", "ATC"))
		synctest.Wait()
		require.Len(t, requests.snapshot(), 1)
		service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, old)
		synctest.Wait()
		require.Len(t, requests.snapshot(), 1)
		close(release)
		synctest.Wait()
		calls := requests.snapshot()
		require.Len(t, calls, 2)
		require.Equal(t, "OBT/1512/10", calls[0].query.Get("value"))
		require.Equal(t, "151200", calls[1].query.Get("tobt"), "even an unsent initial proposal must refresh after a TOBT acknowledgement")
	})
}
