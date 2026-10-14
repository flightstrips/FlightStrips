package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectNextControllerPersistsSIWithoutCoordination(t *testing.T) {
	ctx := context.Background()
	const owner = "121.730"
	strip := &models.Strip{
		Callsign: "SAS123", Session: 42, Origin: "EKCH", Destination: "ENGM",
		Owner: stringPtr(owner), Runway: stringPtr("22R"), Stand: stringPtr("A18"),
		NextOwners: []string{"118.580"}, Bay: "TAXI",
	}
	session := &models.Session{ID: 42, Airport: "EKCH", ActiveRunways: pkgModels.ActiveRunways{DepartureRunways: []string{"22R"}}}
	owners := []*models.SectorOwner{{Sector: []string{"TW"}, Position: "119.355"}}
	hub := &testutil.MockFrontendHub{}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		SetNextControllerSelectionFn: func(_ context.Context, sessionID int32, callsign, source, sector string, next []string, display *models.NextDisplay) (int64, error) {
			assert.Equal(t, int32(42), sessionID)
			assert.Equal(t, "SAS123", callsign)
			assert.Equal(t, owner, source)
			assert.Equal(t, "TW", sector)
			assert.Equal(t, []string{"119.355"}, next)
			require.NotNil(t, display)
			assert.Equal(t, "TW", display.Label)
			return 1, nil
		},
	}
	s := &Server{
		stripRepo: repo, frontendHub: hub,
		controllerRepo: &testutil.MockControllerRepository{ListFn: func(context.Context, int32) ([]*models.Controller, error) {
			return []*models.Controller{{Callsign: "EKCH_D_TWR", Position: "119.355"}}, nil
		}},
		sessionRepo: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) { return session, nil }},
		sectorRepo:  &testutil.MockSectorOwnerRepository{ListBySessionFn: func(context.Context, int32) ([]*models.SectorOwner, error) { return owners, nil }},
	}
	require.NoError(t, s.SelectNextControllerContext(ctx, 42, "SAS123", owner, "TW"))
	require.Len(t, hub.OwnersUpdates, 1)
	assert.Equal(t, owner, hub.OwnersUpdates[0].Owner)
	assert.Equal(t, []string{"119.355"}, hub.OwnersUpdates[0].NextOwners)
	assert.Empty(t, hub.CoordinationTransfers)
	assert.Equal(t, "TAXI", strip.Bay)

	route, updated, err := computeRouteStateForStrip(strip, session, owners, routeRadioState{})
	require.NoError(t, err)
	require.True(t, updated)
	assert.Equal(t, []string{"119.355"}, route.NextOwners, "normal recalculation must retain the selection")

	require.ErrorContains(t, s.SelectNextControllerContext(ctx, 42, "SAS123", "other", "TW"), "not assumed")
	require.ErrorContains(t, s.SelectNextControllerContext(ctx, 42, "SAS123", owner, "INVALID"), "invalid")
	require.ErrorContains(t, s.SelectNextControllerContext(ctx, 42, "SAS123", owner, "AA"), "not available")
	assert.Equal(t, []string{"119.355"}, strip.NextOwners, "offline selection must not replace the existing target")
	require.Len(t, hub.OwnersUpdates, 1)

	repo.SetNextControllerSelectionFn = func(context.Context, int32, string, string, string, []string, *models.NextDisplay) (int64, error) {
		return 0, nil
	}
	require.ErrorContains(t, s.SelectNextControllerContext(ctx, 42, "SAS123", owner, "TW"), "coordination is pending")
	require.Len(t, hub.OwnersUpdates, 1)
}

func TestSelectedNextControllerDisconnectionClearsSI(t *testing.T) {
	strip := &models.Strip{
		Origin: "EKCH", Destination: "ENGM", Owner: stringPtr("121.730"),
		Runway: stringPtr("22R"), Stand: stringPtr("A18"),
		NextControllerSector: stringPtr("TW"), NextOwners: []string{"118.105"},
	}
	session := &models.Session{Airport: "EKCH"}
	route, updated, err := computeRouteStateForStrip(strip, session, nil, routeRadioState{})
	require.NoError(t, err)
	require.True(t, updated)
	assert.Empty(t, route.NextOwners)
	assert.Nil(t, route.NextDisplay)
}
