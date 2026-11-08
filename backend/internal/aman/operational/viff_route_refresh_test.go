package operational

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

type bulkRefreshMaterializer struct{ *amendmentRouteCache }

func (*bulkRefreshMaterializer) RouteRefreshInterval() time.Duration { return 30 * time.Second }

func (m *bulkRefreshMaterializer) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	return m.terminalIdentityGeometry.Route(ctx, key)
}

func TestFlightBasedRoutesRefreshPeriodicallyAndPassFlightIdentity(t *testing.T) {
	s, state, now := recomputeFlightFixture(t)
	cache := &amendmentRouteCache{terminalIdentityGeometry: s.deps.Geometry.(terminalIdentityGeometry)}
	source := &bulkRefreshMaterializer{cache}
	s.deps.Geometry, s.deps.Materializer = source, source
	f := state.Flights[0]
	f.FreezeReason = aman.FreezeNone
	obs := *f.LatestObservation
	var err error
	f, err = s.reconcileFlight(t.Context(), state, f, obs, now)
	require.NoError(t, err)
	require.Equal(t, 1, cache.calls)
	require.Equal(t, obs.Callsign, cache.queries[0].Callsign)
	require.NotNil(t, cache.queries[0].TerminalEntry)
	require.EqualValues(t, "TESPI", *cache.queries[0].TerminalEntry)
	require.Equal(t, now.Add(30*time.Second), *f.RouteRefreshAfter)
	f, err = s.reconcileFlight(t.Context(), state, f, obs, now.Add(10*time.Second))
	require.NoError(t, err)
	require.Equal(t, 1, cache.calls)
	_, err = s.reconcileFlight(t.Context(), state, f, obs, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, 2, cache.calls)
}

func TestFlightBasedSourceRejectsAmendmentEvenWithoutRevisionChange(t *testing.T) {
	s, state, now := recomputeFlightFixture(t)
	cache := &amendmentRouteCache{terminalIdentityGeometry: s.deps.Geometry.(terminalIdentityGeometry), failure: errors.New("vIFF has not processed amendment")}
	source := &bulkRefreshMaterializer{cache}
	s.deps.Geometry, s.deps.Materializer = source, source
	f := state.Flights[0]
	f.FreezeReason = aman.FreezeNone
	obs := *f.LatestObservation
	obs.FiledRoute = stringPointer("DCT REVISED DCT TESPI")
	_, err := s.reconcileFlight(t.Context(), state, f, obs, now)
	require.ErrorContains(t, err, "not processed amendment")
	require.Equal(t, 1, cache.calls)
}

func TestFlightBasedRefreshFailureRetainsUsableSameQueryGeometry(t *testing.T) {
	s, state, now := recomputeFlightFixture(t)
	cache := &amendmentRouteCache{terminalIdentityGeometry: s.deps.Geometry.(terminalIdentityGeometry)}
	source := &bulkRefreshMaterializer{cache}
	s.deps.Geometry, s.deps.Materializer = source, source
	f := state.Flights[0]
	f.FreezeReason = aman.FreezeNone
	obs := *f.LatestObservation
	f, err := s.reconcileFlight(t.Context(), state, f, obs, now)
	require.NoError(t, err)
	oldKey := *f.ActiveRouteKey
	cache.failure = errors.New("vIFF offline")
	f, err = s.reconcileFlight(t.Context(), state, f, obs, now.Add(30*time.Second))
	require.NoError(t, err)
	require.Equal(t, oldKey, *f.ActiveRouteKey)
	require.Equal(t, 2, cache.calls)
	_, err = s.reconcileFlight(t.Context(), state, f, obs, now.Add(40*time.Second))
	require.NoError(t, err)
	require.Equal(t, 2, cache.calls)
}

func TestFlightBasedRefreshDoesNotReplaceTMAFrozenGeometry(t *testing.T) {
	s, state, now := recomputeFlightFixture(t)
	cache := &amendmentRouteCache{terminalIdentityGeometry: s.deps.Geometry.(terminalIdentityGeometry), failure: errors.New("unexpected route refresh")}
	source := &bulkRefreshMaterializer{cache}
	s.deps.Geometry, s.deps.Materializer = source, source
	f := state.Flights[0]
	f.FreezeReason = aman.FreezeTMA
	f.FrozenAt = &now
	frozenETA := f.Prediction.OperationalTETA
	f.FrozenOperationalTETA = &frozenETA
	f.FrozenSlot = f.Slot
	obs := *f.LatestObservation
	_, err := s.reconcileFlight(t.Context(), state, f, obs, now)
	require.NoError(t, err)
	require.Zero(t, cache.calls)
}
