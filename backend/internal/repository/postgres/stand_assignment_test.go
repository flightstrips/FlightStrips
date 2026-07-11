package postgres

import (
	"FlightStrips/internal/database"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
)

func TestStandAssignmentToModelMapsDecisionAndProvenance(t *testing.T) {
	eta := time.Date(2026, 7, 11, 12, 30, 0, 0, time.UTC)
	assignedAt := eta.Add(-10 * time.Minute)
	ruleID := "SAS"
	tier := int32(2)
	variant := "ECHO-01"
	etaSource := "FILED"
	acknowledgedBy := "EKCH_GND"
	cid := int64(1234567)
	revision := int64(4)

	got := standAssignmentToModel(database.StandAssignment{
		ID:             7,
		SessionID:      11,
		Callsign:       "SAS123",
		Stand:          "ECHO12",
		Direction:      "ARRIVAL",
		Stage:          "ASSIGNED",
		Source:         "AUTOMATIC",
		RuleID:         &ruleID,
		Tier:           &tier,
		MatchedVariant: &variant,
		Eta:            pgtype.Timestamptz{Time: eta, Valid: true},
		EtaSource:      &etaSource,
		AssignedAt:     pgtype.Timestamptz{Time: assignedAt, Valid: true},
		Manual:         false,
		Acknowledged:   true,
		AcknowledgedAt: pgtype.Timestamptz{Time: eta, Valid: true},
		AcknowledgedBy: &acknowledgedBy,
		VatsimCid:      &cid,
		VatsimRevision: &revision,
		Version:        3,
		CreatedAt:      pgtype.Timestamptz{Time: assignedAt, Valid: true},
		UpdatedAt:      pgtype.Timestamptz{Time: eta, Valid: true},
	})

	require.Equal(t, int64(7), got.ID)
	require.Equal(t, int32(11), got.SessionID)
	require.Equal(t, "ECHO12", got.Stand)
	require.Equal(t, &variant, got.MatchedVariant)
	require.Equal(t, &eta, got.ETA)
	require.Equal(t, &assignedAt, got.AssignedAt)
	require.Equal(t, &cid, got.VatsimCID)
	require.Equal(t, &revision, got.VatsimRevision)
	require.True(t, got.Acknowledged)
	require.Equal(t, time.Time(eta), got.UpdatedAt)
}

func TestStandAssignmentToModelPreservesNullOptionalValues(t *testing.T) {
	got := standAssignmentToModel(database.StandAssignment{
		ID:        8,
		SessionID: 11,
		Callsign:  "SAS456",
		Stand:     "ECHO13",
		Direction: "DEPARTURE",
		Stage:     "ESTIMATED",
		Source:    "VATSIM",
		Version:   1,
	})

	require.Nil(t, got.ETA)
	require.Nil(t, got.AssignedAt)
	require.Nil(t, got.ExpiresAt)
	require.Nil(t, got.AcknowledgedAt)
	require.Nil(t, got.VatsimCID)
	require.Nil(t, got.VatsimRevision)
	require.Equal(t, time.Time{}, got.CreatedAt)
}

func TestStandBlockToModelMapsOptionalOccupancyProvenance(t *testing.T) {
	expires := time.Date(2026, 7, 11, 13, 0, 0, 0, time.UTC)
	reason := "maintenance"
	callsign := "SAS789"
	createdBy := "EKCH_TWR"

	got := standBlockToModel(database.StandBlock{
		ID:        12,
		SessionID: 11,
		Stand:     "FOXTROT1",
		BlockType: "OCCUPANCY",
		Source:    "CONTROLLER",
		Reason:    &reason,
		Callsign:  &callsign,
		CreatedBy: &createdBy,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
		Manual:    true,
		Version:   2,
	})

	require.Equal(t, "OCCUPANCY", got.BlockType)
	require.Equal(t, &callsign, got.Callsign)
	require.Equal(t, &createdBy, got.CreatedBy)
	require.Equal(t, &expires, got.ExpiresAt)
	require.True(t, got.Manual)
}
