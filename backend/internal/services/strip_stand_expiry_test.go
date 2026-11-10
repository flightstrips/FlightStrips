package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoveExpiredStandStripDeletesOnlySameStripStillInStand(t *testing.T) {
	tests := []struct {
		name        string
		currentID   int32
		currentBay  string
		destination string
		wantDeleted bool
	}{
		{name: "same arrival still in stand", currentID: 42, currentBay: shared.BAY_STAND, destination: "EKCH", wantDeleted: true},
		{name: "strip moved out of stand", currentID: 42, currentBay: shared.BAY_AIRBORNE, destination: "EKCH"},
		{name: "callsign now belongs to another strip", currentID: 43, currentBay: shared.BAY_STAND, destination: "EKCH"},
		{name: "same callsign reused for a departure", currentID: 42, currentBay: shared.BAY_STAND, destination: "EGLL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deleted := false
			strip := &models.Strip{ID: tt.currentID, Callsign: "THA123", Bay: tt.currentBay, Destination: tt.destination}
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					return strip, nil
				},
				DeleteFn: func(context.Context, int32, string) error {
					deleted = true
					return nil
				},
			}
			svc := newWorkflowTestService(repo)
			svc.SetFrontendHub(&testutil.MockFrontendHub{})
			bound, err := svc.workflowFor(context.Background(), 1, "")
			require.NoError(t, err)

			svc.removeExpiredStandStrip(context.Background(), 1, strip.Callsign, 42, bound)

			require.Equal(t, tt.wantDeleted, deleted)
		})
	}
}
