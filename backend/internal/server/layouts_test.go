package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateLayoutsContext_UsesSyncStateWithoutReloadingSessionOrControllers(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
	}))
	t.Cleanup(config.SetLayoutsForTest(map[string][]config.LayoutVariant{
		"EKCH_A_TWR": {
			{Active: []string{"22L"}, Layout: "TWR"},
		},
	}))

	ctx := shared.WithSyncState(context.Background(), &shared.SyncState{
		Session: &models.Session{
			ID: 1,
			ActiveRunways: pkgModels.ActiveRunways{
				DepartureRunways: []string{"22L"},
				ArrivalRunways:   []string{"22L"},
			},
		},
		ExistingControllers: map[string]*models.Controller{
			"EKCH_A_TWR": {Callsign: "EKCH_A_TWR", Position: "118.100"},
		},
	})

	layouts := make(map[string]string)
	server := &Server{
		sessionRepo: &testutil.MockSessionRepository{},
		controllerRepo: &testutil.MockControllerRepository{
			SetLayoutFn: func(_ context.Context, session int32, position string, layout *string) (int64, error) {
				assert.Equal(t, int32(1), session)
				require.NotNil(t, layout)
				layouts[position] = *layout
				return 1, nil
			},
		},
		frontendHub: &testutil.MockFrontendHub{},
	}

	err := server.UpdateLayoutsContext(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"118.100": "TWR"}, layouts)
}

func TestUpdateLayoutsContext_PrefersCallsignPositionOverCrossCoupledFrequency(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_A_TWR", Frequency: "118.100"},
		{Name: "EKCH_W_APP", Frequency: "119.805"},
	}))
	t.Cleanup(config.SetLayoutsForTest(map[string][]config.LayoutVariant{
		"EKCH_A_TWR": {
			{Active: []string{"22L"}, Layout: "TWR"},
		},
		"EKCH_W_APP": {
			{Active: []string{"22L"}, Layout: "APP"},
		},
	}))

	ctx := shared.WithSyncState(context.Background(), &shared.SyncState{
		Session: &models.Session{
			ID: 1,
			ActiveRunways: pkgModels.ActiveRunways{
				DepartureRunways: []string{"22L"},
				ArrivalRunways:   []string{"22L"},
			},
		},
		ExistingControllers: map[string]*models.Controller{
			"EKCH_A_TWR": {Callsign: "EKCH_A_TWR", Position: "119.805"},
		},
	})

	layouts := make(map[string]string)
	server := &Server{
		sessionRepo: &testutil.MockSessionRepository{},
		controllerRepo: &testutil.MockControllerRepository{
			SetLayoutFn: func(_ context.Context, session int32, position string, layout *string) (int64, error) {
				assert.Equal(t, int32(1), session)
				require.NotNil(t, layout)
				layouts[position] = *layout
				return 1, nil
			},
		},
		frontendHub: &testutil.MockFrontendHub{},
	}

	err := server.UpdateLayoutsContext(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"118.100": "TWR"}, layouts)
}
