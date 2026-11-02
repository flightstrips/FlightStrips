package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestRouteRecoveryRematerializesInvalidSignedZeroCache(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	flight := state.Flights[0]
	query := navdata.RouteQuery{Version: geometry.version, Origin: "ENGM", Destination: "EKCH", FiledRoute: *flight.LatestObservation.FiledRoute, RunwayGroup: flight.SelectedRunwayGroup}
	legacy := navdata.RouteCandidate{Query: query, ResolverVersion: "airacnet-route-v4", SchemaVersion: navdata.CanonicalSchemaVersion}
	legacyKey, err := legacy.PersistenceKey()
	require.NoError(t, err)
	oldKey := string(legacyKey)
	flight.ActiveRouteKey = &oldKey
	current := legacy
	current.ResolverVersion = routeResolverVersion
	newKey, err := current.PersistenceKey()
	require.NoError(t, err)
	require.NotEqual(t, legacyKey, newKey, "recovery must avoid the immutable legacy cache entry")
	cache := &signedZeroRecoveryCache{terminalIdentityGeometry: geometry, legacyKey: legacyKey}
	service.deps.Geometry, service.deps.Materializer = cache, cache
	observation := *flight.LatestObservation
	position := *observation.Surveillance
	position.ObservedAt = &now
	observation.Surveillance, observation.ReconciledAt = &position, now

	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
	require.NoError(t, err)
	require.Equal(t, 1, cache.materializations)
	require.Equal(t, string(newKey), *updated.ActiveRouteKey)
	require.True(t, updated.Prediction.Publishable)
	require.NotNil(t, updated.Prediction.DistanceToGoNM)
	require.Equal(t, now, updated.Prediction.InputObservedAt)
}

type signedZeroRecoveryCache struct {
	terminalIdentityGeometry
	legacyKey        navdata.RouteKey
	materializations int
}

func (c *signedZeroRecoveryCache) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	if key == c.legacyKey {
		return navdata.RouteGeometry{}, &aman.DomainError{Class: aman.ErrorCorruptData, Message: "navigation cache: validate route geometry"}
	}
	return c.terminalIdentityGeometry.Route(ctx, key)
}

func (c *signedZeroRecoveryCache) MaterializeRoute(_ context.Context, query navdata.RouteQuery, resolver string) (navdata.RouteKey, error) {
	c.materializations++
	candidate := navdata.RouteCandidate{Query: query, ResolverVersion: resolver, SchemaVersion: navdata.CanonicalSchemaVersion}
	return candidate.PersistenceKey()
}

func TestRouteRecoveryRefreshesLegacyGeometryTimingAndAutomaticSlot(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	geometry.fixes[1].Position.LatitudeDeg, geometry.fixes[1].Position.LongitudeDeg = 55, 14
	geometry.fixes[2].Position.LatitudeDeg, geometry.fixes[2].Position.LongitudeDeg = 55, 16
	service.deps.Geometry, service.deps.Wind = geometry, unavailableWind{}
	initialAt := now.Add(-time.Minute)
	observation := *state.Flights[0].LatestObservation
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55, 12.01
	position.ObservedAt = &initialAt
	observation.Surveillance, observation.ReconciledAt = &position, initialAt
	observation.WakeCategory = stringPointer("M")
	initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, initialAt), observation, initialAt)
	require.NoError(t, err)
	require.NotNil(t, initial.Prediction.Calculation)
	require.Equal(t, "TESPI", initial.Prediction.Calculation.Legs[0].To)
	initial.RouteProgress.ObservedAt = nil // An aggregate persisted before this fix.
	initial.State = aman.StateStable
	initial.Prediction.OperationalTETA = now.Add(90 * time.Minute)
	initial.RawTETASamples = []aman.RawTETASample{{TETA: initial.Prediction.OperationalTETA, GeneratedAt: initialAt}}
	initial.Slot = &aman.Slot{Time: now.Add(92 * time.Minute), RunwayGroupID: *initial.SelectedRunwayGroup, Sequence: 1, Reason: "rate_wtc"}
	oldSlot, oldDistance := initial.Slot.Time, *initial.Prediction.DistanceToGoNM

	at := initialAt.Add(20 * time.Minute)
	position.LatitudeDegrees, position.LongitudeDegrees, position.ObservedAt = 55, 14.5, &at
	observation.ReconciledAt = at
	updated, err := service.reconcileFlight(t.Context(), state, initial, observation, at)
	require.NoError(t, err)
	require.Equal(t, "TNO", updated.Prediction.Calculation.Legs[0].To)
	require.Less(t, *updated.Prediction.DistanceToGoNM, oldDistance-50)
	require.Equal(t, at, updated.Prediction.InputObservedAt)
	require.Equal(t, at, *updated.RouteProgress.ObservedAt)
	require.Equal(t, aman.OperationalReasonRouteRecovery, updated.Prediction.OperationalReason)
	require.Equal(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA)
	state.Flights = []aman.AMANFlight{updated}
	service.resequence(&state, at)
	require.True(t, state.Flights[0].Slot.Time.Before(oldSlot))
}
