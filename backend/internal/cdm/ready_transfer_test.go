package cdm

import (
	"testing"
	"time"

	"FlightStrips/internal/models"

	"github.com/stretchr/testify/assert"
)

func TestReadyTransferTobtUsesTsatOnly(t *testing.T) {
	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		tsat       string
		wantTobt   string
		wantUpdate bool
		wantBlock  bool
	}{
		{name: "seven minutes ahead updates TOBT to now", tsat: "1207", wantTobt: "1200", wantUpdate: true, wantBlock: true},
		{name: "six minutes ahead does not update TOBT and blocks", tsat: "1206", wantTobt: "1145", wantBlock: true},
		{name: "five minutes ahead leaves TOBT unchanged", tsat: "1205", wantTobt: "1145"},
		{name: "seven minutes past leaves TOBT unchanged", tsat: "1153", wantTobt: "1145"},
		{name: "eight minutes past blocks without changing TOBT", tsat: "1152", wantTobt: "1145", wantBlock: true},
		{name: "missing TSAT blocks without changing TOBT", wantTobt: "1145", wantBlock: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tobt := "1145"
			data := &models.CdmData{Tobt: &tobt}
			if tc.tsat != "" {
				data.Tsat = &tc.tsat
			}
			gotTobt, gotUpdate, gotBlock := readyTransferTobt(data, now)
			assert.Equal(t, tc.wantTobt, gotTobt)
			assert.Equal(t, tc.wantUpdate, gotUpdate)
			assert.Equal(t, tc.wantBlock, gotBlock)
		})
	}
}

func TestTsatWithinFiveMinutes(t *testing.T) {
	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	assert.True(t, tsatWithinFiveMinutes("1205", now))
	assert.True(t, tsatWithinFiveMinutes("1155", now))
	assert.False(t, tsatWithinFiveMinutes("1206", now))
	assert.False(t, tsatWithinFiveMinutes("", now))
}

func TestValidReadyTransferTsatBoundaries(t *testing.T) {
	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	assert.True(t, validReadyTransferTsat("1205", now))
	assert.False(t, validReadyTransferTsat("1206", now))
	assert.True(t, validReadyTransferTsat("1153", now))
	assert.False(t, validReadyTransferTsat("1152", now))
	assert.False(t, validReadyTransferTsat("", now))
}
