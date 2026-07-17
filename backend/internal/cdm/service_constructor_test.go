package cdm

import (
	"FlightStrips/internal/testutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func validCdmServiceDependencies() ServiceDependencies {
	return ServiceDependencies{
		Client:                NewClient(),
		Strips:                &testutil.MockStripRepository{},
		Sessions:              &testutil.MockSessionRepository{},
		Controllers:           &testutil.MockControllerRepository{},
		Frontend:              testCdmPublisher{},
		Euroscope:             testCdmEuroscope{},
		ValidationReevaluator: testCdmValidationReevaluator{},
	}
}

func TestNewCdmServiceRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*ServiceDependencies)
		want   string
	}{
		{"client", func(d *ServiceDependencies) { d.Client = nil }, "CDM service requires client"},
		{"strips", func(d *ServiceDependencies) { d.Strips = nil }, "CDM service requires strip store"},
		{"sessions", func(d *ServiceDependencies) { d.Sessions = nil }, "CDM service requires session repository"},
		{"controllers", func(d *ServiceDependencies) { d.Controllers = nil }, "CDM service requires controller repository"},
		{"frontend", func(d *ServiceDependencies) { d.Frontend = nil }, "CDM service requires frontend publisher"},
		{"euroscope", func(d *ServiceDependencies) { d.Euroscope = nil }, "CDM service requires EuroScope publisher"},
		{"validation", func(d *ServiceDependencies) { d.ValidationReevaluator = nil }, "CDM service requires validation reevaluator"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validCdmServiceDependencies()
			test.remove(&deps)
			_, err := NewCdmService(deps)
			require.EqualError(t, err, test.want)
		})
	}
}

func validSequenceServiceDependencies() SequenceServiceDependencies {
	return SequenceServiceDependencies{
		Strips:    &testutil.MockStripRepository{},
		Sessions:  &testutil.MockSessionRepository{},
		Config:    &stubConfigProvider{},
		Frontend:  testCdmPublisher{},
		Euroscope: testCdmEuroscope{},
	}
}

func TestNewSequenceServiceRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*SequenceServiceDependencies)
		want   string
	}{
		{"strips", func(d *SequenceServiceDependencies) { d.Strips = nil }, "CDM sequence service requires strip store"},
		{"sessions", func(d *SequenceServiceDependencies) { d.Sessions = nil }, "CDM sequence service requires session repository"},
		{"config", func(d *SequenceServiceDependencies) { d.Config = nil }, "CDM sequence service requires configuration provider"},
		{"frontend", func(d *SequenceServiceDependencies) { d.Frontend = nil }, "CDM sequence service requires frontend publisher"},
		{"EuroScope", func(d *SequenceServiceDependencies) { d.Euroscope = nil }, "CDM sequence service requires EuroScope publisher"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validSequenceServiceDependencies()
			test.remove(&deps)
			_, err := NewSequenceService(deps)
			require.EqualError(t, err, test.want)
		})
	}
}
