package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"context"
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

func TestArrivalEstimateDeferralUsesActiveBlocks(t *testing.T) {
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
			if active {
				assert.NotEqual(t, estimate.Stand, updated.Stand)
			} else {
				assert.Equal(t, estimate.Stand, updated.Stand)
			}
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
