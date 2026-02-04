package mocks

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// Message is a copy to avoid import cycle
type Message struct {
	From   string
	To     string
	Type   string
	Packet string
	Raw    string
}

type HoppieClient struct {
	mock.Mock
}

func (m *HoppieClient) Poll(ctx context.Context, callsign string) ([]Message, error) {
	args := m.Called(ctx, callsign)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]Message), args.Error(1)
}

func (m *HoppieClient) SendCPDLC(ctx context.Context, from, to, packet string) error {
	args := m.Called(ctx, from, to, packet)
	return args.Error(0)
}

func (m *HoppieClient) SendTelex(ctx context.Context, from, to, packet string) error {
	args := m.Called(ctx, from, to, packet)
	return args.Error(0)
}
