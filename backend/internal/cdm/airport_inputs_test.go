package cdm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAirportInputsShareAcquisitionAndIsolateDefaults(t *testing.T) {
	var requests atomic.Int32
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`[{"airport":"AAAA","rates":["31"]},{"airport":"BBBB","rates":["42"]},{"airport":"EKCH","rates":["99"]}]`))
	}))
	defer server.Close()
	store := NewAirportConfigStore([]AirportConfigInput{
		{Config: &CdmAirportConfig{Airport: "AAAA", DefaultRate: 31, DefaultRateLvo: 11, DefaultTaxiMinutes: 3, DeiceConfig: CdmDeiceConfig{Light: 4}}, RateURI: server.URL},
		{Config: &CdmAirportConfig{Airport: "BBBB", DefaultRate: 42, DefaultRateLvo: 12, DefaultTaxiMinutes: 8, DeiceConfig: CdmDeiceConfig{Light: 9}}, RateURI: server.URL},
	}, time.Minute, server.Client())
	store.refresh(t.Context())
	require.EqualValues(t, 1, requests.Load())
	a, b := store.ConfigForAirport("AAAA"), store.ConfigForAirport("BBBB")
	require.Equal(t, 3, a.DefaultTaxiMinutes)
	require.Equal(t, 9, b.DeiceConfig.Light)
	require.Equal(t, "31", a.Rates[0].Rates[0])
	require.Equal(t, "42", b.Rates[0].Rates[0])
	a.Rates[0].Rates[0] = "mutated"
	require.Equal(t, "31", store.ConfigForAirport("AAAA").Rates[0].Rates[0])
	require.Nil(t, store.ConfigForAirport("EKCH"))
	require.Nil(t, store.DefaultConfigForAirport("UNKNOWN"))
	store.SetLvo("UNKNOWN", true)
	store.SetDelay(CdmDelay{Airport: "UNKNOWN"})
	require.Nil(t, store.ConfigForAirport("UNKNOWN"))
	fail.Store(true)
	store.refresh(t.Context())
	require.Equal(t, "42", store.ConfigForAirport("BBBB").Rates[0].Rates[0])
}

func TestAirportInputsUseSessionRunwaysAndRejectUnconfiguredCalculation(t *testing.T) {
	store := NewAirportConfigStore([]AirportConfigInput{{Config: &CdmAirportConfig{Airport: "AAAA", DefaultRate: 31, DefaultRateLvo: 11, DefaultTaxiMinutes: 3}}}, 0, nil)
	var reads atomic.Int32
	service := newTestSequenceService(&testutil.MockStripRepository{ListByOriginFn: func(context.Context, int32, string) ([]*models.Strip, error) { reads.Add(1); return nil, nil }}, &testutil.MockSessionRepository{GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
		return &models.Session{ID: id, Airport: "AAAA"}, nil
	}}, store, &testutil.MockFrontendHub{}, &testutil.MockEuroscopeHub{})
	live := service.configForSession("AAAA", nil, nil, map[string]string{"01": "LOW_VIS"})
	exercise := service.configForSession("AAAA", nil, nil, nil)
	require.True(t, live.LvoActive)
	require.False(t, exercise.LvoActive)
	store.SetLvo("AAAA", true)
	require.False(t, store.ConfigForAirport("AAAA").LvoActive)
	require.NoError(t, service.RecalculateAirport(t.Context(), 1, "AAAA"))
	require.Error(t, service.RecalculateAirport(t.Context(), 1, "BBBB"))
	cdmService := newTestCdmService(NewClient(), &testutil.MockStripRepository{}, &testutil.MockSessionRepository{}, &testutil.MockControllerRepository{})
	cdmService.SetConfigProvider(store)
	require.True(t, cdmService.airportEnabled("AAAA"))
	require.False(t, cdmService.airportEnabled("BBBB"))
	cdmService.RegisterInboundSession(2, "LIVE", "BBBB")
	require.Empty(t, cdmService.inbound.airports)
}
