package services

import (
	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/vatsim"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrivalAdvisoryRecoveryPreservesAcceptedStage(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	for _, accepted := range []string{"", StageEstimated, StageAssigned, StageConfirmed} {
		for _, report := range []struct {
			name     string
			altitude *int32
			stage    string
		}{
			{name: "missing_altitude", stage: StageEstimated},
			{name: "high_altitude", altitude: altPtr(20000), stage: StageEstimated},
			{name: "approaching", altitude: altPtr(8000), stage: StageAssigned},
			{name: "final", altitude: altPtr(2000), stage: StageConfirmed},
		} {
			name := accepted
			if name == "" {
				name = "new_arrival"
			}
			t.Run(name+"/"+report.name, func(t *testing.T) {
				lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
				clock.set(time.Now().UTC().Truncate(time.Second))
				seedTestArrivalStrip(t, queries, session, "SAS940")
				eta := clock.current().Add(time.Minute)
				setArrivalETA(t, strips, session, "SAS940", eta)
				if report.altitude != nil {
					_, err := strips.UpdateAircraftPosition(ctx, session, "SAS940", posPtr(55.8), posPtr(12.642625), report.altitude, "FINAL", nil)
					require.NoError(t, err)
				}
				var expiresAt *time.Time
				if accepted != "" {
					expiry := clock.current().Add(25 * time.Minute)
					expiresAt = &expiry
					require.NoError(t, lifecycle.recordDisplacedArrivalAdvisory(ctx, models.StandAssignment{
						SessionID: session, Callsign: "SAS940", Direction: "ARRIVAL", Stand: "A1",
						Stage: accepted, Source: "AUTOMATIC", ETA: &eta, ExpiresAt: expiresAt,
					}, "physically displaced: no capacity"))
				}
				// A failed retry must leave the non-occupying record untouched.
				var blocks []*models.StandBlock
				for _, stand := range []string{"A1", "A2"} {
					block := &models.StandBlock{SessionID: session, Stand: stand, BlockType: "MANUAL", Source: "CONTROLLER", Manual: true}
					require.NoError(t, allocations.CreateManualBlock(ctx, "EKCH", block))
					blocks = append(blocks, block)
				}
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS940"), arrivalFlight("SAS940", 1)))
				if accepted != "" {
					pending, err := assignments.GetAssignment(ctx, session, "SAS940")
					require.NoError(t, err)
					require.Equal(t, accepted, pending.Stage)
					require.Empty(t, pending.Stand)
					require.NotNil(t, pending.ConflictReason)
				}
				require.Empty(t, loadStrip(t, strips, session, "SAS940").Stand)
				_, err := allocations.DeleteManualBlock(ctx, session, blocks[1].ID, blocks[1].Version)
				require.NoError(t, err)
				var published []StandAllocationResult
				allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
					published = append(published, result)
					return nil
				})
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS940"), arrivalFlight("SAS940", 2)))
				recovered, err := assignments.GetAssignment(ctx, session, "SAS940")
				require.NoError(t, err)
				wantStage := report.stage
				if accepted == StageConfirmed || (accepted == StageAssigned && wantStage == StageEstimated) {
					wantStage = accepted
				}
				assert.Equal(t, wantStage, recovered.Stage)
				assert.Equal(t, "A2", recovered.Stand)
				assert.Nil(t, recovered.ConflictReason)
				if expiresAt == nil {
					assert.Nil(t, recovered.ExpiresAt)
				} else {
					require.NotNil(t, recovered.ExpiresAt)
					assert.True(t, expiresAt.Equal(*recovered.ExpiresAt), "recovery preserves the accepted retention deadline")
				}
				operational := wantStage != StageEstimated
				if operational {
					assert.Equal(t, strp("A2"), loadStrip(t, strips, session, "SAS940").Stand)
				} else {
					assert.Empty(t, loadStrip(t, strips, session, "SAS940").Stand)
				}
				require.Len(t, published, 1)
				assert.Equal(t, wantStage, published[0].Assignment.Stage)
				assert.Equal(t, operational, published[0].StandChanged)
				assert.Equal(t, operational, published[0].NotifyEuroscope)
			})
		}
	}
}

