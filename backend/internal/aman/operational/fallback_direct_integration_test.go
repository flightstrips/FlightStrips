package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/routefact"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

type fallbackTrackingReader struct{}

func (fallbackTrackingReader) GetByCallsign(_ context.Context, session int32, callsign string) (*models.Strip, error) {
	return &models.Strip{Session: session, Callsign: callsign, TrackingController: "EKCH_A_APP"}, nil
}

type fallbackFactReconciler func(context.Context)

func (f fallbackFactReconciler) Reconcile(ctx context.Context) { f(ctx) }

func TestLiveDirectToCommitsAndRecalculatesFallbackOnlyArrival(t *testing.T) {
	for _, retained := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_filed_route", true: "with_obsolete_filed_route"}[retained], func(t *testing.T) {
			now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
			service, state, observation := fallbackFixture(t, now)
			geometry := service.deps.Geometry.(arrivalChangeGeometry)
			geometry.fixes[2].Position = navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 11.2}
			geometry.fixes[3].Position = navdata.Coordinate{LatitudeDeg: 55.3, LongitudeDeg: 12.8}
			for i := range geometry.fixes {
				geometry.fixes[i].Provenance = navdata.Provenance{SourceID: "fixture", SourceRevision: "test", ImportedAt: now.Add(-time.Hour), EffectiveFrom: geometry.version.EffectiveFrom, EffectiveUntil: geometry.version.EffectiveUntil}
			}
			service.deps.Geometry = geometry
			if retained {
				service.deps.Materializer = fixedNavigation{key: "old-filed-route"}
			}
			initial, err := service.reconcileFlight(t.Context(), state, newFlight(observation, now), observation, now)
			require.NoError(t, err)
			if retained {
				require.NotNil(t, initial.ActiveRouteKey)
				require.NotNil(t, initial.RouteProgress)
			} else {
				require.Nil(t, initial.ActiveRouteKey)
				require.Nil(t, initial.RouteProgress)
				require.Equal(t, aman.PredictionBasisFallback, initial.Prediction.Basis)
			}
			state.SessionID, state.Flights = 42, []aman.AMANFlight{initial}
			service.resequence(&state, now)
			originalSlot := *state.Flights[0].Slot
			originalETA := state.Flights[0].Prediction.RawTETA
			directAt := now.Add(time.Second)
			if retained {
				amendmentAt := now.Add(5 * time.Minute)
				amended := observation
				revision := *observation.FlightPlan.Revision + 1
				amended.FlightPlan.Revision, amended.FiledRoute = &revision, stringPointer("DCT AMENDED DCT TESPI")
				position := *observation.Surveillance
				position.LatitudeDegrees, position.LongitudeDegrees, position.ObservedAt = 55.35, 12.64, &amendmentAt
				amended.Surveillance, amended.ReconciledAt = &position, amendmentAt
				service.deps.Materializer = unavailableNavigation{}
				fallback, err := service.reconcileFlight(t.Context(), state, state.Flights[0], amended, amendmentAt)
				require.NoError(t, err)
				require.Equal(t, aman.PredictionBasisFallback, fallback.Prediction.Basis)
				require.NotNil(t, fallback.ActiveRouteKey)
				require.NotNil(t, fallback.RouteProgress)
				require.Equal(t, revision, fallback.FallbackRouteProgress.FlightPlanRevision)
				require.NotEqual(t, fallback.RouteProgress.FlightPlanRevision, fallback.FallbackRouteProgress.FlightPlanRevision)
				require.Equal(t, 1, fallback.FallbackRouteProgress.RejoinLegIndex, "the current terminal cursor has passed HOLD")
				state.Flights[0] = fallback
				state = cloneGapState(t, state)
				observation, directAt = amended, amendmentAt.Add(time.Second)
			}
			repository := &memoryRepository{has: true, state: state}
			replanCalls := 0
			replan := fallbackFactReconciler(func(ctx context.Context) {
				replanCalls++
				next := cloneGapState(t, repository.state)
				updated, err := service.reconcileFlight(ctx, next, next.Flights[0], observation, directAt)
				require.NoError(t, err)
				previousRevision := next.Revision
				next.Revision++
				next.GeneratedAt, next.Flights[0] = directAt, updated
				service.resequence(&next, directAt)
				service.refreshHoldingPlans(&next)
				_, err = repository.Commit(ctx, aman.StateCommit{ExpectedRevision: previousRevision, State: next})
				require.NoError(t, err)
			})
			dependencies := routefact.Dependencies{Repository: repository, Strips: fallbackTrackingReader{}, Geometry: geometry, Publisher: &recordingPublisher{}, Reconciler: replan, Now: func() time.Time { return directAt }, NewID: func() string { return "live-fallback-direct" }}
			live, err := routefact.New(dependencies)
			require.NoError(t, err)
			if retained {
				passed := "HOLD"
				require.Error(t, live.ReportDirectTo(t.Context(), 42, "EKCH", initial.Callsign, "EKCH_A_APP", &passed, directAt))
				require.Empty(t, repository.commits)
				require.Zero(t, replanCalls)
			}
			target := "END"
			require.NoError(t, live.ReportDirectTo(t.Context(), 42, "EKCH", initial.Callsign, "EKCH_A_APP", &target, directAt))
			updated := repository.state.Flights[0]
			require.Equal(t, "live-fallback-direct", updated.ActiveRouteFact.ID)
			require.Equal(t, target, updated.ActiveRouteFact.Fix)
			require.Equal(t, aman.PredictionBasisFallback, updated.Prediction.Basis)
			require.True(t, updated.Prediction.RawTETA.Before(originalETA.Add(-3*time.Minute)))
			require.Equal(t, []string{"END"}, updated.Prediction.RemainingRouteFixes)
			require.Nil(t, updated.Prediction.HoldingFixETA)
			require.Nil(t, updated.Prediction.HoldingPlan)
			if retained {
				require.Equal(t, initial.ActiveRouteKey, updated.ActiveRouteKey)
			} else {
				require.Nil(t, updated.ActiveRouteKey)
			}
			require.Equal(t, repository.state.Revision, updated.Slot.Revision)
			require.Equal(t, originalSlot.Revision, state.Flights[0].Slot.Revision, "the live fact commit must not mutate the source reservation")
			require.Len(t, repository.commits, 2)
			require.Equal(t, 1, replanCalls)
			require.NoError(t, repository.state.Validate())
			// Restart/replay uses the committed fact and terminal progress; it does
			// not need a filed route and must not commit the same command again.
			repository.state = cloneGapState(t, repository.state)
			restarted, err := routefact.New(dependencies)
			require.NoError(t, err)
			require.NoError(t, restarted.ReportDirectTo(t.Context(), 42, "EKCH", initial.Callsign, "EKCH_A_APP", &target, directAt))
			require.Len(t, repository.commits, 2)
			require.Equal(t, 1, replanCalls)
			offPath := "ORIGIN"
			require.Error(t, restarted.ReportDirectTo(t.Context(), 42, "EKCH", initial.Callsign, "EKCH_A_APP", &offPath, directAt))
			require.Len(t, repository.commits, 2)

		})
	}
}
