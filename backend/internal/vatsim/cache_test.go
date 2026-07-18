package vatsim

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCacheVerifyPilotOwnsCallsign(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, `{"pilots":[{"cid":1234567,"callsign":"dal123"}]}`)

	ok, err := cache.VerifyPilotOwnsCallsign(context.Background(), "1234567", "DAL123")
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = cache.VerifyPilotOwnsCallsign(context.Background(), "7654321", "DAL123")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCacheVerifyPilotOwnsCallsignReturnsFalseForUnknownPilot(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, `{"pilots":[]}`)

	ok, err := cache.VerifyPilotOwnsCallsign(context.Background(), "1234567", "DAL123")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestCacheSnapshotDecodesPilotsAndPrefiles(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, `{
  "general":{"update_timestamp":"2020-07-12T10:00:00Z"},
  "pilots":[{
    "cid":1234567,"callsign":"dal123","latitude":55.618,"longitude":12.656,"altitude":34000,"groundspeed":456,
    "logon_time":"2026-07-12T08:00:00Z","last_updated":"2026-07-12T09:59:50Z",
    "flight_plan":{"flight_rules":"I","aircraft":"A320/M-SDE2E3FGHIJ1RWXYZ/LB1","aircraft_faa":"A320/L","aircraft_short":"A320","departure":"EKCH","arrival":"EGLL","alternate":"EGKK","deptime":"0815","enroute_time":"0145","altitude":"F350","remarks":"PBN/B2","route":"DCT LAM","revision_id":4,"assigned_transponder":"1234"}
  }],
  "prefiles":[{
    "cid":7654321,"callsign":" sas456 ","last_updated":"2026-07-12T09:58:00Z",
    "flight_plan":{"flight_rules":"V","aircraft":"C172/L-S","aircraft_faa":"C172/L","aircraft_short":"C172","departure":"EKRK","arrival":"EKCH","alternate":"","deptime":"1015","enroute_time":"0045","remarks":"TRAINING","route":"DCT","revision_id":2,"assigned_transponder":"7000"}
  }]
}`)

	require.NoError(t, cache.refresh(context.Background()))
	snapshot := cache.Snapshot()

	assert.Equal(t, time.Date(2020, 7, 12, 10, 0, 0, 0, time.UTC), snapshot.Timestamp)
	assert.NoError(t, snapshot.LastRefreshError)

	pilot, ok := snapshot.FlightByCallsign(" DAL123 ")
	require.True(t, ok)
	assert.True(t, pilot.Online())
	assert.False(t, pilot.Prefile())
	assert.Equal(t, "1234567", pilot.CID)
	assert.Equal(t, 55.618, pilot.Latitude)
	assert.Equal(t, 12.656, pilot.Longitude)
	assert.Equal(t, 34000, pilot.Altitude)
	assert.Equal(t, 456, pilot.Groundspeed)
	assert.Equal(t, time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC), pilot.LogonTime)
	assert.Equal(t, time.Date(2026, 7, 12, 9, 59, 50, 0, time.UTC), pilot.LastUpdated)
	assert.Equal(t, FlightPlan{
		FlightRules: "I", Aircraft: "A320/M-SDE2E3FGHIJ1RWXYZ/LB1", AircraftFAA: "A320/L", AircraftShort: "A320",
		Origin: "EKCH", Destination: "EGLL", Alternate: "EGKK", EOBT: "0815", EnrouteDuration: "0145", RequestedLevel: "F350",
		Remarks: "PBN/B2", Route: "DCT LAM", AssignedSquawk: "1234", Revision: 4,
	}, pilot.FlightPlan)

	prefile, ok := snapshot.FlightByCID("7654321")
	require.True(t, ok)
	assert.Equal(t, "SAS456", prefile.Callsign)
	assert.True(t, prefile.Prefile())
	assert.False(t, prefile.Online())
	assert.True(t, prefile.LogonTime.IsZero())
	assert.Equal(t, "EKRK", prefile.FlightPlan.Origin)
	assert.Equal(t, "EKCH", prefile.FlightPlan.Destination)
	assert.Equal(t, "7000", prefile.FlightPlan.AssignedSquawk)
}

func TestCacheSnapshotRetainsFlightsWithoutFlightPlans(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, `{"pilots":[{"cid":1234567,"callsign":"DAL123"}],"prefiles":[{"cid":7654321,"callsign":"SAS456"}]}`)
	require.NoError(t, cache.refresh(context.Background()))

	snapshot := cache.Snapshot()
	pilot, ok := snapshot.FlightByCallsign("DAL123")
	require.True(t, ok)
	assert.Equal(t, FlightPlan{}, pilot.FlightPlan)
	prefile, ok := snapshot.FlightByCallsign("SAS456")
	require.True(t, ok)
	assert.Equal(t, FlightPlan{}, prefile.FlightPlan)
}

