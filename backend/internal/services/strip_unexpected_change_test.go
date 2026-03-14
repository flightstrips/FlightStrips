package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	pkgModels "FlightStrips/pkg/models"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helpers to build a minimal syncEuroscopeStrip setup.
// Returns (svc, stripRepo, hub) wired together with a session repository that
// returns an empty ActiveRunways so that autoAssignRunway is a no-op.
func newSyncTestFixture(t *testing.T, _ *models.Strip, stripRepo *testutil.MockStripRepository) (*StripService, *testutil.MockFrontendHub) {
	t.Helper()

	sessionRepo := &testutil.MockSessionRepository{
		GetByIDFn: func(_ context.Context, _ int32) (*models.Session, error) {
			return &models.Session{ActiveRunways: pkgModels.ActiveRunways{}}, nil
		},
	}

	mockServer := &testutil.MockServer{
		SessionRepoVal: sessionRepo,
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(mockServer)

	// MoveToBay needs GetMaxSequenceInBayFn; provide a safe default.
	if stripRepo.GetMaxSequenceInBayFn == nil {
		stripRepo.GetMaxSequenceInBayFn = func(_ context.Context, _ int32, _ string) (int32, error) {
			return int32(0), nil
		}
	}
	if stripRepo.UpdateBayAndSequenceFn == nil {
		stripRepo.UpdateBayAndSequenceFn = func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		}
	}
	// UpdateRegistration is called inside syncEuroscopeStrip for existing strips.
	if stripRepo.UpdateRegistrationFn == nil {
		stripRepo.UpdateRegistrationFn = func(_ context.Context, _ int32, _ string, _ string) error {
			return nil
		}
	}

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	return svc, hub
}

// buildEuroscopeStrip builds a minimal euroscope.Strip for testing.
func buildEuroscopeStrip(callsign, stand, runway string) euroscopeEvents.Strip {
	return euroscopeEvents.Strip{
		Callsign: callsign,
		Stand:    stand,
		Runway:   runway,
	}
}

// ── Stand unexpected change ────────────────────────────────────────────────

func TestSyncEuroscopeStrip_StandOverwrite_SetsUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS123"

	existingStand := "A6"
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_NOT_CLEARED,
		Stand:    &existingStand,
	}

	var appendedField string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, _ int32, _ string, field string) error {
			appendedField = field
			return nil
		},
	}

	svc, hub := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "B9", "") // stand changed A6 → B9
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)

	assert.Equal(t, "stand", appendedField, "expected stand to be marked as unexpected change")
	require.Len(t, hub.StripUpdates, 1)
	assert.Equal(t, callsign, hub.StripUpdates[0].Callsign)
}

func TestSyncEuroscopeStrip_StandFirstSet_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "EZY456"

	emptyStand := ""
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_NOT_CLEARED,
		Stand:    &emptyStand, // no stand yet
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set — panics if called
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "A3", "") // first stand assignment
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)
	// No panic → AppendUnexpectedChangeField was not called
}

func TestSyncEuroscopeStrip_StandSameValue_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "BAW789"

	stand := "D3"
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_NOT_CLEARED,
		Stand:    &stand,
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set — panics if called
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "D3", "") // same stand value
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)
}

// ── Runway unexpected change ───────────────────────────────────────────────

func TestSyncEuroscopeStrip_RunwayOverwrite_ApronBay_SetsUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "DLH200"

	existingRunway := "04L"
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_TAXI, // apron bay → runway yellow applies
		Runway:   &existingRunway,
	}

	var appendedField string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		AppendUnexpectedChangeFieldFn: func(_ context.Context, _ int32, _ string, field string) error {
			appendedField = field
			return nil
		},
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "", "22R") // runway changed 04L → 22R
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)

	assert.Equal(t, "runway", appendedField)
}

func TestSyncEuroscopeStrip_RunwayOverwrite_NonApronBay_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "KLM300"

	existingRunway := "04L"
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_NOT_CLEARED, // CLX/DEL bay → runway yellow NOT applied
		Runway:   &existingRunway,
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set — panics if called
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "", "22R")
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)
}

func TestSyncEuroscopeStrip_RunwayFirstSet_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "AFR500"

	emptyRunway := ""
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_TAXI,
		Runway:   &emptyRunway, // no runway yet
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "", "04L")
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)
}

func TestSyncEuroscopeStrip_RunwaySameValue_NoUnexpectedChange(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "RYR600"

	runway := "22R"
	existingStrip := &models.Strip{
		Callsign: callsign,
		Bay:      shared.BAY_TAXI,
		Runway:   &runway,
	}

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return existingStrip, nil
		},
		UpdateFn: func(_ context.Context, _ *models.Strip) (int64, error) {
			return 1, nil
		},
		// AppendUnexpectedChangeFieldFn intentionally NOT set
	}

	svc, _ := newSyncTestFixture(t, existingStrip, stripRepo)

	esStrip := buildEuroscopeStrip(callsign, "", "22R") // same runway value
	err := svc.syncEuroscopeStrip(ctx, session, esStrip, "EKCH")
	require.NoError(t, err)
}

// ── Acknowledge unexpected change (service: RemoveUnexpectedChangeField) ──

func TestRemoveUnexpectedChangeField_CallsRepo(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS999"
	const field = "stand"

	var removedField string
	stripRepo := &testutil.MockStripRepository{
		RemoveUnexpectedChangeFieldFn: func(_ context.Context, _ int32, _ string, f string) error {
			removedField = f
			return nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := stripRepo.RemoveUnexpectedChangeField(ctx, session, callsign, field)
	require.NoError(t, err)
	assert.Equal(t, field, removedField)
}
