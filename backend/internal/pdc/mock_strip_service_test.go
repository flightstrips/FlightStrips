package pdc

import (
	"context"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"

	"github.com/stretchr/testify/mock"
)

type mockPdcStripService struct {
	mock.Mock
}

func (m *mockPdcStripService) ClearStrip(ctx context.Context, session int32, callsign, cid string) error {
	args := m.Called(ctx, session, callsign, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) UnclearStrip(ctx context.Context, session int32, callsign, cid string) error {
	args := m.Called(ctx, session, callsign, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) ClearMandatoryRouteCdm(ctx context.Context, session int32, callsign string) {
	m.Called(ctx, session, callsign)
}

func (m *mockPdcStripService) MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error {
	args := m.Called(ctx, session, callsign, bay, sendNotification)
	return args.Error(0)
}

func (m *mockPdcStripService) MoveFrontendStrip(ctx context.Context, session int32, callsign string, targetBay string, cid string, airport string, clientPosition string) error {
	args := m.Called(ctx, session, callsign, targetBay, cid, airport, clientPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) MoveStripBetween(ctx context.Context, session int32, callsign string, insertAfter *frontend.StripRef, bay string) error {
	args := m.Called(ctx, session, callsign, insertAfter, bay)
	return args.Error(0)
}

func (m *mockPdcStripService) MoveTacticalStripBetween(ctx context.Context, session int32, id int64, insertAfter *frontend.StripRef, bay string) error {
	args := m.Called(ctx, session, id, insertAfter, bay)
	return args.Error(0)
}

func (m *mockPdcStripService) CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error {
	args := m.Called(ctx, session, callsign, from, to)
	return args.Error(0)
}

func (m *mockPdcStripService) CreateEsArrivalCoordination(ctx context.Context, session int32, callsign string, from string, to string, esHandoverCid *string) error {
	args := m.Called(ctx, session, callsign, from, to, esHandoverCid)
	return args.Error(0)
}

func (m *mockPdcStripService) AcceptCoordination(ctx context.Context, session int32, callsign string, assumingPosition string) error {
	args := m.Called(ctx, session, callsign, assumingPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) AutoTransferAirborneStrip(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *mockPdcStripService) AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *mockPdcStripService) AutoAssumeForClearedStripByCid(ctx context.Context, session int32, callsign string, cid string) error {
	args := m.Called(ctx, session, callsign, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error {
	args := m.Called(ctx, session, controllerPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) AssumeStripCoordination(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *mockPdcStripService) RejectCoordination(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *mockPdcStripService) CancelCoordinationTransfer(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *mockPdcStripService) FreeStrip(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *mockPdcStripService) CreateTagRequest(ctx context.Context, session int32, callsign string, requesterPosition string) error {
	args := m.Called(ctx, session, callsign, requesterPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) AcceptTagRequest(ctx context.Context, session int32, callsign string, ownerPosition string) error {
	args := m.Called(ctx, session, callsign, ownerPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	args := m.Called(ctx, session, callsign, squawk)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	args := m.Called(ctx, session, callsign, squawk)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	args := m.Called(ctx, session, callsign, altitude)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	args := m.Called(ctx, session, callsign, altitude)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType string) error {
	args := m.Called(ctx, session, callsign, commType)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateHeading(ctx context.Context, session int32, callsign string, heading int32) error {
	args := m.Called(ctx, session, callsign, heading)
	return args.Error(0)
}

func (m *mockPdcStripService) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateGroundState(ctx context.Context, session int32, callsign string, groundState string, airport string) error {
	args := m.Called(ctx, session, callsign, groundState, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool) error {
	args := m.Called(ctx, session, callsign, cleared)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	args := m.Called(ctx, session, callsign, stand)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int32, airport string) error {
	args := m.Called(ctx, session, callsign, lat, lon, altitude, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) HandleTrackingControllerChanged(ctx context.Context, session int32, callsign string, trackingController string) error {
	args := m.Called(ctx, session, callsign, trackingController)
	return args.Error(0)
}

func (m *mockPdcStripService) HandleCoordinationReceived(ctx context.Context, session int32, callsign string, sourceControllerCallsign string, targetControllerCallsign string) error {
	args := m.Called(ctx, session, callsign, sourceControllerCallsign, targetControllerCallsign)
	return args.Error(0)
}

func (m *mockPdcStripService) SyncStrip(ctx context.Context, session int32, cid string, strip interface{}, airport string) error {
	args := m.Called(ctx, session, cid, strip, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string) error {
	args := m.Called(ctx, session, callsign, isCleared, bay, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateGroundStateForMove(ctx context.Context, session int32, callsign string, bay string, cid string, airport string) error {
	args := m.Called(ctx, session, callsign, bay, cid, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) ConfirmPdcClearance(ctx context.Context, session int32, callsign string, bay string, cid string) error {
	args := m.Called(ctx, session, callsign, bay, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string) error {
	args := m.Called(ctx, session, callsign, releasePoint)
	return args.Error(0)
}

func (m *mockPdcStripService) ApplyReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string, clientPosition string) error {
	args := m.Called(ctx, session, callsign, releasePoint, clientPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateStartReq(ctx context.Context, session int32, callsign string, startReq bool) error {
	args := m.Called(ctx, session, callsign, startReq)
	return args.Error(0)
}

func (m *mockPdcStripService) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error {
	args := m.Called(ctx, session, callsign, marked)
	return args.Error(0)
}

func (m *mockPdcStripService) RunwayClearance(ctx context.Context, session int32, callsign string, cid string, airport string) error {
	args := m.Called(ctx, session, callsign, cid, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) RunwayConfirmation(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *mockPdcStripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	args := m.Called(ctx, session, airport, oldRunways, newRunways)
	return args.Error(0)
}

func (m *mockPdcStripService) ForceAssumeStrip(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *mockPdcStripService) CreateManualFPL(ctx context.Context, session int32, req frontend.CreateManualFPLAction, cid string, airport string) error {
	args := m.Called(ctx, session, req, cid, airport)
	return args.Error(0)
}

func (m *mockPdcStripService) CreateVFRFPL(ctx context.Context, session int32, req frontend.CreateVFRFPLAction, cid string) error {
	args := m.Called(ctx, session, req, cid)
	return args.Error(0)
}

func (m *mockPdcStripService) MissedApproach(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}

func (m *mockPdcStripService) SetValidationStatus(ctx context.Context, session int32, callsign string, status *internalModels.ValidationStatus) error {
	args := m.Called(ctx, session, callsign, status)
	return args.Error(0)
}

func (m *mockPdcStripService) AcknowledgeValidationStatus(ctx context.Context, session int32, callsign string, activationKey string, requestingPosition string) error {
	args := m.Called(ctx, session, callsign, activationKey, requestingPosition)
	return args.Error(0)
}

func (m *mockPdcStripService) ClearValidationStatus(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *mockPdcStripService) IsValidationBlocking(ctx context.Context, session int32, callsign string) (bool, error) {
	args := m.Called(ctx, session, callsign)
	return args.Bool(0), args.Error(1)
}

func (m *mockPdcStripService) ReevaluatePdcInvalidValidation(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	for _, call := range m.ExpectedCalls {
		if call.Method == "ReevaluatePdcInvalidValidation" {
			args := m.Called(ctx, session, callsign, publish, forceReactivate)
			return args.Error(0)
		}
	}

	return nil
}

func (m *mockPdcStripService) ReevaluatePdcRequestValidations(ctx context.Context, session int32, callsign string, publish bool, forceReactivate bool) error {
	for _, call := range m.ExpectedCalls {
		if call.Method == "ReevaluatePdcRequestValidations" {
			args := m.Called(ctx, session, callsign, publish, forceReactivate)
			return args.Error(0)
		}
	}

	return nil
}
