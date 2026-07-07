package shared

import (
	"FlightStrips/internal/models"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildEuroscopeBackendSyncCdmData_MatchesCdmUpdateFields(t *testing.T) {
	data := (&models.CdmData{
		Eobt:            testStringPointer("101500"),
		Tobt:            testStringPointer("102000"),
		TobtSetBy:       testStringPointer("EKCH_DEL"),
		TobtConfirmedBy: testStringPointer("ATC"),
		ReqTobt:         testStringPointer("102500"),
		ReqTobtType:     testStringPointer("PILOT"),
		Tsat:            testStringPointer("103000"),
		Ttot:            testStringPointer("104000"),
		Ctot:            testStringPointer("104500"),
		CtotSource:      testStringPointer("ATFCM"),
		Asat:            testStringPointer("105500"),
		Asrt:            testStringPointer("103500"),
		Tsac:            testStringPointer("103700"),
		Status:          testStringPointer("READY"),
		EcfmpID:         testStringPointer("REGUL"),
		Phase:           testStringPointer("I"),
		EcfmpRestrictions: []models.EcfmpRestriction{
			{MeasureID: 12, Ident: "REGUL", Type: "ground_stop", Reason: "Weather", Routes: []string{"VEDAR DCT"}, HasCtot: true},
		},
	}).Normalize()

	update := BuildEuroscopeCdmUpdateEvent("SAS123", data)
	sync := BuildEuroscopeBackendSyncCdmData(data)

	assert.Equal(t, euroscopeEvents.BackendSyncCdmData{
		Eobt:                  update.Eobt,
		Tobt:                  update.Tobt,
		TobtSetBy:             update.TobtSetBy,
		TobtConfirmedBy:       update.TobtConfirmedBy,
		ReqTobt:               update.ReqTobt,
		ReqTobtType:           update.ReqTobtType,
		Tsat:                  update.Tsat,
		Ttot:                  update.Ttot,
		Ctot:                  update.Ctot,
		CtotSource:            update.CtotSource,
		Asat:                  update.Asat,
		Asrt:                  update.Asrt,
		Tsac:                  update.Tsac,
		Status:                update.Status,
		EcfmpID:               update.EcfmpID,
		Phase:                 update.Phase,
		EcfmpRestrictionsJSON: update.EcfmpRestrictionsJSON,
	}, sync)
}

func testStringPointer(value string) *string {
	return &value
}