func TestArrivalEstimateKeepsStandDespiteBlocks(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "active"}[active], func(t *testing.T) {
			lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			clock.set(time.Now().UTC())
			seedTestArrivalStrip(t, queries, session, "SAS910")
			seedTestArrivalStrip(t, queries, session, "SAS911")
			setArrivalETA(t, strips, session, "SAS910", clock.current().Add(45*time.Minute))
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
			estimate, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			actions := NewStandActionService(allocations, assignments, strips, nil, nil, nil)
			_, err = actions.AssignManually(ctx, session, "EKCH", "GND", "SAS911", estimate.Stand, 0)
			require.NoError(t, err)
			expiresAt := clock.current().Add(-time.Minute)
			if active {
				expiresAt = clock.current().Add(time.Hour)
			}
			require.NoError(t, assignments.CreateBlock(ctx, &models.StandBlock{
				SessionID: session, Stand: estimate.Stand, BlockType: "MANUAL", Source: "CONTROLLER", Manual: true, ExpiresAt: &expiresAt,
			}))
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 2)))
			updated, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			assert.Equal(t, StageEstimated, updated.Stage)
			// ESTIMATED stands are only re-evaluated at promotion to ASSIGNED.
			assert.Equal(t, estimate.Stand, updated.Stand)
		})
	}
}

func TestArrivalAirportOperationalStandResolvesOnlyFreeStandWarning(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, releaseBeforeLanding := range []bool{false, true} {
		t.Run(map[bool]string{false: "blocked_then_released", true: "already_free"}[releaseBeforeLanding], func(t *testing.T) {
			lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS910")
			seedTestArrivalStrip(t, queries, session, "SAS911")
			setArrivalETA(t, strips, session, "SAS910", clock.current().Add(45*time.Minute))
			_, err := strips.UpdateAircraftPosition(ctx, session, "SAS910", posPtr(55.8), posPtr(12.642625), altPtr(8000), "FINAL", nil)
			require.NoError(t, err)
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
			operational, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			require.Equal(t, StageAssigned, operational.Stage, "the airport-area lock applies to an operational assignment")
			actions := NewStandActionService(allocations, assignments, strips, nil, nil, nil)
			blocker, err := actions.Override(ctx, session, "EKCH", "GND", "SAS911", operational.Stand, "explicit controller overlap", 0)
			require.NoError(t, err)
			require.NoError(t, lifecycle.recordArrivalPromotionConflict(ctx, operational, operational.ETA, operational.ETASource))
			if releaseBeforeLanding {
				require.NoError(t, allocations.ReleaseAssignment(ctx, &blocker.Assignment))
			}
			_, err = strips.UpdateAircraftPosition(ctx, session, "SAS910", posPtr(55.6181), posPtr(12.6561), altPtr(0), "TAXI", nil)
			require.NoError(t, err)
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 2)))
			confirmed, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			assert.Equal(t, StageConfirmed, confirmed.Stage)
			assert.Equal(t, operational.Stand, confirmed.Stand)
			assert.Equal(t, operational.Stand, *loadStrip(t, strips, session, "SAS910").Stand)
			if !releaseBeforeLanding {
				require.NotNil(t, confirmed.ConflictReason)
				assert.Equal(t, arrivalPromotionConflictReason, *confirmed.ConflictReason)
				require.NoError(t, allocations.ReleaseAssignment(ctx, &blocker.Assignment))
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 3)))
				confirmed, err = assignments.GetAssignment(ctx, session, "SAS910")
				require.NoError(t, err)
			}
			assert.Nil(t, confirmed.ConflictReason)
			assert.Equal(t, operational.Stand, confirmed.Stand)
		})
	}
}

