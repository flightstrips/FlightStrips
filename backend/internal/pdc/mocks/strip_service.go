package mocks

import (
	"context"

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

func (m *StripService) MoveStripBetween(ctx context.Context, session int32, callsign string, before *string, bay string) error {
	args := m.Called(ctx, session, callsign, before, bay)
	return args.Error(0)
}
