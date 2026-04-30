package clx

import (
	"FlightStrips/internal/models"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSidAircraftTypeFaults(t *testing.T) {
	tests := []struct {
		name           string
		sid            string
		engineType     string
		route          string
		expectedRemark string
	}{
		{
			name:           "jet on KOPEX family",
			sid:            "KOPEX2A",
			engineType:     "J",
			route:          "MICOS T503",
			expectedRemark: "Reclear on NEXEN T503 MICOS... as filed",
		},
		{
			name:           "prop on LANGO family",
			sid:            "LANGO3B",
			engineType:     "P",
			route:          "ALS P999",
			expectedRemark: "Reclear on LANGO P999 AMRAK... as filed",
		},
		{
			name:           "turboprop on NEXEN family",
			sid:            "NEXEN1D",
			engineType:     "T",
			route:          "ALASA M611",
			expectedRemark: "Reclear on LANGO M611 ALASA... as filed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strip := testStrip()
			strip.Sid = ptr(tt.sid)
			strip.EngineType = tt.engineType
			strip.Route = ptr(tt.route)

			validation := Validate(strip, testContext())
			fault := requireFault(t, validation, "sid_aircraft_type")

			assert.Equal(t, []string{FieldSID}, fault.Fields)
			assert.Contains(t, fault.NitosRemark, tt.expectedRemark)
		})
	}
}

func TestValidateCategoryFRunwayAndSidSuffix(t *testing.T) {
	strip := testStrip()
	strip.AircraftType = ptr("A388/M-SDE2FGHIRWY/LB1")
	strip.Runway = ptr("22R")
	strip.Sid = ptr("NEXEN2D")

	validation := Validate(strip, testContext())
	fault := requireFault(t, validation, "category_f_runway")

	assert.ElementsMatch(t, []string{FieldRunway, FieldSID}, fault.Fields)
	assert.Equal(t, "Planned RWY not available for aircraft Category (CAT F). Only 04R/22L approved", fault.NitosRemark)
}

func TestValidateRnavFaults(t *testing.T) {
	tests := []struct {
		name         string
		sid          *string
		route        string
		remarks      string
		expectedCode string
	}{
		{
			name:         "explicit SID with no PBN",
			sid:          ptr("NEXEN2A"),
			route:        "DCT MICOS",
			remarks:      "REG/OYABC",
			expectedCode: "rnav_nil",
		},
		{
			name:         "explicit SID with RNAV 10",
			sid:          ptr("NEXEN2A"),
			route:        "DCT MICOS",
			remarks:      "PBN/A1",
			expectedCode: "rnav_insufficient",
		},
		{
			name:         "known SID first waypoint without explicit SID",
			sid:          nil,
			route:        "BETUD DCT",
			remarks:      "REG/OYABC",
			expectedCode: "rnav_nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strip := testStrip()
			strip.Sid = tt.sid
			strip.Route = ptr(tt.route)
			strip.Remarks = ptr(tt.remarks)

			validation := Validate(strip, testContext())
			fault := requireFault(t, validation, tt.expectedCode)

			assert.Equal(t, []string{FieldRNAV}, fault.Fields)
		})
	}
}

func TestValidateRouteSidFaultsAndOverrideSuppression(t *testing.T) {
	strip := testStrip()
	strip.Sid = ptr("LANGO2A")
	strip.Route = ptr("ARTEX DCT")

	validation := Validate(strip, testContext())
	fault := requireFault(t, validation, "route_lango_egpx")
	require.NotEmpty(t, fault.OverrideKey)
	assert.Contains(t, fault.OverrideKey, "|SAS123|")

	suppressed := Validate(strip, Context{
		Now:       testNow(),
		Overrides: map[string]bool{fault.OverrideKey: true},
	})
	assertNoFault(t, suppressed, "route_lango_egpx")

	otherStrip := *strip
	otherStrip.Callsign = "SAS456"
	otherValidation := Validate(&otherStrip, Context{
		Now:       testNow(),
		Overrides: map[string]bool{fault.OverrideKey: true},
	})
	requireFault(t, otherValidation, "route_lango_egpx")
}

func TestValidateRouteSidSpecificRules(t *testing.T) {
	tests := []struct {
		name string
		sid  string
		text string
		code string
	}{
		{name: "VEDAR with EKDK remarks", sid: "VEDAR2A", text: "RMK/EKDK/", code: "route_vedar_ekdk"},
		{name: "BETUD always invalid", sid: "BETUD2A", text: "DCT", code: "route_betud"},
		{name: "SIMEG with SALLO later in route", sid: "SIMEG2A", text: "DCT SALLO", code: "route_simeg_sallo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			strip := testStrip()
			strip.Sid = ptr(tt.sid)
			strip.Route = ptr(tt.text)

			validation := Validate(strip, testContext())
			requireFault(t, validation, tt.code)
		})
	}
}

func TestValidatePastEobtAndTobt(t *testing.T) {
	strip := testStrip()
	strip.CdmData = &models.CdmData{
		Eobt: ptr("1000"),
		Tobt: ptr("1005"),
	}

	validation := Validate(strip, Context{Now: time.Date(2026, 1, 1, 10, 10, 0, 0, time.UTC)})
	fault := requireFault(t, validation, "eobt_tobt_past")

	assert.ElementsMatch(t, []string{FieldEOBT, FieldTOBT}, fault.Fields)
}

func TestValidatePastEobtAndTobtHandlesMidnightRollover(t *testing.T) {
	futureAfterMidnight := testStrip()
	futureAfterMidnight.CdmData = &models.CdmData{
		Eobt: ptr("0005"),
		Tobt: ptr("0010"),
	}
	validation := Validate(futureAfterMidnight, Context{Now: time.Date(2026, 1, 1, 23, 55, 0, 0, time.UTC)})
	assertNoFault(t, validation, "eobt_tobt_past")

	pastBeforeMidnight := testStrip()
	pastBeforeMidnight.CdmData = &models.CdmData{
		Eobt: ptr("2350"),
		Tobt: ptr("2355"),
	}
	validation = Validate(pastBeforeMidnight, Context{Now: time.Date(2026, 1, 2, 0, 10, 0, 0, time.UTC)})
	requireFault(t, validation, "eobt_tobt_past")
}

func testStrip() *models.Strip {
	return &models.Strip{
		Callsign:     "SAS123",
		AircraftType: ptr("B738/M-SDE2FGHIRWY/LB1"),
		Remarks:      ptr("PBN/A1B1C1D1S1S2"),
		Route:        ptr("DCT"),
		Sid:          ptr("NEXEN2A"),
		Runway:       ptr("22L"),
		EngineType:   "J",
	}
}

func testContext() Context {
	return Context{Now: testNow()}
}

func testNow() time.Time {
	return time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
}

func requireFault(t *testing.T, validation *Validation, code string) Fault {
	t.Helper()
	require.NotNil(t, validation)
	for _, fault := range validation.Faults {
		if fault.Code == code {
			return fault
		}
	}
	require.Failf(t, "missing fault", "expected %s in [%s]", code, strings.Join(faultCodes(validation), ", "))
	return Fault{}
}

func assertNoFault(t *testing.T, validation *Validation, code string) {
	t.Helper()
	if validation == nil {
		return
	}
	assert.False(t, slices.Contains(faultCodes(validation), code), "unexpected fault %s", code)
}

func faultCodes(validation *Validation) []string {
	if validation == nil {
		return nil
	}
	codes := make([]string, 0, len(validation.Faults))
	for _, fault := range validation.Faults {
		codes = append(codes, fault.Code)
	}
	return codes
}

func ptr[T any](value T) *T {
	return &value
}
