package cdm

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type viffTestLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *viffTestLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *viffTestLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func TestPushViffAfterRecalcAsync_RetriesFeedLagWithoutAnotherRecalculation(t *testing.T) {
	var logs viffTestLogBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		var mu sync.Mutex
		var proposals []string
		stored := &readyRetryStore{}
		snapshot := func() []string {
			mu.Lock()
			defer mu.Unlock()
			return slices.Clone(proposals)
		}
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			proposals = append(proposals, r.URL.Query().Get("tsat"))
			mu.Unlock()
			response := "false"
			// SAS1635 was first processed 51 seconds after the rejected write.
			if time.Since(start) >= 51*time.Second {
				response = "true"
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
		})}))
		service := newTestCdmService(client, &testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: "SAS1635", Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return stored.load(), nil
			},
		}, nil, nil)
		markSessionLive(service, 1)
		defer service.RemoveSession(1)
		push := func(tsat string) {
			data := &models.CdmData{
				Tobt: stringPtr("1510"), Tsat: stringPtr(tsat), Ttot: stringPtr("1524"),
			}
			stored.save(data)
			service.pushViffAfterRecalcAsync(1, "SAS1635", &models.Strip{Callsign: "SAS1635", Origin: "EKCH"}, data)
		}
		push("151000")
		synctest.Wait()
		require.Len(t, snapshot(), 1)
		time.Sleep(21 * time.Second)
		synctest.Wait()
		require.Len(t, snapshot(), 3)
		push("151100")
		push("151200")
		synctest.Wait()
		require.Len(t, snapshot(), 3, "queued changes do not create another worker during the retry wait")
		time.Sleep(60 * time.Second)
		synctest.Wait()
		require.Equal(t, []string{"151000", "151000", "151000", "151200", "151200"}, snapshot())
		require.NotContains(t, logs.String(), `"level":"WARN"`)
		require.Contains(t, logs.String(), `"level":"DEBUG"`)
		before := len(snapshot())
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.Len(t, snapshot(), before, "acknowledgement stops automatic retries")
		push("151200")
		synctest.Wait()
		require.Len(t, snapshot(), before, "confirmed proposal is deduplicated")
	})
}

func TestPushViffAfterRecalcAsync_GivesUpAfterFiveMinutesOfFalse(t *testing.T) {
	var logs viffTestLogBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	synctest.Test(t, func(t *testing.T) {
		var requests atomic.Int32
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("false"))}, nil
		})}))
		service := newTestCdmService(client, nil, nil, nil)
		markSessionLive(service, 1)
		defer service.RemoveSession(1)
		push := func(tsat string) {
			service.pushViffAfterRecalcAsync(1, "SAS1635", nil, &models.CdmData{
				Tobt: stringPtr("1510"), Tsat: stringPtr(tsat), Ttot: stringPtr("1524"),
			})
		}
		push("151000")
		synctest.Wait()
		time.Sleep(4 * time.Minute)
		synctest.Wait()
		require.Empty(t, logs.String(), "feed lag should not produce warnings")
		push("151100")
		time.Sleep(time.Minute - time.Nanosecond)
		synctest.Wait()
		require.Empty(t, logs.String(), "changing proposals must not make early warnings")
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.Greater(t, requests.Load(), int32(6), "retrying must continue beyond the old six-attempt limit")
		require.Equal(t, 1, strings.Count(logs.String(), `"level":"WARN"`), "the aircraft's outage survives changing proposals")
		require.Contains(t, logs.String(), `"callsign":"SAS1635"`)
		require.Contains(t, logs.String(), `"tsat":"151100"`)
		require.Contains(t, logs.String(), "giving up")
		before := requests.Load()
		push("151100")
		synctest.Wait()
		require.Equal(t, before, requests.Load(), "sync must not immediately restart an exhausted update")
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.Equal(t, before, requests.Load(), "there must be no retries after the five-minute cutoff")
		require.Equal(t, 1, strings.Count(logs.String(), `"level":"WARN"`), "warn once when giving up")
	})
}

func TestPushViffAfterRecalcAsync_RemovalCancelsWaitingRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests atomic.Int32
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("false"))}, nil
		})}))
		service := newTestCdmService(client, nil, nil, nil)
		markSessionLive(service, 1)
		service.pushViffAfterRecalcAsync(1, "SAS1635", nil, &models.CdmData{
			Tobt: stringPtr("1510"), Tsat: stringPtr("151000"), Ttot: stringPtr("1524"),
		})
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load())
		service.RemoveSession(1)
		synctest.Wait()
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load(), "session removal cancels a sleeping retry")
	})
}

func TestPushViffAfterRecalcAsync_StopsRetryingDeletedFlight(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests atomic.Int32
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("false"))}, nil
		})}))
		service := newTestCdmService(client, &testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return nil, pgx.ErrNoRows
			},
		}, nil, nil)
		markSessionLive(service, 1)
		defer service.RemoveSession(1)
		service.pushViffAfterRecalcAsync(1, "SAS1635", nil, &models.CdmData{
			Tobt: stringPtr("1510"), Tsat: stringPtr("151000"), Ttot: stringPtr("1524"),
		})
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load())
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load())
		require.Empty(t, service.masterViffSync.pushRetries.entries)
	})
}

func TestPushViffAfterRecalcAsync_RetryDoesNotBypassPendingReady(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var requests atomic.Int32
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			requests.Add(1)
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("false"))}, nil
		})}))
		current := &models.CdmData{
			Tobt: stringPtr("1510"), Tsat: stringPtr("151000"), Ttot: stringPtr("1524"),
		}
		var readyPending atomic.Bool
		service := newTestCdmService(client, &testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: "SAS1635", Origin: "EKCH"}, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				data := current.Clone()
				data.ReadySyncPending = readyPending.Load()
				return data, nil
			},
		}, nil, nil)
		markSessionLive(service, 1)
		defer service.RemoveSession(1)
		service.pushViffAfterRecalcAsync(1, "SAS1635", nil, current)
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load())
		readyPending.Store(true)
		time.Sleep(time.Minute)
		synctest.Wait()
		require.Equal(t, int32(1), requests.Load(), "retry must wait for READY's own REA/1 acknowledgement")
		require.Empty(t, service.masterViffSync.pushRetries.entries)
	})
}