func TestManualEstimatedArrivalPromotionWritesPinnedStripStand(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, stage := range []string{StageAssigned, StageConfirmed} {
		t.Run(stage, func(t *testing.T) {
			lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS910")
			eta := clock.current().Add(time.Minute)
			setArrivalETA(t, strips, session, "SAS910", eta)
			estimate := &models.StandAssignment{SessionID: session, Callsign: "SAS910", Stand: "A2", Direction: "ARRIVAL",
				Stage: StageEstimated, Manual: true, Source: "MANUAL_OVERRIDE", ETA: &eta}
			require.NoError(t, assignments.CreateAssignment(ctx, estimate))
			require.Nil(t, loadStrip(t, strips, session, "SAS910").Stand)
			altitude := int32(8000)
			if stage == StageConfirmed {
				altitude = 2000
			}
			_, err := strips.UpdateAircraftPosition(ctx, session, "SAS910", posPtr(55.8), posPtr(12.642625), &altitude, "FINAL", nil)
			require.NoError(t, err)
			var published []StandAllocationResult
			allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
				published = append(published, result)
				return nil
			})
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
			promoted, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			assert.Equal(t, stage, promoted.Stage)
			assert.True(t, promoted.Manual)
			assert.Equal(t, "MANUAL_OVERRIDE", promoted.Source)
			assert.Equal(t, "A2", promoted.Stand)
			strip := loadStrip(t, strips, session, "SAS910")
			require.NotNil(t, strip.Stand)
			assert.Equal(t, "A2", *strip.Stand)
			require.Len(t, published, 1)
			assert.True(t, published[0].NotifyEuroscope)
			assert.Equal(t, promoted.Version, published[0].Assignment.Version)
		})
	}
}

func TestArrivalPromotionRetriesWhenAMANPublishesAfterOrdering(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, currentStage := range []string{StageEstimated, StageAssigned} {
		t.Run(currentStage, func(t *testing.T) {
			ctx := t.Context()
			lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			const callsign = "SAS910"
			seedTestArrivalStrip(t, queries, session, callsign)
			now := clock.current()
			setArrivalETA(t, strips, session, callsign, now)
			require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
				SessionID: session, Callsign: callsign, Stand: "A1", Direction: "ARRIVAL",
				Stage: currentStage, ETA: &now, Source: "MANUAL", Manual: currentStage == StageAssigned,
			}))

			latestETA := now.Add(30 * time.Minute)
			ordered := &arrivalPriorityRefresh{
				ArrivalLifecycleService: lifecycle,
				publish: func() {
					_, err := strips.UpdateArrivalETA(ctx, session, callsign, models.ArrivalETA{
						Time: latestETA, Source: compatibility.ArrivalETASource, CalculatedAt: now,
					})
					require.NoError(t, err)
				},
			}
			source := vatsim.NewSnapshotReplaySource()
			feed := fmt.Sprintf(`{
				"general": {"update_timestamp": %q},
				"pilots": [{
					"cid": 1, "callsign": %q, "last_updated": %q,
					"latitude": 55, "longitude": 10, "altitude": 2000, "groundspeed": 30,
					"flight_plan": {"departure": "ESSA", "arrival": "EKCH", "aircraft_short": "A320", "revision_id": 1}
				}]
			}`, now.Format(time.RFC3339), callsign, now.Format(time.RFC3339))
			require.NoError(t, source.Load(strings.NewReader(feed), now))
			sessions := postgres.NewSessionRepository(pool)
			departures, err := NewDepartureLifecycleService(allocations, assignments, strips, sessions, lifecycle.stands, nil, nil,
				sat.NewAirportCountryRegistry(), WithDepartureLifecycleClock(clock.current))
			require.NoError(t, err)
			reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
				Cache: source, Sessions: sessions, Strips: strips, Assignments: assignments,
				DepartureLifecycle: departures, ArrivalLifecycle: ordered, Notifier: satHistoryNotifier{},
			}, time.Second, vatsim.WithClock(clock.current), vatsim.WithLegacyArrivalETAWriter(false))
			require.NoError(t, err)

			var published []StandAllocationResult
			allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
				published = append(published, result)
				return nil
			})
			require.NoError(t, reconciler.ReconcileSession(ctx, session))
			current, err := assignments.GetAssignment(ctx, session, callsign)
			require.NoError(t, err)
			require.Equal(t, currentStage, current.Stage, "a rejected promotion must not become operational on a stale ETA")
			require.GreaterOrEqual(t, len(ordered.priorities), 2)
			assert.Equal(t, arrivalStagePriority(StageConfirmed), ordered.priorities[0])
			assert.Equal(t, arrivalStagePriority(currentStage), ordered.priorities[1])
			for _, result := range published {
				assert.False(t, result.NotifyEuroscope)
			}
		})
	}
}

type arrivalPriorityRefresh struct {
	*ArrivalLifecycleService
	publish    func()
	priorities []int
}

