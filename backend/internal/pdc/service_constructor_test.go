package pdc

import (
	"FlightStrips/internal/testutil"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type constructorHoppieClient struct{}

func (constructorHoppieClient) Poll(context.Context, string) ([]Message, error) { return nil, nil }
func (constructorHoppieClient) SendCPDLC(context.Context, string, string, string) error {
	return nil
}
func (constructorHoppieClient) SendTelex(context.Context, string, string, string) error {
	return nil
}

func validPdcServiceDependencies() ServiceDependencies {
	return ServiceDependencies{
		Client:       constructorHoppieClient{},
		Sessions:     &testutil.MockSessionRepository{},
		Strips:       &testutil.MockStripRepository{},
		Sectors:      &testutil.MockSectorOwnerRepository{},
		Controllers:  &testutil.MockControllerRepository{},
		Frontend:     &mockPdcFrontendHub{},
		Euroscope:    &mockPdcEuroscopeHub{},
		StripService: &mockPdcStripService{},
	}
}

func TestNewPDCServiceRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*ServiceDependencies)
		want   string
	}{
		{"client", func(d *ServiceDependencies) { d.Client = nil }, "pdc service requires Hoppie client"},
		{"sessions", func(d *ServiceDependencies) { d.Sessions = nil }, "pdc service requires session repository"},
		{"strips", func(d *ServiceDependencies) { d.Strips = nil }, "pdc service requires strip store"},
		{"sectors", func(d *ServiceDependencies) { d.Sectors = nil }, "pdc service requires sector repository"},
		{"controllers", func(d *ServiceDependencies) { d.Controllers = nil }, "pdc service requires controller repository"},
		{"frontend", func(d *ServiceDependencies) { d.Frontend = nil }, "pdc service requires frontend publisher"},
		{"euroscope", func(d *ServiceDependencies) { d.Euroscope = nil }, "pdc service requires EuroScope commander"},
		{"strip service", func(d *ServiceDependencies) { d.StripService = nil }, "pdc service requires strip service"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validPdcServiceDependencies()
			test.remove(&deps)
			_, err := NewPDCService(deps)
			require.EqualError(t, err, test.want)
		})
	}
}

func TestNewPDCServiceRejectsNilTransceiverProvider(t *testing.T) {
	deps := validPdcServiceDependencies()
	deps.TransceiverProviders = []TransceiverLookup{nil}
	_, err := NewPDCService(deps)
	require.EqualError(t, err, "pdc service transceiver provider 0 is nil")
}
