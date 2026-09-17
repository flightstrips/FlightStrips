package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChangedSquawkResolvesOldPeersAndPreservesOtherDuplicates(t *testing.T) {
	oldCode, newCode, otherCode := "4231", "4232", "4233"
	owner := "EKCH_A_GND"
	oldWarning := func() *models.ValidationStatus {
		return &models.ValidationStatus{IssueType: duplicateSquawkValidationIssueType, OwningPosition: owner, Active: true, ActivationKey: "prior"}
	}
	strips := []*models.Strip{
		{Callsign: "TARGET", Squawk: &newCode},
		{Callsign: "OLD", AssignedSquawk: &oldCode, Owner: &owner, Bay: shared.BAY_TAXI, ValidationStatus: oldWarning()},
		{Callsign: "NEW", AssignedSquawk: &newCode, Owner: &owner, Bay: shared.BAY_TAXI},
		{Callsign: "BRIDGE", Squawk: &oldCode, AssignedSquawk: &otherCode, Owner: &owner, Bay: shared.BAY_TAXI, ValidationStatus: oldWarning()},
		{Callsign: "OUTSIDE", Squawk: &otherCode},
		{Callsign: "UNRELATED", ValidationStatus: oldWarning()},
	}
	// OLD still shares oldCode with BRIDGE; clear that actual code after the
	// first pass to test removal of an old duplicate on the second change.
	reads := 0
	repo := &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) { reads++; return strips, nil },
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			t.Fatal("validation reloaded a strip from its current list")
			return nil, nil
		},
		SetValidationStatusFn: func(_ context.Context, _ int32, cs string, _ *models.ValidationStatus) error {
			require.NotEqual(t, "UNRELATED", cs)
			return nil
		},
		ClearValidationStatusFn: func(_ context.Context, _ int32, cs string) error { require.NotEqual(t, "UNRELATED", cs); return nil },
	}
	oldPeer, newPeer, bridge, unrelated := strips[1], strips[2], strips[3], strips[5]
	svc := NewStripService(repo)
	svc.SetFrontendHub(&testutil.MockFrontendHub{})
	require.NoError(t, svc.reevaluateChangedSquawk(context.Background(), 1, "TARGET", &oldCode, newCode, true))
	require.Equal(t, 1, reads)
	require.NotNil(t, newPeer.ValidationStatus, "new duplicate must activate")
	require.Equal(t, duplicateSquawkValidationIssueType, bridge.ValidationStatus.IssueType, "outside peer still contributes to membership")
	bridge.Squawk = nil
	require.NoError(t, svc.reevaluateChangedSquawk(context.Background(), 1, "BRIDGE", &oldCode, "", true))
	require.Equal(t, 2, reads, "a new pass must read fresh state")
	require.False(t, isDuplicateSquawkValidation(oldPeer.ValidationStatus), "old duplicate must resolve")
	require.Equal(t, "prior", unrelated.ValidationStatus.ActivationKey, "unrelated warnings are untouched")
	require.Nil(t, shared.GetWebsocketMessageState(context.Background()))
}
