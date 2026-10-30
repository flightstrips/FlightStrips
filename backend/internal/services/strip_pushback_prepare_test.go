package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPreparePushbackRequiresOverrideWithoutMoving(t *testing.T) {
	tsat := time.Now().UTC().Add(20 * time.Minute).Format("1504")
	fixture := newFrontendMoveFixture(&models.Strip{Callsign: "SAS123", Origin: "EKCH", Bay: shared.BAY_CLEARED, CdmData: &models.CdmData{Tsat: &tsat}})
	fixture.svc.validationStore = &validationStoreFake{setValidationStatusFn: func(_ context.Context, _ int32, _ string, status *models.ValidationStatus) error {
		fixture.strip.ValidationStatus = status
		return nil
	}}
	fixture.svc.cdmService = pushbackCdmStub{prepare: func(context.Context, int32, string) (string, string, bool, error) { return tsat, "", true, nil }}
	prepare := func() error {
		return fixture.svc.PrepareFrontendPushback(fixture.ctx, 42, "SAS123", "123", "EKCH", "121.905")
	}
	require.ErrorContains(t, prepare(), "override")
	require.True(t, fixture.strip.IsValidationLocked())
	require.Equal(t, shared.BAY_CLEARED, fixture.strip.Bay)
	fixture.strip.ValidationStatus.Active = false
	require.NoError(t, prepare())
	require.Equal(t, shared.BAY_CLEARED, fixture.strip.Bay)
	require.Empty(t, fixture.updateBayTargets)
	require.Empty(t, fixture.updateGroundBays)
}
func TestPreparePushbackChecksOwnership(t *testing.T) {
	owner := "121.730"
	fixture := newFrontendMoveFixture(&models.Strip{Callsign: "SAS123", Origin: "EKCH", Bay: shared.BAY_CLEARED, Owner: &owner})
	require.ErrorContains(t, fixture.svc.PrepareFrontendPushback(fixture.ctx, 42, "SAS123", "123", "EKCH", "121.905"), "not authorized")
	require.Empty(t, fixture.updateBayTargets)
}
