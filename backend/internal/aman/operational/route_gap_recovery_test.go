package operational

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestRepairedRouteResetsFilteringAndPreservesEveryFreeze(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeManual, aman.FreezeTMA, aman.FreezeSuperstable} {
		t.Run(string(freeze), func(t *testing.T) {
			service, state, now := recomputeFlightFixture(t)
			geometry := service.deps.Geometry.(terminalIdentityGeometry)
			geometry.route.Digest = "corrected-route"
			geometry.route.Coverage = navdata.CoveragePartial
			geometry.route.Unresolved = []string{"AMBIGUOUS_ROUTE_FIX:SPY"}
			geometry.route.Legs = append([]navdata.ProcedureLeg{{ID: navdata.RouteGapPrefix + "SPY", PathTerminator: navdata.PathUnsupported}}, geometry.route.Legs...)
			service.deps.Geometry = geometry
			flight := state.Flights[0]
			flight.FreezeReason = freeze
			oldETA := now.Add(90 * time.Minute)
			flight.Prediction.OperationalTETA = oldETA
			flight.RawTETASamples = []aman.RawTETASample{{TETA: oldETA, GeneratedAt: now.Add(-time.Minute)}}
			if freeze != aman.FreezeNone {
				flight.FrozenOperationalTETA = &oldETA
				at := now.Add(-time.Minute)
				flight.FrozenAt = &at
				if freeze != aman.FreezeManual {
					slot := *flight.Slot
					flight.FrozenSlot = &slot
				}
			}
			previousETA, previousSlot := flight.Prediction.RawTETA, *flight.Slot
			observation := *flight.LatestObservation
			position := *observation.Surveillance
			position.TrackTrueDegrees = repairPointer(30.0)
			observation.Surveillance = &position
			for sample := 1; sample <= 3; sample++ {
				at := now.Add(time.Duration(sample-1) * 10 * time.Second)
				position.ObservedAt = &at
				position.LatitudeDegrees, position.LongitudeDegrees = 55.01+float64(sample)*.001, 12.01+float64(sample)*.001
				observation.ReconciledAt = at
				updated, err := service.reconcileFlight(t.Context(), state, flight, observation, at)
				flight = updated
				if sample < 3 {
					require.ErrorContains(t, err, "ROUTE_RECOVERY_CONFIRMATION_PENDING")
					require.Equal(t, previousETA, flight.Prediction.RawTETA)
					require.EqualValues(t, sample, flight.RouteRecovery.Samples)
					if sample == 1 {
						bytes, marshalErr := json.Marshal(flight)
						require.NoError(t, marshalErr)
						require.NoError(t, json.Unmarshal(bytes, &flight))
						duplicate, duplicateErr := service.reconcileFlight(t.Context(), state, flight, observation, at)
						require.Error(t, duplicateErr)
						require.EqualValues(t, 1, duplicate.RouteRecovery.Samples)
						flight = duplicate
					}
				} else {
					require.NoError(t, err)
				}
			}
			require.Nil(t, flight.RouteRecovery)
			require.NotEqual(t, previousETA, flight.Prediction.RawTETA)
			require.Len(t, flight.RawTETASamples, 1, "obsolete smoothing samples must be discarded")
			require.Equal(t, previousSlot, *flight.Slot)
			if freeze == aman.FreezeNone {
				require.Equal(t, aman.OperationalReasonRouteRecovery, flight.Prediction.OperationalReason)
				require.Equal(t, flight.Prediction.RawTETA, flight.Prediction.OperationalTETA)
			} else {
				require.Equal(t, freeze, flight.FreezeReason)
				require.Equal(t, oldETA, flight.Prediction.OperationalTETA)
			}
		})
	}
}

type failingRepairMaterializer struct{ calls int }

func (m *failingRepairMaterializer) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	m.calls++
	return "", errors.New("AMBIGUOUS_ROUTE_FIX:SPY")
}

func TestRouteRepairRetriesAreBoundedAcrossReconciliationAndRestart(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	geometry.route.Legs = append(geometry.route.Legs, navdata.ProcedureLeg{ID: "OLD-CLOSING-LEG", PathTerminator: navdata.PathTF, ToFix: repairPointer(navdata.FixID("EKCH"))})
	service.deps.Geometry = geometry
	materializer := &failingRepairMaterializer{}
	service.deps.Materializer = materializer
	flight := state.Flights[0]
	observation := *flight.LatestObservation
	updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
	require.ErrorContains(t, err, "SPY")
	require.Equal(t, 1, materializer.calls)
	bytes, err := json.Marshal(updated)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bytes, &flight))
	for range 5 {
		flight, err = service.reconcileFlight(t.Context(), state, flight, observation, now.Add(10*time.Second))
		require.ErrorContains(t, err, "retry pending")
	}
	require.Equal(t, 1, materializer.calls)
	_, err = service.reconcileFlight(t.Context(), state, flight, observation, now.Add(time.Minute))
	require.ErrorContains(t, err, "SPY")
	require.Equal(t, 2, materializer.calls)
}

