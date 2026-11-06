package metar

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type weatherHub struct{ values map[int32]string }

func (h *weatherHub) SendAtisUpdate(session int32, metar, arr, dep string) {
	h.values[session] = metar + ":" + arr + ":" + dep
}

type weatherAuth struct{}

func (weatherAuth) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("123", 0, nil), nil
}

func TestSharedWeatherCountsAndLastGoodAcrossSessionsAndBrowserReads(t *testing.T) {
	var metars, atises atomic.Int32
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/atis" {
			atises.Add(1)
			if fail.Load() {
				w.WriteHeader(503)
				return
			}
			_, _ = fmt.Fprintf(w, `[{"callsign":"AAAA_ATIS","atis_code":"C","frequency":"118.000","text_atis":["shared"],"last_updated":%q}]`, time.Now().UTC().Format(time.RFC3339))
			return
		}
		metars.Add(1)
		if fail.Load() {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(r.URL.Path + " METAR"))
	}))
	defer upstream.Close()
	hub := &weatherHub{values: make(map[int32]string)}
	p := NewPoller(&testutil.MockSessionRepository{ListFn: func(context.Context) ([]*models.Session, error) {
		return []*models.Session{{ID: 1, Airport: "AAAA", Name: "LIVE"}, {ID: 2, Airport: "AAAA", Name: "SWEATBOX_1"}, {ID: 3, Airport: "BBBB", Name: "LIVE"}}, nil
	}}, hub)
	p.metarBaseURL, p.atisDataURL, p.httpClient = upstream.URL, upstream.URL+"/atis", upstream.Client()
	p.poll(t.Context())
	require.EqualValues(t, 2, metars.Load())
	require.EqualValues(t, 1, atises.Load())
	require.Equal(t, hub.values[1], hub.values[2])
	before := p.GetATIS("AAAA", false)
	require.False(t, before.Stale)
	before.Text[0] = "mutated"
	mux := http.NewServeMux()
	RegisterRoutes(mux, weatherAuth{}, p)
	for range 40 {
		response := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/weather/atis?airport=AAAA", nil)
		request.Header.Set("Authorization", "Bearer test")
		mux.ServeHTTP(response, request)
		require.Equal(t, 200, response.Code)
		require.Contains(t, response.Body.String(), "shared")
	}
	require.EqualValues(t, 1, atises.Load(), "tabs read a cache, never an upstream feed")
	fail.Store(true)
	p.poll(t.Context())
	require.True(t, p.GetATIS("AAAA", false).Stale)
	require.Equal(t, []string{"shared"}, p.GetATIS("AAAA", false).Text)
	require.Equal(t, "/AAAA METAR:C:C", hub.values[1])
	require.Equal(t, hub.values[1], hub.values[2])
}
