package pdc

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// Message is a copy to avoid import cycle
type mockHoppieMessage struct {
	From   string
	To     string
	Type   string
	Packet string
	Raw    string
}

type mockHoppieClient struct {
	mock.Mock
}

func (m *mockHoppieClient) Poll(ctx context.Context, callsign string) ([]mockHoppieMessage, error) {
	args := m.Called(ctx, callsign)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]mockHoppieMessage), args.Error(1)
}

func (m *mockHoppieClient) SendCPDLC(ctx context.Context, from, to, packet string) error {
	args := m.Called(ctx, from, to, packet)
	return args.Error(0)
}

func (m *mockHoppieClient) SendTelex(ctx context.Context, from, to, packet string) error {
	args := m.Called(ctx, from, to, packet)
	return args.Error(0)
}
