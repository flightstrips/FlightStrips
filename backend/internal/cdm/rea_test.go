package cdm

import (
	"FlightStrips/internal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReaRequestTobtResetsTobtOnRepeatedRea(t *testing.T) {
	now := time.Date(2026, 10, 12, 14, 50, 0, 0, time.UTC)

	for _, currentTobt := range []string{"1453", ""} {
		t.Run(currentTobt, func(t *testing.T) {
			status := "REA"
			data := &models.CdmData{Status: &status}
			if currentTobt != "" {
				data.Tobt = &currentTobt
			}

			tobt, change := reaRequestTobt(data, now)

			assert.True(t, change)
			assert.Equal(t, "1450", tobt)
		})
	}
}
