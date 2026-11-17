package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunwayArrivalMoveCompletesWithoutPublisher(t *testing.T) {
	owner := "TWR"
	fixture := newFrontendMoveFixture(&models.Strip{
		Callsign: "SAS901", Origin: "ESSA", Destination: "EKCH", Bay: shared.BAY_FINAL, Owner: &owner,
	})
	fixture.svc.SetFrontendHub(nil)
	fixture.repo.SetRwyArrRunwayStatusOnEntryFn = func(context.Context, int32, string) (int64, error) {
		require.Equal(t, shared.BAY_RWY_ARR, fixture.strip.Bay)
		fixture.strip.RunwayCleared = true
		fixture.strip.RunwayConfirmed = true
		return 1, nil
	}
	require.NotPanics(t, func() {
		require.NoError(t, fixture.svc.MoveFrontendStrip(fixture.ctx, 1, "SAS901", shared.BAY_RWY_ARR, "1234567", "EKCH", owner, false, false))
	})
	require.Equal(t, shared.BAY_RWY_ARR, fixture.strip.Bay)
	require.True(t, fixture.strip.RunwayCleared)
	require.True(t, fixture.strip.RunwayConfirmed)
}

func TestPushbackValidationPersistsWithoutPublisher(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		inWindow, prepareValid bool
	}{
		{name: "reject outside window"},
		{name: "clear warning for valid timing", inWindow: true},
		{name: "clear warning after preparation", prepareValid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			valid, tsat := now.Format("1504"), now.Add(30*time.Minute).Format("1504")
			if tc.inWindow {
				tsat = valid
			}
			strip := &models.Strip{Callsign: "SAS779", CdmData: &models.CdmData{Tsat: &tsat}}
			if tc.inWindow || tc.prepareValid {
				strip.ValidationStatus = &models.ValidationStatus{IssueType: pushbackTsatValidationIssueType, Active: true, ContextKey: "old timing"}
			}
			service := &StripService{
				stripReader: &testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					return strip, nil
				}},
				validationStore: &validationStoreFake{
					setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
						strip.ValidationStatus = status
						return nil
					},
					clearValidationStatusFn: func(context.Context, int32, string) error {
						strip.ValidationStatus = nil
						return nil
					},
				},
				cdmService: pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) {
					require.False(t, tc.inWindow, "valid timing does not need preparation")
					if tc.prepareValid {
						strip.CdmData.Tsat = &valid
					}
					return valid, "", true, nil
				}},
			}
			var err error
			require.NotPanics(t, func() {
				err = service.validatePushbackTiming(t.Context(), 779, strip, "EKCH_GND")
			})
			if tc.inWindow || tc.prepareValid {
				require.NoError(t, err)
				require.Nil(t, strip.ValidationStatus)
			} else {
				require.ErrorContains(t, err, "pushback requires TSAT override")
				require.NotNil(t, strip.ValidationStatus)
				require.Equal(t, pushbackTsatValidationIssueType, strip.ValidationStatus.IssueType)
				require.True(t, strip.ValidationStatus.Active)
			}
		})
	}
}
