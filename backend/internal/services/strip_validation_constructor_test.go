package services

import (
	"FlightStrips/internal/testutil"
	"testing"

	"github.com/stretchr/testify/require"
)

func validStripValidationDependencies() StripValidationDependencies {
	strips := &testutil.MockStripRepository{}
	return StripValidationDependencies{
		Strips: strips, Statuses: strips, Publisher: testStripValidationPublisher{},
	}
}

func TestNewStripValidationServiceRejectsMissingRequiredDependencies(t *testing.T) {
	tests := []struct {
		name   string
		remove func(*StripValidationDependencies)
		want   string
	}{
		{"strip reader", func(d *StripValidationDependencies) { d.Strips = nil }, "strip validation service requires strip reader"},
		{"status store", func(d *StripValidationDependencies) { d.Statuses = nil }, "strip validation service requires validation status store"},
		{"publisher", func(d *StripValidationDependencies) { d.Publisher = nil }, "strip validation service requires strip update publisher"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			deps := validStripValidationDependencies()
			test.remove(&deps)
			_, err := NewStripValidationService(deps)
			require.EqualError(t, err, test.want)
		})
	}
}
