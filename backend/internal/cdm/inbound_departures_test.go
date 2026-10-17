package cdm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssessInboundDeparture(t *testing.T) {
	for _, tt := range []struct {
		name, ctot, atot, status string
		delta, outside           int
		valid                    bool
	}{
		{"early", "1200", "1154", "early", -360, 60, true},
		{"lower boundary", "1200", "1155", "compliant", -300, 0, true},
		{"upper boundary", "1200", "1210", "compliant", 600, 0, true},
		{"late example", "1906", "1924", "late", 1080, 480, true},
		{"seconds early", "120000", "115459", "early", -301, 1, true},
		{"seconds late", "120000", "121001", "late", 601, 1, true},
		{"midnight late boundary", "2358", "0008", "compliant", 600, 0, true},
		{"midnight late", "2358", "0009", "late", 660, 60, true},
		{"midnight early", "0002", "2356", "early", -360, 60, true},
		{"midnight early boundary", "0002", "2357", "compliant", -300, 0, true},
		{"missing ctot", "", "1215", "", 0, 0, false},
		{"missing atot", "1200", "", "", 0, 0, false},
		{"invalid hour", "2400", "1215", "", 0, 0, false},
		{"invalid minute", "1260", "1215", "", 0, 0, false},
		{"invalid second", "120060", "121500", "", 0, 0, false},
		{"invalid digit", "12x0", "1215", "", 0, 0, false},
		{"invalid format", "12:00", "1215", "", 0, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			flight, valid := assessInboundDeparture(IFPSData{CTOT: tt.ctot, ATOT: tt.atot, CDMStatus: "COMPLY"})
			assert.Equal(t, tt.valid, valid)
			assert.Equal(t, tt.status, flight.Status)
			assert.Equal(t, tt.delta, flight.DeviationSeconds)
			assert.Equal(t, tt.outside, flight.OutsideWindowSeconds)
		})
	}
}

func TestInboundCachePreservesDepartedSlotAndFailures(t *testing.T) {
	m := &inboundDepartures{}
	entry := &inboundAirport{snapshot: frontend.InboundDeparturesEvent{Airport: "EKCH"}, departed: make(map[string]inboundDepartureSnapshot)}
	now := time.Date(2026, 10, 13, 20, 43, 0, 0, time.UTC)
	row := IFPSData{Callsign: "FBT08F", Departure: "LFLL", Arrival: "EKCH", CTOT: "1906", ATOT: "1924"}
	m.apply(entry, BulkIFPSData{row, row, {Callsign: "OTHER", Arrival: "EKBI", CTOT: "1200", ATOT: "1220"}}, nil, now)
	require.Len(t, entry.snapshot.Flights, 1)
	assert.Equal(t, 480, entry.snapshot.Flights[0].OutsideWindowSeconds)
	row.CTOT = "1924"
	row.ATOT = "192400"
	m.apply(entry, BulkIFPSData{row}, nil, now.Add(inboundPollInterval))
	assert.Equal(t, "1906", entry.snapshot.Flights[0].Ctot)
	before := entry.snapshot
	m.apply(entry, nil, errors.New("timeout"), now.Add(2*inboundPollInterval))
	assert.True(t, entry.snapshot.Stale)
	assert.Equal(t, before.Flights, entry.snapshot.Flights)
	assert.Equal(t, before.UpdatedAt, entry.snapshot.UpdatedAt)
	m.apply(entry, BulkIFPSData{}, nil, now.Add(3*inboundPollInterval))
	assert.Empty(t, entry.snapshot.Flights)
	m.apply(entry, BulkIFPSData{row}, nil, now.Add(4*inboundPollInterval))
	require.Len(t, entry.snapshot.Flights, 1)
	assert.Equal(t, "1906", entry.snapshot.Flights[0].Ctot, "retain the departed snapshot through temporary disappearance")
	row.Departure = "EGLL"
	m.apply(entry, BulkIFPSData{row}, nil, now.Add(5*inboundPollInterval))
	assert.Empty(t, entry.snapshot.Flights, "a reused callsign on a different flight must not inherit the slot")
	row.Departure = "LFLL"
	m.apply(entry, BulkIFPSData{row}, nil, now.Add(24*time.Hour))
	assert.Empty(t, entry.snapshot.Flights, "next-day flight must not inherit previous history")
}