func TestDisplacedArrivalUsesCurrentStripETA(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, source := range []string{"FILED", compatibility.ArrivalETASource, "expired AMAN", "missing"} {
		t.Run(source, func(t *testing.T) {
			ctx := t.Context()
			lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS910")
			seedTestArrivalStrip(t, queries, session, "SAS911")
			oldETA, currentETA := clock.current().Add(time.Hour), clock.current().Add(90*time.Minute)
			var wantETA *time.Time
			var wantSource *string
			if source != "missing" {
				eta := models.ArrivalETA{Time: currentETA, Source: source, CalculatedAt: clock.current()}
				wantETA, wantSource = &currentETA, &source
				if source == "expired AMAN" {
					eta.Source = compatibility.ArrivalETASource
					eta.CalculatedAt = clock.current().Add(-compatibility.FreshFor - time.Minute)
					wantETA, wantSource = nil, nil
				}
				_, err := strips.UpdateArrivalETA(ctx, session, "SAS910", eta)
				require.NoError(t, err)
			}
			require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
				SessionID: session, Callsign: "SAS911", Stand: "A1", Direction: "ARRIVAL",
				Stage: StageConfirmed, Manual: true, Source: "MANUAL", ETA: &currentETA,
			}))
			require.NoError(t, lifecycle.reassignDisplacedArrival(ctx, models.StandAssignment{
				SessionID: session, Callsign: "SAS910", Stand: "A1", Direction: "ARRIVAL",
				Stage: StageAssigned, Source: "AUTOMATIC", ETA: &oldETA, ETASource: strp("FILED"),
			}))
			relocated, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			require.Equal(t, "A2", relocated.Stand, "the old 11:00 ETA would incorrectly select A1")
			if wantETA == nil {
				require.Nil(t, relocated.ETA)
			} else {
				require.NotNil(t, relocated.ETA)
				require.True(t, wantETA.Equal(*relocated.ETA))
			}
			require.Equal(t, wantSource, relocated.ETASource)
		})
	}
}

func TestDisplacedArrivalRetainsAdvisoryWhenETAChangesAfterTakeover(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
	for _, callsign := range []string{"SAS910", "SAS911"} {
		seedTestArrivalStrip(t, queries, session, callsign)
	}
	now := clock.current()
	_, err := pool.Exec(ctx, "UPDATE strips SET vatsim_seen_at=$1 WHERE session=$2 AND callsign='SAS910'", now, session)
	require.NoError(t, err)
	require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
		SessionID: session, Callsign: "SAS910", Stand: "A1", Direction: "ARRIVAL",
		Stage: StageAssigned, Source: "AUTOMATIC", ETA: &now,
	}))
	_, err = strips.UpdateGroundState(ctx, session, "SAS911", strp("PARK"), "ARR_HIDDEN", nil)
	require.NoError(t, err)
	source := vatsim.NewSnapshotReplaySource()
	feed := fmt.Sprintf(`{
		"general": {"update_timestamp": %q},
		"pilots": [{"cid": 2, "callsign": "SAS911",
			"latitude": 55.6285306, "longitude": 12.642625, "altitude": 0, "groundspeed": 0,
			"flight_plan": {"departure": "ESSA", "arrival": "EKCH", "aircraft_short": "A320", "revision_id": 1}}]
	}`, now.Format(time.RFC3339))
	require.NoError(t, source.Load(strings.NewReader(feed), now))
	sessions := postgres.NewSessionRepository(pool)
	departures, err := NewDepartureLifecycleService(allocations, assignments, strips, sessions, lifecycle.stands, nil, nil,
		sat.NewAirportCountryRegistry(), WithDepartureLifecycleClock(clock.current))
	require.NoError(t, err)
	reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
		Cache: source, Sessions: sessions, Strips: strips, Assignments: assignments,
		DepartureLifecycle: departures, ArrivalLifecycle: lifecycle, Notifier: satHistoryNotifier{},
	}, time.Second, vatsim.WithClock(clock.current), vatsim.WithLegacyArrivalETAWriter(false))
	require.NoError(t, err)
	takeovers := 0
	allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		if len(result.RemovedAssignments) > 0 {
			takeovers++
			_, err := strips.UpdateArrivalETA(ctx, session, "SAS911", models.ArrivalETA{
				Time: now.Add(time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now,
			})
			require.NoError(t, err)
		}
		return nil
	})
	require.NoError(t, reconciler.ReconcileSession(ctx, session))
	require.Equal(t, 1, takeovers, "the committed takeover must never be replayed")
	parked, err := assignments.GetAssignment(ctx, session, "SAS911")
	require.NoError(t, err)
	require.Equal(t, "A1", parked.Stand)
	advisory, err := assignments.GetAssignment(ctx, session, "SAS910")
	require.NoError(t, err)
	require.Empty(t, advisory.Stand)
	require.Equal(t, StageAssigned, advisory.Stage)
	require.NotNil(t, advisory.ConflictReason)
	require.Contains(t, *advisory.ConflictReason, "timing changed")
	actions := NewStandActionService(allocations, assignments, strips, nil, nil, nil)
	recovered, err := actions.AssignManually(ctx, session, "EKCH", "APP", "SAS910", "A2", advisory.Version, &advisory.ID)
	require.NoError(t, err)
	require.Equal(t, "A2", recovered.Assignment.Stand)
}

