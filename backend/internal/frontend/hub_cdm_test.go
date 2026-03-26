package frontend

import (
	internalModels "FlightStrips/internal/models"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapStripToFrontendModel_TruncatesCdmTimes(t *testing.T) {
	ctot := "104500"
	strip := &internalModels.Strip{
		Callsign: "SAS123",
		CdmData: (&internalModels.CdmData{
			Eobt: testStringPointer("101500"),
			Tobt: testStringPointer("102000"),
			Tsat: testStringPointer("103000"),
			Ctot: &ctot,
		}).Normalize(),
	}

	model := MapStripToFrontendModel(strip)

	assert.Equal(t, "1015", model.Eobt)
	assert.Equal(t, "1020", model.Tobt)
	assert.Equal(t, "1030", model.Tsat)
	assert.Equal(t, "1045", model.Ctot)
}

func TestMapStripToFrontendModel_TruncatesCtot(t *testing.T) {
	ctot := "103000"
	strip := &internalModels.Strip{
		Callsign: "SAS456",
		CdmData: (&internalModels.CdmData{
			Ctot: &ctot,
		}).Normalize(),
	}

	model := MapStripToFrontendModel(strip)

	assert.Equal(t, "1030", model.Ctot)
}

func TestSendCdmUpdate_TruncatesClockFields(t *testing.T) {
	hub := &Hub{send: make(chan internalMessage, 1)}

	hub.SendCdmUpdate(42, "SAS123", "101500", "102000", "103000", "104500")

	msg := <-hub.send
	event, ok := msg.message.(frontendEvents.CdmDataEvent)
	require.True(t, ok)
	assert.Equal(t, int32(42), msg.session)
	assert.Equal(t, "SAS123", event.Callsign)
	assert.Equal(t, "1015", event.Eobt)
	assert.Equal(t, "1020", event.Tobt)
	assert.Equal(t, "1030", event.Tsat)
	assert.Equal(t, "1045", event.Ctot)
}

func testStringPointer(value string) *string {
	return &value
}
