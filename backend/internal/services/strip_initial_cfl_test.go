package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── New strip ────────────────────────────────────────────────────────────────

func TestSyncEuroscopeStrip_NewDeparture_AutoSetsCFL_HighRunway(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS100",
		Origin:   "EKCH",
		Runway:   "04R",
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.Equal(t, shared.BAY_NOT_CLEARED, createdStrip.Bay)
	require.NotNil(t, createdStrip.ClearedAltitude)
	assert.Equal(t, int32(7000), *createdStrip.ClearedAltitude)
}

func TestSyncEuroscopeStrip_NewDeparture_AutoSetsCFL_LowRunway(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS101",
		Origin:   "EKCH",
		Runway:   "12",
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.Equal(t, shared.BAY_NOT_CLEARED, createdStrip.Bay)
	require.NotNil(t, createdStrip.ClearedAltitude)
	assert.Equal(t, int32(4000), *createdStrip.ClearedAltitude)
}

func TestSyncEuroscopeStrip_NewDeparture_DoesNotOverrideExistingCFL(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:        "SAS102",
		Origin:          "EKCH",
		Runway:          "04R",
		ClearedAltitude: 9000, // already set by EuroScope
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	require.NotNil(t, createdStrip.ClearedAltitude)
	assert.Equal(t, int32(9000), *createdStrip.ClearedAltitude, "must not override CFL already set by EuroScope")
}

func TestSyncEuroscopeStrip_NewDeparture_UnknownRunway_DoesNotSetCFL(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS103",
		Origin:   "EKCH",
		Runway:   "99", // not in config
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	// ClearedAltitude should remain 0 (nil pointer to zero value)
	if createdStrip.ClearedAltitude != nil {
		assert.Equal(t, int32(0), *createdStrip.ClearedAltitude, "unknown runway must not set CFL")
	}
}

func TestSyncEuroscopeStrip_NewArrival_DoesNotSetCFL(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	var createdStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, strip *models.Strip) error {
			createdStrip = strip
			return nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:    "SAS104",
		Origin:      "ESSA",
		Destination: "EKCH",
		Runway:      "04R",
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, createdStrip)
	assert.Equal(t, shared.BAY_ARR_HIDDEN, createdStrip.Bay)
	if createdStrip.ClearedAltitude != nil {
		assert.Equal(t, int32(0), *createdStrip.ClearedAltitude, "arrivals must not get an auto-CFL")
	}
}

// ── Existing strip ───────────────────────────────────────────────────────────

func TestSyncEuroscopeStrip_ExistingDeparture_TransitionsToNotCleared_AutoSetsCFL(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	existingStrip := &models.Strip{
		Callsign: "SAS200",
		Origin:   "EKCH",
		Bay:      shared.BAY_HIDDEN,
		Runway:   ptr("04L"),
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS200",
		Origin:   "EKCH",
		// ClearedAltitude is 0 (not set)
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	assert.Equal(t, shared.BAY_NOT_CLEARED, updatedStrip.Bay)
	require.NotNil(t, updatedStrip.ClearedAltitude)
	assert.Equal(t, int32(7000), *updatedStrip.ClearedAltitude)
}

func TestSyncEuroscopeStrip_ExistingDeparture_AlreadyInNotCleared_ZeroCFL_AutoSets(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	existingStrip := &models.Strip{
		Callsign: "SAS201",
		Origin:   "EKCH",
		Bay:      shared.BAY_NOT_CLEARED,
		Runway:   ptr("04R"),
		// ClearedAltitude is nil — not yet set
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	// EuroScope sends ClearedAltitude=0; strip is already in NOT_CLEARED but
	// has no CFL yet — the auto-set must still fire.
	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:        "SAS201",
		Origin:          "EKCH",
		ClearedAltitude: 0,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.ClearedAltitude)
	assert.Equal(t, int32(7000), *updatedStrip.ClearedAltitude)
}

func TestSyncEuroscopeStrip_ExistingDeparture_AlreadyInNotCleared_ExistingCFL_NotOverridden(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	existingClearedAlt := int32(7000)
	existingStrip := &models.Strip{
		Callsign:        "SAS202",
		Origin:          "EKCH",
		Bay:             shared.BAY_NOT_CLEARED,
		Runway:          ptr("04R"),
		ClearedAltitude: &existingClearedAlt,
	}

	var updatedStrip *models.Strip
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, strip *models.Strip) (int64, error) {
			updatedStrip = strip
			return 1, nil
		},
	}

	svc, _, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	// EuroScope sends a real ClearedAltitude — must not be overridden by auto-set.
	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:        "SAS202",
		Origin:          "EKCH",
		ClearedAltitude: 9000,
	}, "EKCH")
	require.NoError(t, err)
	require.NotNil(t, updatedStrip)
	require.NotNil(t, updatedStrip.ClearedAltitude)
	assert.Equal(t, int32(9000), *updatedStrip.ClearedAltitude, "ES-provided CFL must not be overridden")
}