func TestArrivalPromotionRetriesWhenCompetingETAChangesAfterOrdering(t *testing.T) {
	t.Run("earlier competing ETA", func(t *testing.T) {
		testArrivalPromotionAfterOrderChange(t, false)
	})
	t.Run("competing strip deleted", func(t *testing.T) {
		testArrivalPromotionAfterOrderChange(t, true)
	})
}

func testArrivalPromotionAfterOrderChange(t *testing.T, deleteCompetitor bool) {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
	require.NoError(t, allocations.CreateManualBlock(ctx, "EKCH", &models.StandBlock{
		SessionID: session, Stand: "A2", BlockType: "MANUAL", Source: "CONTROLLER", Manual: true,
	}))
	now := clock.current()
	for _, tc := range []struct {
		callsign string
		eta      time.Time
	}{
		{callsign: "SAS910", eta: now.Add(10 * time.Minute)},
		{callsign: "SAS911", eta: now.Add(45 * time.Minute)},
	} {
		seedTestArrivalStrip(t, queries, session, tc.callsign)
		setArrivalETA(t, strips, session, tc.callsign, tc.eta)
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: tc.callsign, Stand: "A1", Direction: "ARRIVAL",
			Stage: StageEstimated, ETA: &tc.eta, Source: "AUTOMATIC",
		}))
	}

	ordered := &arrivalPriorityRefresh{
		ArrivalLifecycleService: lifecycle,
		publish: func() {
			if deleteCompetitor {
				require.NoError(t, strips.Delete(ctx, session, "SAS911"))
				return
			}
			_, err := strips.UpdateArrivalETA(ctx, session, "SAS911", models.ArrivalETA{
				Time: now.Add(time.Minute), Source: compatibility.ArrivalETASource, CalculatedAt: now,
			})
			require.NoError(t, err)
		},
	}
	source := vatsim.NewSnapshotReplaySource()
	feed := fmt.Sprintf(`{
		"general": {"update_timestamp": %q},
		"pilots": [
			{"cid": 1, "callsign": "SAS910", "latitude": 55.8, "longitude": 12.642625, "altitude": 2000, "groundspeed": 30,
			 "flight_plan": {"departure": "ESSA", "arrival": "EKCH", "aircraft_short": "A320", "revision_id": 1}},
			{"cid": 2, "callsign": "SAS911", "latitude": 55, "longitude": 10, "altitude": 2000, "groundspeed": 30,
			 "flight_plan": {"departure": "ESSA", "arrival": "EKCH", "aircraft_short": "A320", "revision_id": 1}}
		]
	}`, now.Format(time.RFC3339))
	require.NoError(t, source.Load(strings.NewReader(feed), now))
	sessions := postgres.NewSessionRepository(pool)
	departures, err := NewDepartureLifecycleService(allocations, assignments, strips, sessions, lifecycle.stands, nil, nil,
		sat.NewAirportCountryRegistry(), WithDepartureLifecycleClock(clock.current))
	require.NoError(t, err)
	reconciler, err := vatsim.NewReconciler(vatsim.ReconcilerDependencies{
		Cache: source, Sessions: sessions, Strips: strips, Assignments: assignments,
		DepartureLifecycle: departures, ArrivalLifecycle: ordered, Notifier: satHistoryNotifier{},
	}, time.Second, vatsim.WithClock(clock.current), vatsim.WithLegacyArrivalETAWriter(false))
	require.NoError(t, err)

	var operational []string
	allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		if result.NotifyEuroscope {
			operational = append(operational, result.Assignment.Callsign)
		}
		return nil
	})
	require.NoError(t, reconciler.ReconcileSession(ctx, session))
	waiting, err := assignments.GetAssignment(ctx, session, "SAS910")
	require.NoError(t, err)
	if deleteCompetitor {
		assert.Equal(t, StageAssigned, waiting.Stage)
		assert.Equal(t, "A1", waiting.Stand)
		assert.Equal(t, []string{"SAS910"}, operational)
		_, err := strips.GetByCallsign(ctx, session, "SAS911")
		require.Error(t, err)
		return
	}
	assert.Equal(t, StageEstimated, waiting.Stage)
	confirmed, err := assignments.GetAssignment(ctx, session, "SAS911")
	require.NoError(t, err)
	assert.Equal(t, StageConfirmed, confirmed.Stage)
	assert.Equal(t, "A1", confirmed.Stand)
	assert.Equal(t, []string{"SAS911"}, operational)
}

