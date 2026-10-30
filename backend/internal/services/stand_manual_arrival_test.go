package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManualArrivalStandImmediatelyBecomesAssigned(t *testing.T) {
	for _, actor := range []string{"controller", "pilot", "override"} {
		for _, stage := range []string{"", StageEstimated, StageAssigned, StageConfirmed} {
			t.Run(actor+"/"+stage, func(t *testing.T) {
				pool, queries := testdata.SetupTestDB(t)
				allocation, session, assignments := standAllocationFixture(t, pool, queries, "", "")
				testdata.SeedTestStrip(t, queries, session, "SAS900")
				ctx := context.Background()
				_, err := pool.Exec(ctx, "UPDATE strips SET origin = 'EDDF', destination = 'EKCH' WHERE session = $1 AND callsign = 'SAS900'", session)
				require.NoError(t, err)
				version := int32(0)
				if stage != "" {
					initial := withStand(standAllocationRequest(session, "SAS900"), "A1")
					initial.Stage = stage
					result, err := allocation.AssignManually(ctx, initial)
					require.NoError(t, err)
					version = result.Assignment.Version
				}
				var published *StandAllocationResult
				allocation.SetPublisher(func(_ context.Context, result StandAllocationResult) error {
					published = &result
					return nil
				})
				actions := NewStandActionService(allocation, assignments, postgres.NewStripRepository(pool), nil, nil, nil)
				var result *StandAllocationResult
				switch actor {
				case "controller":
					result, err = actions.AssignManually(ctx, session, "EKCH", "GND", "SAS900", "A2", version)
				case "pilot":
					result, err = actions.AssignForPilot(ctx, session, "EKCH", "1234567", "SAS900", "A2", version, false)
				case "override":
					result, err = actions.Override(ctx, session, "EKCH", "GND", "SAS900", "A2", "manual selection", version)
				}
				require.NoError(t, err)
				assert.Equal(t, StageAssigned, result.Assignment.Stage)
				assert.True(t, result.Assignment.Manual)
				assert.Nil(t, result.Assignment.ExpiresAt)
				persisted, err := assignments.GetAssignment(ctx, session, "SAS900")
				require.NoError(t, err)
				assert.Equal(t, StageAssigned, persisted.Stage)
				assert.Equal(t, "A2", persisted.Stand)
				strip, err := queries.GetStrip(ctx, database.GetStripParams{Session: session, Callsign: "SAS900"})
				require.NoError(t, err)
				require.NotNil(t, strip.Stand)
				assert.Equal(t, "A2", *strip.Stand)
				require.NotNil(t, published)
				assert.Equal(t, StageAssigned, published.Assignment.Stage)
				assert.True(t, published.NotifyEuroscope)
			})
		}
	}
}
