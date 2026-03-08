package mocks

import (
	"context"

	"FlightStrips/pkg/events/frontend"

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