type amendmentRouteCache struct {
	terminalIdentityGeometry
	failure error
	calls   int
	queries []navdata.RouteQuery
}

func (c *amendmentRouteCache) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	route, err := c.terminalIdentityGeometry.Route(ctx, key)
	if key == "amended-route" {
		route.Digest = "amended-digest"
	}
	return route, err
}

func (c *amendmentRouteCache) MaterializeRoute(_ context.Context, query navdata.RouteQuery, _ string) (navdata.RouteKey, error) {
	c.calls++
	c.queries = append(c.queries, query)
	return "amended-route", c.failure
}

func TestFailedRouteAmendmentDoesNotReusePreviousQueryGeometry(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	cache := &amendmentRouteCache{terminalIdentityGeometry: service.deps.Geometry.(terminalIdentityGeometry), failure: errors.New("route resolver unavailable")}
	service.deps.Geometry, service.deps.Materializer = cache, cache
	flight := state.Flights[0]
	oldKey, oldQuery, oldPrediction := *flight.ActiveRouteKey, flight.ActiveRouteQueryKey, *flight.Prediction
	observation := *flight.LatestObservation
	observation.FiledRoute = stringPointer("DCT REVISED DCT TESPI") // Same feeder, different en-route geometry.
	observation.FlightPlan.Revision = repairPointer(flight.RouteProgress.FlightPlanRevision + 1)
	position := *observation.Surveillance
	observation.Surveillance = &position
	position.ObservedAt, observation.ReconciledAt = &now, now

	failed, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
	require.ErrorContains(t, err, "route resolver unavailable")
	require.Equal(t, 1, cache.calls)
	failed = applyUnavailablePrediction(failed, observation, now, err)
	bytes, err := json.Marshal(failed)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bytes, &flight))

	// A restarted reconciler must wait for the pending amendment rather than
	// accepting the old route under the amended query's identity.
	cache.failure = nil
	reconciledAt := now.Add(10 * time.Second)
	position.ObservedAt, observation.ReconciledAt = &reconciledAt, reconciledAt
	blocked, err := service.reconcileFlight(t.Context(), state, flight, observation, reconciledAt)
	require.ErrorContains(t, err, "retry pending")
	require.Equal(t, 1, cache.calls, "the amendment must retain its retry deadline")
	require.Equal(t, oldKey, *blocked.ActiveRouteKey)
	require.Equal(t, oldQuery, blocked.ActiveRouteQueryKey)
	pendingQuery, err := cache.queries[0].Key()
	require.NoError(t, err)
	require.Equal(t, string(pendingQuery), blocked.RouteRefreshQueryKey)
	require.Equal(t, oldPrediction.RawTETA, blocked.Prediction.RawTETA)
	require.Equal(t, oldPrediction.InputObservedAt, blocked.Prediction.InputObservedAt)
	require.Equal(t, oldPrediction.GeometryDigest, blocked.Prediction.GeometryDigest)

	retryAt := now.Add(routeRepairRetryEvery)
	position.ObservedAt, observation.ReconciledAt = &retryAt, retryAt
	repaired, err := service.reconcileFlight(t.Context(), state, blocked, observation, retryAt)
	require.NoError(t, err)
	require.Equal(t, 2, cache.calls)
	require.Equal(t, *observation.FiledRoute, cache.queries[1].FiledRoute)
	require.Equal(t, "amended-route", *repaired.ActiveRouteKey)
	expectedQuery, err := cache.queries[1].Key()
	require.NoError(t, err)
	require.Equal(t, string(expectedQuery), repaired.ActiveRouteQueryKey)
	require.NotEqual(t, oldQuery, repaired.ActiveRouteQueryKey)
	require.Equal(t, "amended-digest", repaired.Prediction.GeometryDigest)
	require.Equal(t, retryAt, repaired.Prediction.InputObservedAt)
	require.Nil(t, repaired.RouteRefreshAfter)
}

func repairPointer[T any](v T) *T { return &v }