// ── ES notification on auto-set CFL ─────────────────────────────────────────

func TestSyncEuroscopeStrip_NewDeparture_AutoSetsCFL_SendsAltitudeToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "1234567", euroscope.Strip{
		Callsign: "SAS300",
		Origin:   "EKCH",
		Runway:   "04R",
	}, "EKCH")
	require.NoError(t, err)

	require.Len(t, esHub.ClearedAltitudes, 1, "ES must receive the auto-set CFL")
	call := esHub.ClearedAltitudes[0]
	assert.Equal(t, session, call.Session)
	assert.Equal(t, "1234567", call.Cid)
	assert.Equal(t, "SAS300", call.Callsign)
	assert.Equal(t, int32(7000), call.Altitude)
}

func TestSyncEuroscopeStrip_NewDeparture_ExistingCFL_DoesNotSendAltitudeToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:        "SAS301",
		Origin:          "EKCH",
		Runway:          "04R",
		ClearedAltitude: 9000, // already set by ES — no auto-set should fire
	}, "EKCH")
	require.NoError(t, err)

	assert.Empty(t, esHub.ClearedAltitudes, "ES must not receive a CFL that was already set by ES")
}

func TestSyncEuroscopeStrip_NewDeparture_UnknownRunway_DoesNotSendAltitudeToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *models.Strip) error {
			return nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, nil, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign: "SAS302",
		Origin:   "EKCH",
		Runway:   "99", // not in config — no default CFL
	}, "EKCH")
	require.NoError(t, err)

	assert.Empty(t, esHub.ClearedAltitudes, "ES must not receive a CFL when no default is configured for the runway")
}

func TestSyncEuroscopeStrip_ExistingDeparture_TransitionsToNotCleared_SendsAltitudeToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	existingStrip := &models.Strip{
		Callsign: "SAS400",
		Origin:   "EKCH",
		Bay:      shared.BAY_HIDDEN,
		Runway:   ptr("04L"),
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "9876543", euroscope.Strip{
		Callsign: "SAS400",
		Origin:   "EKCH",
		// ClearedAltitude is 0 — triggers auto-set on transition to NOT_CLEARED
	}, "EKCH")
	require.NoError(t, err)

	require.Len(t, esHub.ClearedAltitudes, 1, "ES must receive the auto-set CFL on bay transition")
	call := esHub.ClearedAltitudes[0]
	assert.Equal(t, session, call.Session)
	assert.Equal(t, "9876543", call.Cid)
	assert.Equal(t, "SAS400", call.Callsign)
	assert.Equal(t, int32(7000), call.Altitude)
}

func TestSyncEuroscopeStrip_ExistingDeparture_CFLAlreadySet_DoesNotSendAltitudeToES(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)

	existingCFL := int32(7000)
	existingStrip := &models.Strip{
		Callsign:        "SAS401",
		Origin:          "EKCH",
		Bay:             shared.BAY_NOT_CLEARED,
		Runway:          ptr("04R"),
		ClearedAltitude: &existingCFL,
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
	}

	svc, _, esHub := newSyncTestFixture(t, existingStrip, stripRepo)

	err := svc.syncEuroscopeStrip(ctx, session, "", euroscope.Strip{
		Callsign:        "SAS401",
		Origin:          "EKCH",
		ClearedAltitude: 9000, // ES is providing a real CFL — no auto-set
	}, "EKCH")
	require.NoError(t, err)

	assert.Empty(t, esHub.ClearedAltitudes, "ES must not receive a CFL that was already provided by ES")
}