func TestCacheSnapshotPrefersOnlinePilotForDuplicateCallsign(t *testing.T) {
	t.Parallel()

	cache, _ := newTestCache(t, `{
  "pilots":[{"cid":1234567,"callsign":"DAL123","altitude":15000,"flight_plan":{"route":"ONLINE","revision_id":1}}],
  "prefiles":[{"cid":7654321,"callsign":" dal123 ","flight_plan":{"route":"PREFILE","revision_id":9}}]
}`)
	require.NoError(t, cache.refresh(context.Background()))

	flights := cache.Snapshot().Flights()
	require.Len(t, flights, 1)
	assert.Equal(t, "1234567", flights[0].CID)
	assert.True(t, flights[0].Online())
	assert.Equal(t, "ONLINE", flights[0].FlightPlan.Route)
}

func TestCacheSnapshotDoesNotMoveFlightPlanRevisionBackward(t *testing.T) {
	t.Parallel()

	cache, payload := newTestCache(t, `{"pilots":[{"cid":1234567,"callsign":"DAL123","altitude":30000,"flight_plan":{"route":"NEW","revision_id":5}}]}`)
	require.NoError(t, cache.refresh(context.Background()))

	payload.Store(`{"pilots":[{"cid":1234567,"callsign":"DAL123","altitude":31000,"flight_plan":{"route":"STALE","revision_id":4}}]}`)
	require.NoError(t, cache.refresh(context.Background()))

	flight, ok := cache.Snapshot().FlightByCallsign("DAL123")
	require.True(t, ok)
	assert.Equal(t, 31000, flight.Altitude, "fresh position data should still be retained")
	assert.Equal(t, int64(5), flight.FlightPlan.Revision)
	assert.Equal(t, "NEW", flight.FlightPlan.Route)
}

func TestCacheSnapshotKeepsLastGoodDataAfterFailedRefreshAndRecovers(t *testing.T) {
	t.Parallel()

	cache, payload := newTestCache(t, `{"general":{"update_timestamp":"2026-07-12T10:00:00Z"},"pilots":[{"cid":1234567,"callsign":"DAL123","flight_plan":{"revision_id":1}}]}`)
	require.NoError(t, cache.refresh(context.Background()))
	beforeFailure := cache.Snapshot()

	payload.Store("FAIL")
	err := cache.refresh(context.Background())
	require.Error(t, err)
	afterFailure := cache.Snapshot()
	assert.Equal(t, beforeFailure.Timestamp, afterFailure.Timestamp)
	_, ok := afterFailure.FlightByCallsign("DAL123")
	assert.True(t, ok)
	assert.Error(t, afterFailure.LastRefreshError)

	payload.Store(`{"general":{"update_timestamp":"2026-07-12T10:00:15Z"},"pilots":[{"cid":1234567,"callsign":"DAL123","flight_plan":{"revision_id":2}}]}`)
	require.NoError(t, cache.refresh(context.Background()))
	afterRecovery := cache.Snapshot()
	assert.Equal(t, time.Date(2026, 7, 12, 10, 0, 15, 0, time.UTC), afterRecovery.Timestamp)
	assert.NoError(t, afterRecovery.LastRefreshError)
	flight, ok := afterRecovery.FlightByCallsign("DAL123")
	require.True(t, ok)
	assert.Equal(t, int64(2), flight.FlightPlan.Revision)
}

func TestCacheSnapshotConcurrentReadersDuringReplacement(t *testing.T) {
	cache, payload := newTestCache(t, `{"pilots":[{"cid":1234567,"callsign":"DAL123","flight_plan":{"revision_id":1}}]}`)
	require.NoError(t, cache.refresh(context.Background()))

	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for range 200 {
				snapshot := cache.Snapshot()
				_, _ = snapshot.FlightByCallsign("DAL123")
				_, _ = snapshot.FlightByCID("1234567")
				_ = snapshot.Flights()
			}
		}()
	}

	for revision := 2; revision < 202; revision++ {
		payload.Store(fmt.Sprintf(`{"pilots":[{"cid":1234567,"callsign":"DAL123","altitude":%d,"flight_plan":{"revision_id":%d}}]}`, revision, revision))
		require.NoError(t, cache.refresh(context.Background()))
	}
	readers.Wait()
}

func newTestCache(t *testing.T, initialPayload string) (*Cache, *atomic.Value) {
	t.Helper()

	var payload atomic.Value
	payload.Store(initialPayload)
	dataServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if payload.Load().(string) == "FAIL" {
			http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload.Load().(string)))
	}))
	t.Cleanup(dataServer.Close)

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"v3":["` + dataServer.URL + `"]}}`))
	}))
	t.Cleanup(statusServer.Close)

	return NewCache(statusServer.URL, time.Second, dataServer.Client()), &payload
}
