package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/sat"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandAvailabilityIncludesDirectReservationCallsigns(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	service, session, assignments := standAllocationFixture(t, pool, queries, "BLOCKS:A2", "")
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	expired := now.Add(-time.Minute)
	for _, item := range []struct {
		callsign string
		expiry   *time.Time
	}{
		{"SAS999", nil}, {"SAS888", nil}, {"EXPIRED", &expired},
	} {
		require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
			SessionID: session, Callsign: item.callsign, Stand: "A1", Stage: StageConfirmed,
			Direction: string(sat.AssignmentDirectionArrival), Source: "MANUAL", ExpiresAt: item.expiry,
		}))
	}
	available, err := service.AvailableStands(ctx, standAllocationRequest(session, "SAS123"))
	require.NoError(t, err)
	require.Len(t, available, 2)
	assert.Equal(t, []string{"SAS888", "SAS999"}, available[0].ReservedBy)
	assert.False(t, available[0].Available)
	assert.Empty(t, available[1].ReservedBy, "an aircraft on a neighboring stand must not be shown as occupying this stand")
	assert.Contains(t, available[1].Reason, "blocked by allocated neighbor A1")

	own, err := service.AvailableStands(ctx, standAllocationRequest(session, "SAS999"))
	require.NoError(t, err)
	assert.Equal(t, []string{"SAS888"}, own[0].ReservedBy, "the requesting aircraft is not a competing occupant")
}

func TestStandAvailabilityOmitsExpiredReservations(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	service, session, assignments := standAllocationFixture(t, pool, queries, "", "")
	expired := time.Now().Add(-time.Minute)
	require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
		SessionID: session, Callsign: "SAS999", Stand: "A1", Stage: StageConfirmed,
		Direction: string(sat.AssignmentDirectionArrival), Source: "MANUAL", ExpiresAt: &expired,
	}))
	available, err := service.AvailableStands(ctx, standAllocationRequest(session, "SAS123"))
	require.NoError(t, err)
	require.Len(t, available, 2)
	assert.True(t, available[0].Available)
	assert.Empty(t, available[0].ReservedBy)
}

func TestConfirmedUnsuitableStandProtectsOccupancyAndBlocks(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	ctx := context.Background()
	for _, protection := range []string{"none", "reserved", "neighbor", "manual", "manual-neighbor"} {
		t.Run(protection, func(t *testing.T) {
			service, session, assignments := standAllocationFixture(t, pool, queries, "WTC:M\nBLOCKS:A2", "")
			testdata.SeedTestStrip(t, queries, session, "SAS123")
			request := withStand(standAllocationRequest(session, "SAS123"), "A1")
			request.FlightFacts.WTC = "H"
			request.ProtectReservations = true
			if protection == "reserved" || protection == "neighbor" {
				stand := "A1"
				if protection == "neighbor" {
					stand = "A2"
				}
				require.NoError(t, assignments.CreateAssignment(ctx, &models.StandAssignment{
					SessionID: session, Callsign: "OTHER", Stand: stand, Stage: StageEstimated,
					Direction: string(sat.AssignmentDirectionArrival), Source: "AUTOMATIC",
				}))
			}
			if protection == "manual" || protection == "manual-neighbor" {
				stand := "A1"
				if protection == "manual-neighbor" {
					stand = "A2"
				}
				require.NoError(t, service.CreateManualBlock(ctx, "EKCH", &models.StandBlock{
					SessionID: session, Stand: stand, BlockType: "MANUAL", Source: "CONTROLLER", Manual: true,
				}))
			}
			availability, err := service.AvailableStands(ctx, request)
			require.NoError(t, err)
			assert.True(t, availability[0].Unsuitable)
			assert.False(t, availability[0].Available)
			assert.Equal(t, protection == "none", availability[0].Selectable)
			_, err = service.AssignManually(ctx, request)
			require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
			request.AllowUnsuitable = true
			result, err := service.AssignManually(ctx, request)
			if protection != "none" {
				require.ErrorIs(t, err, ErrIncompatibleManualAssignment)
				_, err = assignments.GetAssignment(ctx, session, "SAS123")
				require.Error(t, err, "a confirmation must never override occupancy or controller restrictions")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "A1", result.Assignment.Stand)
			assert.Contains(t, result.ConflictReason, "pilot confirmed unsuitable stand")
			assert.True(t, result.Assignment.Manual)
			request.Stand = "UNKNOWN"
			_, err = service.AssignManually(ctx, request)
			require.ErrorIs(t, err, ErrUnknownManualOverrideStand)
		})
	}
}
