package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/require"
)

func TestSetReadyTransferValidationPublishesOwnerScopedOverride(t *testing.T) {
	const session int32 = 779
	const callsign = "SAS779"
	owner := "EKCH_B_GND"
	tsat := "1207"
	var persisted *models.ValidationStatus
	strip := &models.Strip{
		Callsign: callsign,
		Owner:    &owner,
		CdmData:  &models.CdmData{Tsat: &tsat},
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			return strip, nil
		},
		SetValidationStatusFn: func(_ context.Context, gotSession int32, gotCallsign string, status *models.ValidationStatus) error {
			require.Equal(t, session, gotSession)
			require.Equal(t, callsign, gotCallsign)
			persisted = status
			return nil
		},
	}
	service := &StripService{
		stripReader:     repo,
		validationStore: repo,
		publisher:       &testutil.MockFrontendHub{},
	}

	require.NoError(t, service.SetReadyTransferValidation(context.Background(), session, callsign, "EKCH_DEL"))
	require.NotNil(t, persisted)
	require.Equal(t, models.ValidationIssueTypeReadyTransferTsat, persisted.IssueType)
	require.Equal(t, owner, persisted.OwningPosition)
	require.Equal(t, "1207", persisted.ContextKey)
	require.True(t, persisted.Active)
	require.NotEmpty(t, persisted.ActivationKey)
	require.Contains(t, persisted.Message, "override")
}
