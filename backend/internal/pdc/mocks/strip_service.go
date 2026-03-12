package mocks

import (
	"context"

	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"

	"github.com/stretchr/testify/mock"
)

type StripService struct {
	mock.Mock
}

func (m *StripService) ClearStrip(ctx context.Context, session int32, callsign, cid string) error {
	args := m.Called(ctx, session, callsign, cid)
	return args.Error(0)
}

func (m *StripService) UnclearStrip(ctx context.Context, session int32, callsign, cid string) error {
	args := m.Called(ctx, session, callsign, cid)
	return args.Error(0)
}

func (m *StripService) MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error {
	args := m.Called(ctx, session, callsign, bay, sendNotification)
	return args.Error(0)
}

func (m *StripService) MoveStripBetween(ctx context.Context, session int32, callsign string, insertAfter *frontend.StripRef, bay string) error {
	args := m.Called(ctx, session, callsign, insertAfter, bay)
	return args.Error(0)
}

func (m *StripService) MoveTacticalStripBetween(ctx context.Context, session int32, id int64, insertAfter *frontend.StripRef, bay string) error {
	args := m.Called(ctx, session, id, insertAfter, bay)
	return args.Error(0)
}

func (m *StripService) CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error {
	args := m.Called(ctx, session, callsign, from, to)
	return args.Error(0)
}

func (m *StripService) CreateEsArrivalCoordination(ctx context.Context, session int32, callsign string, from string, to string, esHandoverCid *string) error {
	args := m.Called(ctx, session, callsign, from, to, esHandoverCid)
	return args.Error(0)
}

func (m *StripService) AcceptCoordination(ctx context.Context, session int32, callsign string, assumingPosition string) error {
	args := m.Called(ctx, session, callsign, assumingPosition)
	return args.Error(0)
}

func (m *StripService) AutoTransferAirborneStrip(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *StripService) AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, stripVersion int32) error {
	args := m.Called(ctx, session, callsign, stripVersion)
	return args.Error(0)
}

func (m *StripService) AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error {
	args := m.Called(ctx, session, controllerPosition)
	return args.Error(0)
}

func (m *StripService) AssumeStripCoordination(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *StripService) RejectCoordination(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *StripService) CancelCoordinationTransfer(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *StripService) FreeStrip(ctx context.Context, session int32, callsign string, position string) error {
	args := m.Called(ctx, session, callsign, position)
	return args.Error(0)
}

func (m *StripService) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	args := m.Called(ctx, session, callsign, squawk)
	return args.Error(0)
}

func (m *StripService) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk string) error {
	args := m.Called(ctx, session, callsign, squawk)
	return args.Error(0)
}

func (m *StripService) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	args := m.Called(ctx, session, callsign, altitude)
	return args.Error(0)
}

func (m *StripService) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error {
	args := m.Called(ctx, session, callsign, altitude)
	return args.Error(0)
}

func (m *StripService) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType string) error {
	args := m.Called(ctx, session, callsign, commType)
	return args.Error(0)
}

func (m *StripService) UpdateHeading(ctx context.Context, session int32, callsign string, heading int32) error {
	args := m.Called(ctx, session, callsign, heading)
	return args.Error(0)
}

func (m *StripService) DeleteStrip(ctx context.Context, session int32, callsign string) error {
	args := m.Called(ctx, session, callsign)
	return args.Error(0)
}

func (m *StripService) UpdateGroundState(ctx context.Context, session int32, callsign string, groundState string, airport string) error {
	args := m.Called(ctx, session, callsign, groundState, airport)
	return args.Error(0)
}

func (m *StripService) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool) error {
	args := m.Called(ctx, session, callsign, cleared)
	return args.Error(0)
}

func (m *StripService) UpdateStand(ctx context.Context, session int32, callsign string, stand string) error {
	args := m.Called(ctx, session, callsign, stand)
	return args.Error(0)
}

func (m *StripService) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int32, airport string) error {
	args := m.Called(ctx, session, callsign, lat, lon, altitude, airport)
	return args.Error(0)
}

func (m *StripService) HandleTrackingControllerChanged(ctx context.Context, session int32, callsign string, trackingController string) error {
	args := m.Called(ctx, session, callsign, trackingController)
	return args.Error(0)
}

func (m *StripService) HandleCoordinationReceived(ctx context.Context, session int32, callsign string, controllerCallsign string) error {
	args := m.Called(ctx, session, callsign, controllerCallsign)
	return args.Error(0)
}

func (m *StripService) SyncStrip(ctx context.Context, session int32, strip interface{}, airport string) error {
	args := m.Called(ctx, session, strip, airport)
	return args.Error(0)
}

func (m *StripService) UpdateClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string) error {
	args := m.Called(ctx, session, callsign, isCleared, bay, cid)
	return args.Error(0)
}

func (m *StripService) UpdateGroundStateForMove(ctx context.Context, session int32, callsign string, bay string, cid string, airport string) error {
	args := m.Called(ctx, session, callsign, bay, cid, airport)
	return args.Error(0)
}

func (m *StripService) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string) error {
	args := m.Called(ctx, session, callsign, releasePoint)
	return args.Error(0)
}

func (m *StripService) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error {
	args := m.Called(ctx, session, callsign, marked)
	return args.Error(0)
}

func (m *StripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	args := m.Called(ctx, session, airport, oldRunways, newRunways)
	return args.Error(0)
}
