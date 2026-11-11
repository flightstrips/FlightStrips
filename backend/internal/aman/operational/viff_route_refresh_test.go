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

func TestFlightBasedRouteJoinsTESPIAtFiledTNOEndpoint(t *testing.T) {
	s, state, now := recomputeFlightFixture(t)
	geometry := s.deps.Geometry.(terminalIdentityGeometry)
	tno, after := navdata.FixID("TNO"), navdata.FixID("AFTER")
	geometry.route.Legs[0].ToFix = &tno
	geometry.path.Legs = append(geometry.path.Legs, navdata.ProcedureLeg{ID: "TNO-ONWARD", PathTerminator: navdata.PathTF, FromFix: &tno, ToFix: &after})
	geometry.fixes = append(geometry.fixes, navdata.Fix{ID: after, Position: navdata.Coordinate{LatitudeDeg: 55.3, LongitudeDeg: 12.3}})
	s.deps.Terminal.Paths[0].Fixes = []navdata.FixID{"TESPI", tno, after}
	cache := &amendmentRouteCache{terminalIdentityGeometry: geometry}
	source := &bulkRefreshMaterializer{cache}
	s.deps.Geometry, s.deps.Materializer = source, source
	obs := *state.Flights[0].LatestObservation
	obs.Callsign = "ICE212"
	obs.FiledRoute = stringPointer("AAL M725 ADSEN DCT TNO")

	f, err := s.reconcileFlight(t.Context(), state, newFlight(obs, now), obs, now)

	require.NoError(t, err)
	require.Len(t, cache.queries, 1)
	require.NotNil(t, cache.queries[0].TerminalEntry)
	require.Equal(t, tno, *cache.queries[0].TerminalEntry)
	require.Equal(t, "TESPI", *f.SelectedSTARFamily)
	require.Equal(t, "TNO", *f.SelectedFeederFix)
	require.NotNil(t, f.Prediction)
	require.True(t, f.Prediction.Publishable)
	require.Equal(t, []string{"TNO", "AFTER"}, f.Prediction.RemainingRouteFixes)
	require.True(t, f.Prediction.OperationalTETA.After(now))
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
