package operational

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/viff"
	"github.com/stretchr/testify/require"
)

type starReassignmentSource struct {
	runwaySTARGeometry
	resolver *viff.Adapter
	routes   map[navdata.RouteKey]navdata.RouteGeometry
}

func (*starReassignmentSource) RouteRefreshInterval() time.Duration { return viff.RefreshInterval }

func (s *starReassignmentSource) MaterializeRoute(ctx context.Context, q navdata.RouteQuery, _ string) (navdata.RouteKey, error) {
	route, err := s.resolver.Resolve(ctx, q)
	if err != nil {
		return "", err
	}
	key := navdata.RouteKey(route.Digest)
	s.routes[key] = route
	return key, nil
}

func (s *starReassignmentSource) Route(_ context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	return s.routes[key], nil
}

func TestEuroScopeSTARReassignmentUsesLocalPathWhileVIFFRetainsOldEntry(t *testing.T) {
	now := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	oldEntry := navdata.FixID("OLDENTRY")
	geometry.fixes = append(geometry.fixes, navdata.Fix{ID: oldEntry, Position: navdata.Coordinate{LatitudeDeg: 55.08, LongitudeDeg: 12.14}})
	oldPath := geometry.path
	oldPath.Feeder, oldPath.STARFamily = "MONAK", "MONAK"
	oldPath.FeederFix = "HOLD"
	oldPath.Legs = append([]navdata.ProcedureLeg(nil), oldPath.Legs...)
	oldPath.Legs[0].FromFix = &oldEntry
	geometryReader := runwaySTARGeometry{arrivalChangeGeometry: geometry, paths: []navdata.TerminalPath{oldPath, geometry.path}}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`[{"callsign":"SAS123","departure":"ESSA","arrival":"EKCH","path":[{"name":"ESSA","lat":55,"lon":12},{"name":"OLDENTRY","lat":55.08,"lon":12.14},{"name":"OLD-STAR"},{"name":"EKCH"}]}]`))
	}))
	defer server.Close()
	resolver, err := viff.New(viff.Config{BaseURL: server.URL, References: geometryReader, Now: func() time.Time { return now }})
	require.NoError(t, err)
	source := &starReassignmentSource{runwaySTARGeometry: geometryReader, resolver: resolver, routes: make(map[navdata.RouteKey]navdata.RouteGeometry)}
	service.deps.Geometry, service.deps.Materializer = source, source
	observation.AssignedSTAR = stringPointer("MONAK1A")
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.True(t, flight.Prediction.Publishable)
	oldKey := *flight.ActiveRouteKey
	// The controller changes only the STAR, at the same radar timestamp and
	// flight-plan revision, before either the feed or route-refresh is due.
	protected := gapCommandFlight("SAS123", "ARRIVAL-22", now.Add(20*time.Minute), 1, aman.StateStable, aman.FreezeManual)
	flight.Slot, flight.State, flight.FreezeReason = protected.Slot, protected.State, protected.FreezeReason
	frozenETA := flight.Prediction.OperationalTETA
	flight.FrozenAt, flight.FrozenOperationalTETA = &now, &frozenETA
	observation.AssignedSTAR = stringPointer("TESPI1A")
	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
	require.NoError(t, err)
	require.Equal(t, 1, requests, "the new STAR must work with the unchanged cached vIFF response")
	require.NotEqual(t, oldKey, *updated.ActiveRouteKey)
	require.Equal(t, "TESPI", updated.STARFamilyIdentity())
	require.Equal(t, "END", *updated.SelectedFeederFix)
	require.True(t, updated.Prediction.Publishable)
	require.Equal(t, aman.DataFresh, updated.DataStatus)
	require.False(t, updated.ArrivalPathChanged)
	require.Equal(t, flight.Slot, updated.Slot)
	require.Equal(t, aman.FreezeManual, updated.FreezeReason)
	require.NotContains(t, updated.Prediction.RemainingRouteFixes, string(oldEntry))
	require.Contains(t, updated.Prediction.RemainingRouteFixes, "ENTRY")
	require.Contains(t, updated.Prediction.RemainingRouteFixes, "HOLD")
	require.Contains(t, updated.Prediction.RemainingRouteFixes, "END")
	_, available := aman.GainLossPrediction(updated)
	require.True(t, available)
	// Reverting the controller assignment also works without a feed refresh.
	observation.AssignedSTAR = stringPointer("MONAK1A")
	restored, err := service.reconcileFlight(t.Context(), state, updated, observation, now)
	require.NoError(t, err)
	require.Equal(t, oldKey, *restored.ActiveRouteKey)
	require.Equal(t, "MONAK", restored.STARFamilyIdentity())
	require.Equal(t, "HOLD", *restored.SelectedFeederFix)
	require.True(t, restored.Prediction.Publishable)
	require.Equal(t, 1, requests)
}
