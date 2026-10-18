package cdm

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

type readyRetryStore struct {
	mu   sync.Mutex
	data *models.CdmData
}

func (s *readyRetryStore) load() *models.CdmData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Clone()
}

func (s *readyRetryStore) save(data *models.CdmData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = data.Clone()
}

func newReadyRetryService(transport http.RoundTripper) (*Service, *readyRetryStore, *models.Strip) {
	store := &readyRetryStore{data: &models.CdmData{
		Tobt: stringPtr("1510"), Tsat: stringPtr("151000"), Ttot: stringPtr("152400"),
	}}
	strip := &models.Strip{Session: 1, Callsign: "SAS1635", Origin: "EKCH",
		Runway: stringPtr("04R"), Sid: stringPtr("NEXEN2A")}
	service := newTestCdmService(NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: transport})),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				copy := *strip
				return &copy, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return store.load(), nil
			},
			SetCdmDataFn: func(_ context.Context, _ int32, _ string, data *models.CdmData) (int64, error) {
				store.save(data)
				return 1, nil
			},
		}, nil, nil)
	markSessionLive(service, 1)
	return service, store, strip
}

func newReadyRetryData(store *readyRetryStore, fullState bool) *models.CdmData {
	data := store.load()
	data.Tobt = stringPtr("1515")
	data.Asrt = stringPtr("1442")
	data.ReadySyncPending = true
	data.Tsat, data.Ttot = nil, nil
	if fullState {
		data.Tsat, data.Ttot = stringPtr("155000"), stringPtr("160400")
	}
	store.save(data)
	return data
}

type readyRetryRequest struct {
	path  string
	query url.Values
}

type readyRetryRequests struct {
	mu    sync.Mutex
	items []readyRetryRequest
}

func (r *readyRetryRequests) record(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, readyRetryRequest{path: req.URL.Path, query: req.URL.Query()})
}

func (r *readyRetryRequests) snapshot() []readyRetryRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]readyRetryRequest(nil), r.items...)
}

func TestReadyExportSupersedesSleepingRetry(t *testing.T) {
	for _, fullState := range []bool{true, false} {
		t.Run(map[bool]string{true: "CDM data", false: "TOBT fallback"}[fullState], func(t *testing.T) {
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
				// A duplicate pre-READY callback also queues the obsolete state.
				service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, store.load())
				ready := newReadyRetryData(store, fullState)
				require.NoError(t, service.actionService.completeReadyViffSync(context.Background(), 1, strip, ready, false))
				require.False(t, store.load().ReadySyncPending)
				synctest.Wait()
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				calls := requests.snapshot()
				require.Len(t, calls, 3, "successful READY must invalidate the sleeping retry and its pending proposal")
				require.Equal(t, "REA/1", calls[1].query.Get("value"))
				if fullState {
					require.Equal(t, "151500", calls[2].query.Get("tobt"))
					require.Equal(t, "1442", calls[2].query.Get("asrt"))
					require.Equal(t, "155000", calls[2].query.Get("tsat"))
					service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, store.load())
					synctest.Wait()
					require.Len(t, requests.snapshot(), 3, "the successfully exported READY proposal is already confirmed")
				} else {
					require.Equal(t, "/ifps/dpi", calls[2].path)
					require.Equal(t, "OBT/1515/10", calls[2].query.Get("value"))
				}
			})
		})
	}
}

func TestReadyExportSerializesWithRetry(t *testing.T) {
	for _, retryInFlight := range []bool{true, false} {
		t.Run(map[bool]string{true: "retry already in flight", false: "retry wakes during READY"}[retryInFlight], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var requests readyRetryRequests
				release := make(chan struct{})
				var oldRequests int
				transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
					requests.record(req)
					response := "true"
					if req.URL.Query().Get("tobt") == "151000" {
						oldRequests++
						if oldRequests == 1 {
							response = "false"
						} else if retryInFlight {
							<-release
						}
					}
					if !retryInFlight && req.URL.Query().Get("value") == "REA/1" {
						<-release
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
				})
				service, store, strip := newReadyRetryService(transport)
				defer service.RemoveSession(1)
				service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, store.load())
				synctest.Wait()
				if retryInFlight {
					time.Sleep(5 * time.Second)
					synctest.Wait()
					require.Len(t, requests.snapshot(), 2)
				}
				ready := newReadyRetryData(store, true)
				done := make(chan error, 1)
				go func() {
					done <- service.actionService.completeReadyViffSync(context.Background(), 1, strip, ready, false)
				}()
				synctest.Wait()
				time.Sleep(5 * time.Second)
				synctest.Wait()
				require.Len(t, requests.snapshot(), 2, "READY and the retry must not send concurrently")
				close(release)
				synctest.Wait()
				require.NoError(t, <-done)
				before := len(requests.snapshot())
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				calls := requests.snapshot()
				require.Len(t, calls, before, "no obsolete request may be sent after READY succeeds")
				last := calls[len(calls)-1]
				require.Equal(t, "/ifps/setCdmData", last.path)
				require.Equal(t, "151500", last.query.Get("tobt"))
				require.Equal(t, "1442", last.query.Get("asrt"))
			})
		})
	}
}

func TestFailedReadyExportDoesNotConfirmOrInvalidateQueuedData(t *testing.T) {
	for _, rejectedPath := range []string{"/ifps/dpi", "/ifps/setCdmData"} {
		t.Run(rejectedPath, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var requests readyRetryRequests
				var rejectReady atomic.Bool
				rejectReady.Store(true)
				transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
					requests.record(req)
					response := "true"
					if req.URL.Query().Get("tobt") == "151000" || (rejectReady.Load() && req.URL.Path == rejectedPath) {
						response = "false"
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
				})
				service, store, strip := newReadyRetryService(transport)
				defer service.RemoveSession(1)
				service.pushViffAfterRecalcAsync(1, strip.Callsign, strip, store.load())
				synctest.Wait()
				tracker := &service.masterViffSync.pushRetries
				tracker.mu.Lock()
				original := tracker.entries[viffPushKey(1, strip.Callsign)]
				tracker.mu.Unlock()
				ready := newReadyRetryData(store, true)
				err := service.actionService.completeReadyViffSync(context.Background(), 1, strip, ready, false)
				require.ErrorIs(t, err, errViffUpdatePending)
				require.True(t, store.load().ReadySyncPending)
				tracker.mu.Lock()
				entry := tracker.entries[viffPushKey(1, strip.Callsign)]
				tracker.mu.Unlock()
				require.Same(t, original, entry, "failed READY must not acknowledge an unsent proposal")
				rejectReady.Store(false)
				require.NoError(t, service.actionService.completeReadyViffSync(context.Background(), 1, strip, ready, false))
				require.False(t, store.load().ReadySyncPending)
				before := len(requests.snapshot())
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				require.Len(t, requests.snapshot(), before, "subsequent successful READY supersedes the old retry")
			})
		})
	}
}
