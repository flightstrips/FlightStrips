package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/frontend"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildManualFPLService(t *testing.T, stripRepo *testutil.MockStripRepository, esHub *testutil.MockEuroscopeHub, fHub *testutil.MockFrontendHub) *StripService {
	t.Helper()
	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(fHub)
	svc.SetEuroscopeHub(esHub)
	// MoveToBay needs minimal repo stubs.
	if stripRepo.GetMaxSequenceInBayFn == nil {
		stripRepo.GetMaxSequenceInBayFn = func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		}
	}
	if stripRepo.UpdateBayAndSequenceFn == nil {
		stripRepo.UpdateBayAndSequenceFn = func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		}
	}
	return svc
}

// --- CreateManualFPL tests ---

func TestCreateManualFPL_CallsignNotFound_ReturnsError(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	err := svc.CreateManualFPL(context.Background(), 1, frontend.CreateManualFPLAction{
		Callsign: "SAS123",
		ADES:     "EKBI",
	}, "cid1", "EKCH")

	require.Error(t, err)
	assert.Empty(t, esHub.CreateFPLCalls, "no EuroScope event should be sent on error")
}

func TestCreateManualFPL_Success(t *testing.T) {
	const session int32 = 42
	const callsign = "SAS456"

	strip := &models.Strip{Callsign: callsign, Session: session, Origin: "EKCH", Bay: shared.BAY_NOT_CLEARED}
	var updatedCallsign, updatedDest string

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, cs string) (*models.Strip, error) {
			if cs == callsign {
				return strip, nil
			}
			return nil, pgx.ErrNoRows
		},
		UpdateIFRManualFPLFieldsFn: func(_ context.Context, _ int32, cs string, dest string, _ *string, _ *string, _ *string, _ *string, _ *int32, _ *string, _ *string, _ *string) (int64, error) {
			updatedCallsign = cs
			updatedDest = dest
			return 1, nil
		},
		GetSequenceFn: func(_ context.Context, _ int32, _ string, _ string) (int32, error) {
			return 0, errors.New("not found")
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	req := frontend.CreateManualFPLAction{
		Callsign: callsign,
		ADES:     "EKBI",
		SID:      "OLETO1P",
		EOBT:     "1200",
		RwyDep:   "22L",
	}
	err := svc.CreateManualFPL(context.Background(), session, req, "cid1", "EKCH")

	require.NoError(t, err)
	assert.Equal(t, callsign, updatedCallsign)
	assert.Equal(t, "EKBI", updatedDest)
	require.Len(t, esHub.CreateFPLCalls, 1)
	assert.Equal(t, callsign, esHub.CreateFPLCalls[0].Event.Callsign)
	assert.Equal(t, "EKBI", esHub.CreateFPLCalls[0].Event.Destination)
	found := false
	for _, su := range fHub.StripUpdates {
		if su.Callsign == callsign {
			found = true
			break
		}
	}
	assert.True(t, found, "SendStripUpdate should have been called for %s", callsign)
}

func TestCreateManualFPL_FLConversion(t *testing.T) {
	const session int32 = 1
	const callsign = "NAX100"
	strip := &models.Strip{Callsign: callsign, Session: session, Origin: "EKCH"}

	var capturedAlt *int32
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) { return strip, nil },
		UpdateIFRManualFPLFieldsFn: func(_ context.Context, _ int32, _ string, _ string, _ *string, _ *string, _ *string, _ *string, ra *int32, _ *string, _ *string, _ *string) (int64, error) {
			capturedAlt = ra
			return 1, nil
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	err := svc.CreateManualFPL(context.Background(), session, frontend.CreateManualFPLAction{
		Callsign: callsign,
		ADES:     "ESGG",
		FL:       "330",
	}, "cid", "EKCH")

	require.NoError(t, err)
	require.NotNil(t, capturedAlt)
	assert.Equal(t, int32(33000), *capturedAlt)
}

// --- CreateVFRFPL tests ---

func TestCreateVFRFPL_CallsignNotFound_ReturnsError(t *testing.T) {
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return nil, pgx.ErrNoRows
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	err := svc.CreateVFRFPL(context.Background(), 1, frontend.CreateVFRFPLAction{
		Callsign: "OY-ABC",
	}, "cid1")

	require.Error(t, err)
	assert.Empty(t, esHub.CreateFPLCalls)
}

func TestCreateVFRFPL_DefaultsSSRTo7000(t *testing.T) {
	const session int32 = 5
	const callsign = "OY-DEF"
	strip := &models.Strip{Callsign: callsign, Session: session, Origin: "EKCH"}

	var capturedSSR string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) { return strip, nil },
		UpdateVFRManualFPLFieldsFn: func(_ context.Context, _ int32, _ string, _ *string, _ *int32, ssr string, _ *string, _ *string, _ *string, _ string) (int64, error) {
			capturedSSR = ssr
			return 1, nil
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	err := svc.CreateVFRFPL(context.Background(), session, frontend.CreateVFRFPLAction{
		Callsign: callsign,
		// SSR intentionally left blank
	}, "cid")

	require.NoError(t, err)
	assert.Equal(t, "7000", capturedSSR)
	require.Len(t, esHub.CreateFPLCalls, 1)
	assert.Equal(t, "7000", esHub.CreateFPLCalls[0].Event.AssignedSquawk)
	found := false
	for _, su := range fHub.StripUpdates {
		if su.Callsign == callsign {
			found = true
			break
		}
	}
	assert.True(t, found, "SendStripUpdate should have been called for %s", callsign)
}

func TestCreateVFRFPL_PlacedInControlzoneBay(t *testing.T) {
	const session int32 = 3
	const callsign = "OY-GHI"
	strip := &models.Strip{Callsign: callsign, Session: session, Origin: "EKCH"}

	var capturedBay string
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) { return strip, nil },
		UpdateVFRManualFPLFieldsFn: func(_ context.Context, _ int32, _ string, _ *string, _ *int32, _ string, _ *string, _ *string, _ *string, bay string) (int64, error) {
			capturedBay = bay
			return 1, nil
		},
	}
	esHub := &testutil.MockEuroscopeHub{}
	fHub := &testutil.MockFrontendHub{}
	svc := buildManualFPLService(t, stripRepo, esHub, fHub)

	err := svc.CreateVFRFPL(context.Background(), session, frontend.CreateVFRFPLAction{
		Callsign: callsign,
		SSR:      "7001",
	}, "cid")

	require.NoError(t, err)
	assert.Equal(t, shared.BAY_CONTROLZONE, capturedBay)
}
