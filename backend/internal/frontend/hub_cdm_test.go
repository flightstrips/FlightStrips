package frontend

import (
	"FlightStrips/internal/config"
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
			Eobt:        testStringPointer("101500"),
			Tobt:        testStringPointer("102000"),
			ReqTobt:     testStringPointer("102500"),
			ReqTobtType: testStringPointer("PILOT"),
			TobtSetBy:   testStringPointer("EKCH_DEL"),
			Tsat:        testStringPointer("103000"),
			Ttot:        testStringPointer("104000"),
			Ctot:        &ctot,
			Asat:        testStringPointer("105500"),
			Asrt:        testStringPointer("103500"),
			Tsac:        testStringPointer("103500"),
			Status:      testStringPointer("READY"),
			EcfmpID:     testStringPointer("REGUL"),
			CtotSource:  testStringPointer("ATFCM"),
			Phase:       testStringPointer("I"),
		}).Normalize(),
	}

	model := MapStripToFrontendModel(strip)

	assert.Equal(t, "1015", model.Eobt)
	assert.Equal(t, "1020", model.Tobt)
	assert.Equal(t, "1025", model.ReqTobt)
	assert.Equal(t, "PILOT", model.ReqTobtType)
	assert.Equal(t, "EKCH_DEL", model.TobtSetBy)
	assert.Equal(t, "1030", model.Tsat)
	assert.Equal(t, "1040", model.Ttot)
	assert.Equal(t, "1045", model.Ctot)
	assert.Equal(t, "1055", model.Asat)
	assert.Equal(t, "1035", model.Asrt)
	assert.Equal(t, "1035", model.Tsac)
	assert.Equal(t, "READY", model.Status)
	assert.Equal(t, "REGUL", model.EcfmpID)
	assert.Equal(t, "ATFCM", model.CtotSource)
	assert.Equal(t, "I", model.Phase)
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

func TestMapStripToFrontendModel_HidesMandatoryRouteRestrictionWhenFeatureDisabled(t *testing.T) {
	t.Cleanup(config.SetFeatureFlagsForTest(config.FeatureFlagsConfig{}))

	strip := &internalModels.Strip{
		Callsign: "SAS789",
		CdmData: (&internalModels.CdmData{
			EcfmpRestrictions: []internalModels.EcfmpRestriction{
				{Type: "mandatory_route", Routes: []string{"VEDAR DCT"}},
				{Type: "ground_stop", Reason: "Weather"},
			},
		}).Normalize(),
	}

	model := MapStripToFrontendModel(strip)

	require.Len(t, model.EcfmpRestrictions, 1)
	assert.Equal(t, "ground_stop", model.EcfmpRestrictions[0].Type)
}

func TestSendCdmUpdate_TruncatesClockFields(t *testing.T) {
	hub := &Hub{send: make(chan internalMessage, 1)}

	hub.SendCdmUpdate(42, frontendEvents.CdmDataEvent{
		Callsign:    "SAS123",
		Eobt:        "101500",
		Tobt:        "102000",
		ReqTobt:     "102500",
		ReqTobtType: "PILOT",
		TobtSetBy:   "EKCH_DEL",
		Tsat:        "103000",
		Ttot:        "104000",
		Ctot:        "104500",
		Asat:        "105500",
		Asrt:        "103500",
		Tsac:        "103500",
		Status:      "READY",
		EcfmpID:     "REGUL",
		CtotSource:  "ATFCM",
		Phase:       "I",
	})

	msg := <-hub.send
	event, ok := msg.message.(frontendEvents.CdmDataEvent)
	require.True(t, ok)
	assert.Equal(t, int32(42), msg.session)
	assert.Equal(t, "SAS123", event.Callsign)
	assert.Equal(t, "101500", event.Eobt)
	assert.Equal(t, "102000", event.Tobt)
	assert.Equal(t, "102500", event.ReqTobt)
	assert.Equal(t, "PILOT", event.ReqTobtType)
	assert.Equal(t, "EKCH_DEL", event.TobtSetBy)
	assert.Equal(t, "103000", event.Tsat)
	assert.Equal(t, "104000", event.Ttot)
	assert.Equal(t, "104500", event.Ctot)
	assert.Equal(t, "105500", event.Asat)
	assert.Equal(t, "103500", event.Asrt)
	assert.Equal(t, "103500", event.Tsac)
	assert.Equal(t, "READY", event.Status)
	assert.Equal(t, "REGUL", event.EcfmpID)
	assert.Equal(t, "ATFCM", event.CtotSource)
	assert.Equal(t, "I", event.Phase)
}

func testStringPointer(value string) *string {
	return &value
}
