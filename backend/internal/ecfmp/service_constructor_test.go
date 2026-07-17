package ecfmp

import (
	"FlightStrips/internal/testutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func validServiceDependencies() ServiceDependencies {
	return ServiceDependencies{
		Client:    NewClient(),
		Strips:    &testutil.MockStripRepository{},
		Sessions:  &testutil.MockSessionRepository{},
		Frontend:  &testutil.MockFrontendHub{},
		Euroscope: &testutil.MockEuroscopeHub{},
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*ServiceDependencies)
		want   string
	}{
		{"client", func(d *ServiceDependencies) { d.Client = nil }, "ecfmp service requires client"},
		{"strips", func(d *ServiceDependencies) { d.Strips = nil }, "ecfmp service requires strip store"},
		{"sessions", func(d *ServiceDependencies) { d.Sessions = nil }, "ecfmp service requires session repository"},
		{"frontend", func(d *ServiceDependencies) { d.Frontend = nil }, "ecfmp service requires frontend publisher"},
		{"EuroScope", func(d *ServiceDependencies) { d.Euroscope = nil }, "ecfmp service requires EuroScope publisher"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validServiceDependencies()
			test.remove(&deps)
			_, err := NewService(deps)
			require.EqualError(t, err, test.want)
		})
	}
}
