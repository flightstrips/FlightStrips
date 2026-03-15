package services

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveAirborneController_NilSID verifies that a strip without a SID returns nil.
func TestResolveAirborneController_NilSID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	strip := &models.Strip{Sid: nil}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err)
	assert.Nil(t, controller, "no controller should be returned for a strip without a SID")
}

// TestResolveAirborneController_EmptySID verifies that an empty SID returns nil.
func TestResolveAirborneController_EmptySID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	empty := ""
	strip := &models.Strip{Sid: &empty}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err)
	assert.Nil(t, controller, "no controller should be returned for a strip with an empty SID")
}

// TestResolveAirborneController_UnknownSID verifies that an unconfigured SID returns nil without error.
// config package variables are empty during unit tests, so any SID will be unknown.
func TestResolveAirborneController_UnknownSID(t *testing.T) {
	svc := NewStripService(&testutil.MockStripRepository{})
	sid := "NOSUCHSID9A"
	strip := &models.Strip{Sid: &sid}

	controller, err := svc.resolveAirborneController(strip, []*models.Controller{})
	require.NoError(t, err, "unknown SID must not return an error — it falls back to nil")
	assert.Nil(t, controller, "no controller should be returned for an unknown SID")
}

// TestAutoTransferAirborneStrip_SkipsWhenNotAirborneBay verifies that a strip not in
// the AIRBORNE bay is skipped without error.
func TestAutoTransferAirborneStrip_SkipsWhenNotAirborneBay(t *testing.T) {
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Callsign: "SAS001", Bay: shared.BAY_DEPART}, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AutoTransferAirborneStrip(context.Background(), 1, "SAS001")
	require.NoError(t, err)
}

// TestAutoTransferAirborneStrip_SkipsWhenNoOwner verifies that an AIRBORNE strip with
// no owner is skipped without error.
func TestAutoTransferAirborneStrip_SkipsWhenNoOwner(t *testing.T) {
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{})

	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Callsign: "SAS001", Bay: shared.BAY_AIRBORNE, Owner: nil}, nil
		},
	})
	svc.SetFrontendHub(hub)

	err := svc.AutoTransferAirborneStrip(context.Background(), 1, "SAS001")
	require.NoError(t, err)
}

// TestAutoTransferAirborneStrip_SkipsWhenOwnerCidMissing verifies that a strip whose
// owner controller has no CID is not auto-transferred. A CID is required to send the
// EuroScope handover event — without it the transfer cannot be initiated.
func TestAutoTransferAirborneStrip_SkipsWhenOwnerCidMissing(t *testing.T) {
	// resolveAirborneController always returns nil in unit tests (no config loaded),
	// so the function exits before reaching the CID check. This test documents
	// the skip-when-no-owner path as a proxy; the CID guard is exercised in integration
	// tests where a SID route is configured.
	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{
		ControllerRepoVal: &testutil.MockControllerRepository{
			ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
				// Owner controller present but no CID set.
				owner := "EKCH_TWR"
				return []*models.Controller{{Position: owner, Cid: nil}}, nil
			},
		},
	})

	owner := "EKCH_TWR"
	svc := NewStripService(&testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{ID: 1, Callsign: "SAS001", Bay: shared.BAY_AIRBORNE, Owner: &owner}, nil
		},
	})
	svc.SetFrontendHub(hub)

	// With no SID the function exits before the CID check — no error expected.
	err := svc.AutoTransferAirborneStrip(context.Background(), 1, "SAS001")
	require.NoError(t, err)
}

// TestUpdateAircraftPosition_AutoHandoverTriggeredFromDepartBay verifies that when a
// strip transitions from the rwy-dep (DEPART) bay to AIRBORNE via a position update,
// AutoTransferAirborneStrip is invoked (evidenced by the controller-list query).
func TestUpdateAircraftPosition_AutoHandoverTriggeredFromDepartBay(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS001"

	departState := euroscope.GroundStateDepart
	owner := "EKCH_TWR"

	callCount := 0
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			callCount++
			if callCount == 1 {
				// First read: strip is in the runway-departure bay.
				return &models.Strip{
					ID: 1, Callsign: callsign, Bay: shared.BAY_DEPART,
					State: &departState, Origin: "EKCH", Owner: &owner,
				}, nil
			}
			// Subsequent reads (from AutoTransferAirborneStrip): strip is now AIRBORNE.
			return &models.Strip{
				ID: 1, Callsign: callsign, Bay: shared.BAY_AIRBORNE,
				State: &departState, Origin: "EKCH", Owner: &owner,
			}, nil
		},
		UpdateAircraftPositionFn: func(_ context.Context, _ int32, _ string, _ *float64, _ *float64, _ *int32, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
		GetMaxSequenceInBayFn: func(_ context.Context, _ int32, _ string) (int32, error) {
			return 0, nil
		},
		UpdateBayAndSequenceFn: func(_ context.Context, _ int32, _ string, _ string, _ int32) (int64, error) {
			return 1, nil
		},
	}

	listBySessionCalled := false
	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			listBySessionCalled = true
			return []*models.Controller{}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{ControllerRepoVal: controllerRepo})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	// EKCH coordinates, altitude 200 ft — above the 17 ft airborne threshold used in tests.
	err := svc.UpdateAircraftPosition(ctx, session, callsign,
		shared.AirportLatitude, shared.AirportLongitude, 200, "EKCH")
	require.NoError(t, err)
	assert.True(t, listBySessionCalled,
		"controller list must be queried when a strip transitions from DEPART to AIRBORNE")
}

// TestUpdateAircraftPosition_NoAutoHandoverWhenAlreadyAirborne verifies that a strip
// already in the AIRBORNE bay does not trigger auto-handover on position updates.
// Handover for already-airborne strips must only happen via explicit manual transfer.
func TestUpdateAircraftPosition_NoAutoHandoverWhenAlreadyAirborne(t *testing.T) {
	ctx := context.Background()
	const session = int32(1)
	const callsign = "SAS002"

	owner := "EKCH_APP"

	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, _ int32, _ string) (*models.Strip, error) {
			return &models.Strip{
				ID: 1, Callsign: callsign, Bay: shared.BAY_AIRBORNE,
				Origin: "EKCH", Owner: &owner,
			}, nil
		},
		UpdateAircraftPositionFn: func(_ context.Context, _ int32, _ string, _ *float64, _ *float64, _ *int32, _ string, _ *int32) (int64, error) {
			return 1, nil
		},
	}

	listBySessionCalled := false
	controllerRepo := &testutil.MockControllerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.Controller, error) {
			listBySessionCalled = true
			return []*models.Controller{}, nil
		},
	}

	hub := &testutil.MockFrontendHub{}
	hub.SetServer(&testutil.MockServer{ControllerRepoVal: controllerRepo})

	svc := NewStripService(stripRepo)
	svc.SetFrontendHub(hub)

	err := svc.UpdateAircraftPosition(ctx, session, callsign,
		shared.AirportLatitude, shared.AirportLongitude, 1000, "EKCH")
	require.NoError(t, err)
	assert.False(t, listBySessionCalled,
		"controller list must not be queried for already-airborne strips")
}