func (r *arrivalPriorityRefresh) ArrivalProcessingPriority(strip *models.Strip, flight vatsim.ArrivalFlightInfo, assignment *models.StandAssignment) int {
	priority := r.ArrivalLifecycleService.ArrivalProcessingPriority(strip, flight, assignment)
	r.priorities = append(r.priorities, priority)
	if r.publish != nil {
		r.publish()
		r.publish = nil
	}
	return priority
}

// A blocked pin followed by a missed approach is no longer eligible to
// promote: its pinned-stand warning clears, the controller's prior reason and
// the stand are kept, and no operational stand is published.
func TestPinnedArrivalPromotionWarningClearsAfterMissedApproach(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := t.Context()
	lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
	seedTestArrivalStrip(t, queries, session, "SAS910")
	testdata.SeedTestStrip(t, queries, session, "SAS911")
	eta := clock.current().Add(time.Minute)
	setArrivalETA(t, strips, session, "SAS910", eta)
	const priorReason = "controller approved a compatibility exception"
	require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
		SessionID: session, Callsign: "SAS910", Stand: "A2", Direction: "ARRIVAL",
		Stage: StageEstimated, Manual: true, Source: "MANUAL_OVERRIDE", ETA: &eta,
		ETASource: strp("FILED"), ConflictReason: strp(priorReason), Acknowledged: true,
	}))
	departure := withStand(standAllocationRequest(session, "SAS911"), "A2")
	departure.Direction = sat.AssignmentDirectionDeparture
	departure.FlightFacts.Direction = sat.Departure
	departure.Stage = StageDepartureBlock
	departure.ObservedStand = strp("A2")
	_, err := allocations.assignObservedStand(ctx, departure)
	require.NoError(t, err)

	latitude, longitude := 55.8, 12.642625
	finalAltitude, missedAltitude := int32(2000), int32(20000)
	_, err = strips.UpdateAircraftPosition(ctx, session, "SAS910", &latitude, &longitude, &finalAltitude, "FINAL", nil)
	require.NoError(t, err)
	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
	waiting, err := assignments.GetAssignment(ctx, session, "SAS910")
	require.NoError(t, err)
	require.True(t, HasPinnedArrivalPromotionConflict(waiting), "the occupied pin waits with a warning")

	var published []StandAllocationResult
	allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
		published = append(published, result)
		return nil
	})
	_, err = strips.UpdateAircraftPosition(ctx, session, "SAS910", &latitude, &longitude, &missedAltitude, "", nil)
	require.NoError(t, err)
	require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 2)))

	cleared, err := assignments.GetAssignment(ctx, session, "SAS910")
	require.NoError(t, err)
	assert.False(t, HasPinnedArrivalPromotionConflict(cleared))
	assert.Equal(t, strp(priorReason), cleared.ConflictReason, "the controller's own reason is kept")
	assert.Equal(t, StageEstimated, cleared.Stage)
	assert.Equal(t, "A2", cleared.Stand)
	assert.Nil(t, loadStrip(t, strips, session, "SAS910").Stand, "no operational stand is written to the strip")
	require.Len(t, published, 1)
	assert.False(t, published[0].StandChanged)
	assert.False(t, published[0].NotifyEuroscope)
}

