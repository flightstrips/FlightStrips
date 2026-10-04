package services

import (
	"testing"
	"time"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
)

type logonTimingCdmService struct {
	spyStripCdmService
	data *models.CdmData
}

func (s *logonTimingCdmService) PrepareEuroscopeLogonSync(*models.CdmData, string, time.Time) *models.CdmData {
	return s.data.Clone()
}

func TestPrepareEuroscopeLogonSyncReturnsEobtCorrection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		eobt      string
		tobt      string
		corrected bool
	}{
		{name: "automatic adjustment", eobt: "1230", tobt: "1230", corrected: true},
		{name: "confirmed different TOBT", eobt: "1400", tobt: "1230"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &StripService{cdmService: &logonTimingCdmService{
				data: &models.CdmData{Eobt: &tc.eobt, Tobt: &tc.tobt},
			}}
			updated, eobt, corrected := service.prepareEuroscopeLogonSync(1, &models.CdmData{}, "1400", time.Now().UTC())
			assert.Equal(t, tc.eobt, eobt)
			assert.Equal(t, tc.corrected, corrected)
			assert.Equal(t, tc.tobt, *updated.Tobt)
		})
	}
}