func TestInboundMissingDepartedSlotNeverUsesLaterSlot(t *testing.T) {
	m := &inboundDepartures{}
	entry := &inboundAirport{snapshot: frontend.InboundDeparturesEvent{Airport: "EKCH"}, departed: make(map[string]inboundDepartureSnapshot)}
	row := IFPSData{Callsign: "SAS1", Departure: "EGLL", Arrival: "EKCH", ATOT: "1215", CDMStatus: "AIRB"}
	m.apply(entry, BulkIFPSData{row}, nil, time.Now())
	assert.Empty(t, entry.snapshot.Flights)
	row.CTOT = "1200"
	m.apply(entry, BulkIFPSData{row}, nil, time.Now())
	assert.Empty(t, entry.snapshot.Flights)
}

type inboundTransport func(*http.Request) (*http.Response, error)

func (f inboundTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestInboundPollingSharedLifecycleAndRateLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		client := NewClient(WithAPIKey("test"), WithHTTPClient(&http.Client{Transport: inboundTransport(func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "/ifps/arrAirport", req.URL.Path)
			assert.Equal(t, "EKCH", req.URL.Query().Get("airport"))
			n := calls.Add(1)
			if n == 2 {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"900"}}, Body: io.NopCloser(strings.NewReader("rate limited"))}, nil
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[{"callsign":"SAS1","departure":"EGLL","arrival":"EKCH","ctot":"1200","atot":"1215"}]`))}, nil
		})}))
		s := newCdmService(client, nil, &testutil.MockSessionRepository{ListFn: func(context.Context) ([]*models.Session, error) { return nil, nil }}, nil, testCdmPublisher{}, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		s.inbound.start(ctx)
		s.RegisterInboundSession(1, "LIVE", "ekch")
		s.RegisterInboundSession(2, "LIVE", "EKCH")
		s.RegisterInboundSession(3, "PLAYBACK", "EKCH")
		synctest.Wait()
		assert.Equal(t, int32(1), calls.Load(), "immediate single poll for both sessions")
		first := s.CurrentInboundDepartures(1, "EKCH")
		second := s.CurrentInboundDepartures(2, "EKCH")
		assert.Equal(t, first.Flights, second.Flights)
		assert.False(t, s.CurrentInboundDepartures(3, "EKCH").Enabled)
		first.Flights[0].Ctot = "changed"
		assert.Equal(t, "1200", s.CurrentInboundDepartures(1, "EKCH").Flights[0].Ctot, "snapshot reads must not mutate the cache")
		time.Sleep(4 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(1), calls.Load())
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(2), calls.Load())
		assert.True(t, s.CurrentInboundDepartures(1, "EKCH").Stale)
		s.RegisterInboundSession(4, "LIVE", "EKCH")
		synctest.Wait()
		assert.Equal(t, int32(2), calls.Load(), "new session uses cache without bypassing backoff")
		require.Len(t, s.CurrentInboundDepartures(4, "EKCH").Flights, 1)
		time.Sleep(14 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(2), calls.Load())
		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(3), calls.Load())
		assert.False(t, s.CurrentInboundDepartures(1, "EKCH").Stale)
		s.RemoveSession(1)
		s.RemoveSession(2)
		time.Sleep(inboundPollInterval)
		synctest.Wait()
		assert.Equal(t, int32(4), calls.Load(), "remaining session keeps polling")
		s.RemoveSession(4)
		time.Sleep(20 * time.Minute)
		synctest.Wait()
		assert.Equal(t, int32(4), calls.Load(), "last session stops polling")
		s.RegisterInboundSession(4, "LIVE", "EKCH")
		synctest.Wait()
		assert.Equal(t, int32(4), calls.Load(), "removed sessions cannot restart polling")
	})
}

func TestArrivalEndpointRejectsInvalidResponsesAndParsesRetryDate(t *testing.T) {
	for _, body := range []string{"null", "{}", "not json", `[{"ctot":1234}]`} {
		client := NewClient(WithHTTPClient(&http.Client{Transport: inboundTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}))
		_, err := client.IFPSByArrivalAirport(context.Background(), "EKCH")
		require.Error(t, err)
	}
	client := NewClient(WithHTTPClient(&http.Client{Transport: inboundTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {time.Now().Add(20 * time.Minute).UTC().Format(http.TimeFormat)}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
	})}))
	_, err := client.IFPSByArrivalAirport(context.Background(), "EKCH")
	var apiError *APIError
	require.ErrorAs(t, err, &apiError)
	assert.Greater(t, apiError.RetryAfter, 19*time.Minute)
}