func TestPinnedArrivalPromotionWaitsForOccupiedStand(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, tc := range []struct {
		name         string
		departure    string
		directive    string
		latitude     float64
		longitude    float64
		altitude     int32
		bay          string
		wantConflict string
	}{
		{name: "observed departure on the pinned stand", departure: "A2", latitude: 55.8, longitude: 12.642625, altitude: 2000, bay: "FINAL", wantConflict: "reserved by SAS911"},
		{name: "observed departure blocking the pinned stand at the airport", departure: "A1", directive: "BLOCKS:A2", latitude: 55.6181, longitude: 12.6561, bay: "TAXI", wantConflict: "blocked by allocated neighbor A1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			lifecycle, allocations, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, tc.directive, "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS910")
			testdata.SeedTestStrip(t, queries, session, "SAS911")
			eta := clock.current().Add(time.Minute)
			setArrivalETA(t, strips, session, "SAS910", eta)
			const priorReason = "controller approved a compatibility exception"
			estimate := &models.StandAssignment{
				SessionID: session, Callsign: "SAS910", Stand: "A2", Direction: "ARRIVAL",
				Stage: StageEstimated, Manual: true, Source: "MANUAL_OVERRIDE", ETA: &eta,
				ETASource: strp("FILED"), ConflictReason: strp(priorReason), Acknowledged: true,
			}
			require.NoError(t, assignments.CreateAssignment(ctx, estimate))

			departure := withStand(standAllocationRequest(session, "SAS911"), tc.departure)
			departure.Direction = sat.AssignmentDirectionDeparture
			departure.FlightFacts.Direction = sat.Departure
			departure.Stage = StageDepartureBlock
			departure.ObservedStand = strp(tc.departure)
			observed, err := allocations.assignObservedStand(ctx, departure)
			require.NoError(t, err)
			require.Equal(t, tc.departure, observed.Assignment.Stand)
			require.Empty(t, observed.RemovedAssignments)

			_, err = strips.UpdateAircraftPosition(ctx, session, "SAS910", &tc.latitude, &tc.longitude, &tc.altitude, tc.bay, nil)
			require.NoError(t, err)
			var published []StandAllocationResult
			allocations.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
				published = append(published, result)
				return nil
			})
			process := func() {
				t.Helper()
				require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
			}

			process()
			waiting, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			assert.Equal(t, StageEstimated, waiting.Stage)
			assert.Equal(t, "A2", waiting.Stand)
			assert.True(t, waiting.Manual)
			require.NotNil(t, waiting.ConflictReason)
			assert.Contains(t, *waiting.ConflictReason, tc.wantConflict)
			assert.Contains(t, *waiting.ConflictReason, priorReason)
			assert.False(t, waiting.Acknowledged)
			assert.Nil(t, loadStrip(t, strips, session, "SAS910").Stand)
			require.Len(t, published, 1)
			assert.Equal(t, waiting.ConflictReason, published[0].Assignment.ConflictReason)
			assert.False(t, published[0].StandChanged)
			assert.False(t, published[0].NotifyEuroscope)

			process()
			assert.Len(t, published, 1, "an unchanged conflict needs no new publication")

			require.NoError(t, allocations.ReleaseAssignment(ctx, &observed.Assignment))
			published = nil
			process()
			promoted, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			assert.Equal(t, waiting.ID, promoted.ID)
			assert.Equal(t, StageConfirmed, promoted.Stage)
			assert.Equal(t, "A2", promoted.Stand)
			assert.Equal(t, strp(priorReason), promoted.ConflictReason)
			assert.Equal(t, strp("A2"), loadStrip(t, strips, session, "SAS910").Stand)
			require.Len(t, published, 1)
			assert.True(t, published[0].NotifyEuroscope)
		})
	}
}

func TestPinnedArrivalPromotionHonorsDepartureReleaseProjection(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	for _, tc := range []struct {
		name          string
		releaseOffset time.Duration
		blocked       bool
	}{
		{name: "missed projection", releaseOffset: -5 * time.Minute, blocked: true},
		{name: "projection due now", blocked: true},
		{name: "future release before arrival", releaseOffset: 5 * time.Minute},
		{name: "future release after arrival", releaseOffset: 15 * time.Minute, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, "", "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS910")
			testdata.SeedTestStrip(t, queries, session, "SAS911")
			eta := clock.current().Add(10 * time.Minute)
			setArrivalETA(t, strips, session, "SAS910", eta)
			require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
				SessionID: session, Callsign: "SAS910", Stand: "A2", Direction: "ARRIVAL",
				Stage: StageEstimated, Source: "MANUAL", Manual: true, ETA: &eta,
			}))
			release := clock.current().Add(tc.releaseOffset)
			require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
				SessionID: session, Callsign: "SAS911", Stand: "A2", Direction: "DEPARTURE",
				Stage: StageDepartureBlock, Source: "OBSERVED", ProjectedReleaseAt: &release,
			}))
			_, err := strips.UpdateAircraftPosition(ctx, session, "SAS910", posPtr(55.8), posPtr(12.642625), altPtr(8000), "FINAL", nil)
			require.NoError(t, err)
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS910"), arrivalFlight("SAS910", 1)))
			assignment, err := assignments.GetAssignment(ctx, session, "SAS910")
			require.NoError(t, err)
			require.Equal(t, "A2", assignment.Stand, "the controller's pinned choice must survive")
			if tc.blocked {
				require.Equal(t, StageEstimated, assignment.Stage)
				require.True(t, HasPinnedArrivalPromotionConflict(assignment))
				require.Nil(t, loadStrip(t, strips, session, "SAS910").Stand)
			} else {
				require.Equal(t, StageAssigned, assignment.Stage)
				require.False(t, HasPinnedArrivalPromotionConflict(assignment))
				require.Equal(t, strp("A2"), loadStrip(t, strips, session, "SAS910").Stand)
			}
		})
	}
}

func TestAssignedArrivalKeepsStandAgainstSoftReservations(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, neighbor := range []bool{false, true} {
		t.Run(map[bool]string{false: "same_stand", true: "neighboring_stand"}[neighbor], func(t *testing.T) {
			directive := ""
			if neighbor {
				directive = "BLOCKS:A2"
			}
			lifecycle, _, session, assignments, strips, clock := arrivalLifecycleFixture(t, pool, queries, directive, "", nil)
			seedTestArrivalStrip(t, queries, session, "SAS930")
			seedTestArrivalStrip(t, queries, session, "SAS931")
			eta := clock.current().Add(45 * time.Minute)
			setArrivalETA(t, strips, session, "SAS930", eta)
			_, err := strips.UpdateAircraftPosition(ctx, session, "SAS930", posPtr(55.8), posPtr(12.642625), altPtr(8000), "FINAL", nil)
			require.NoError(t, err)
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS930"), arrivalFlight("SAS930", 1)))
			assigned, err := assignments.GetAssignment(ctx, session, "SAS930")
			require.NoError(t, err)
			require.Equal(t, StageAssigned, assigned.Stage)
			require.Equal(t, "A1", assigned.Stand)
			estimatedStand := assigned.Stand
			if neighbor {
				estimatedStand = "A2"
			}
			require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
				SessionID: session, Callsign: "SAS931", Stand: estimatedStand, Direction: "ARRIVAL", Stage: StageEstimated, Source: "AUTOMATIC", ETA: &eta,
			}))
			require.NoError(t, lifecycle.ProcessArrival(ctx, session, loadStrip(t, strips, session, "SAS930"), arrivalFlight("SAS930", 2)))
			updated, err := assignments.GetAssignment(ctx, session, "SAS930")
			require.NoError(t, err)
			assert.Equal(t, assigned.Stand, updated.Stand)
			assert.Equal(t, assigned.Version, updated.Version, "a soft reservation must not trigger another allocation")
			estimate, err := assignments.GetAssignment(ctx, session, "SAS931")
			require.NoError(t, err)
			assert.Equal(t, StageEstimated, estimate.Stage)
			assert.Equal(t, estimatedStand, estimate.Stand)
		})
	}
}
