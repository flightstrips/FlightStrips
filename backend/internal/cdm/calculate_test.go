package cdm

import (
	"testing"
	"time"
)

func TestCalculate_BaseTimeSelection(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		input        CalcInput
		expectedTsat string
		expectedTtot string
	}{
		{
			name: "prefers tobt over requested tobt and eobt",
			input: CalcInput{
				Callsign: "SAS100",
				Origin:   "EKCH",
				DepRwy:   "04L",
				Eobt:     "0940",
				Tobt:     "1000",
				ReqTobt:  "1015",
				TaxiMin:  10,
			},
			expectedTsat: "100000",
			expectedTtot: "101000",
		},
		{
			name: "uses requested tobt when tobt missing",
			input: CalcInput{
				Callsign: "SAS101",
				Origin:   "EKCH",
				DepRwy:   "04L",
				Eobt:     "0940",
				ReqTobt:  "1015",
				TaxiMin:  10,
			},
			expectedTsat: "101500",
			expectedTtot: "102500",
		},
		{
			name: "falls back to eobt when no tobt exists",
			input: CalcInput{
				Callsign: "SAS102",
				Origin:   "EKCH",
				DepRwy:   "04L",
				Eobt:     "1020",
				TaxiMin:  10,
			},
			expectedTsat: "102000",
			expectedTtot: "103000",
		},
		{
			name: "ignores zero tobt and requested tobt and falls back to eobt",
			input: CalcInput{
				Callsign: "SAS103",
				Origin:   "EKCH",
				DepRwy:   "04L",
				Eobt:     "1020",
				Tobt:     "0000",
				ReqTobt:  "0000",
				TaxiMin:  10,
			},
			expectedTsat: "102000",
			expectedTtot: "103000",
		},
		{
			name: "returns empty result when all base times are zero",
			input: CalcInput{
				Callsign: "SAS104",
				Origin:   "EKCH",
				DepRwy:   "04L",
				Eobt:     "0000",
				Tobt:     "0000",
				ReqTobt:  "0000",
				TaxiMin:  10,
			},
			expectedTsat: "",
			expectedTtot: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := Calculate(tt.input, nil, NewDefaultAirportConfig("EKCH"), now)
			assertClockResult(t, result, tt.expectedTsat, tt.expectedTtot)
		})
	}
}

func TestCalculate_UsesManualCtotFloor(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign:   "SAS123",
		Origin:     "EKCH",
		DepRwy:     "04L",
		Tobt:       "1000",
		TaxiMin:    10,
		HasManCtot: true,
		ManCtot:    "1025",
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "101500", "102500")
}

func TestCalculate_UsesApiCtotFloor(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS123",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		Ctot:     "1030",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "102000", "103000")
}

func TestCalculate_ManualCtotStillWinsWhenLaterThanApiCtot(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign:   "SAS123",
		Origin:     "EKCH",
		DepRwy:     "04L",
		Tobt:       "1000",
		Ctot:       "1020",
		TaxiMin:    10,
		HasManCtot: true,
		ManCtot:    "1035",
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "102500", "103500")
}

func TestCalculate_ApiCtotDoesNotPullEarlierThanNaturalTtot(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS123",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		Ctot:     "1005",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_ManualCtotDoesNotPullEarlierThanNaturalTtot(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign:   "SAS124",
		Origin:     "EKCH",
		DepRwy:     "04L",
		Tobt:       "1000",
		TaxiMin:    10,
		HasManCtot: true,
		ManCtot:    "1005",
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_AppliesRateWindowSpacing(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 20

	result := Calculate(CalcInput{
		Callsign: "SAS456",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS123",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Ttot:     "101100",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100400", "101400")
}

func TestCalculate_IgnoresRateWindowForDifferentOrigin(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 20

	result := Calculate(CalcInput{
		Callsign: "SAS460",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS999",
		Origin:   "ESSA",
		DepRwy:   "04L",
		Ttot:     "101100",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_IgnoresRateWindowForIndependentRunways(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 20

	result := Calculate(CalcInput{
		Callsign: "SAS461",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS998",
		Origin:   "EKCH",
		DepRwy:   "22R",
		Ttot:     "101100",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_AppliesRateWindowToDependentRunways(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.Rates = []CdmRate{{
		Airport:      "EKCH",
		DepRwyYes:    []string{"04L"},
		DependentRwy: []string{"22R"},
		Rates:        []string{"20"},
	}}

	result := Calculate(CalcInput{
		Callsign: "SAS462",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS997",
		Origin:   "EKCH",
		DepRwy:   "22R",
		Ttot:     "101100",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100400", "101400")
}

func TestCalculate_AppliesSidIntervalSpacing(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 60
	config.SidIntervals = []CdmSidInterval{{
		Airport: "EKCH",
		Runway:  "04L",
		Sid1:    "MIKLA1A",
		Sid2:    "NEXEN1A",
		Value:   5,
	}}

	result := Calculate(CalcInput{
		Callsign: "SAS463",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "MIKLA1A",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS996",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "NEXEN1A",
		Ttot:     "101300",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100800", "101800")
}

func TestCalculate_CtotFloorStillHonorsStrongerSidSeparation(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 20
	config.SidIntervals = []CdmSidInterval{{
		Airport: "EKCH",
		Runway:  "04L",
		Sid1:    "MIKLA1A",
		Sid2:    "NEXEN1A",
		Value:   5,
	}}

	result := Calculate(CalcInput{
		Callsign: "SAS463",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "MIKLA1A",
		Tobt:     "1000",
		Ctot:     "1030",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS996",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "NEXEN1A",
		Ttot:     "103200",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "102700", "103700")
}

func TestCalculate_SkipsSidIntervalForEquivalentSidVariants(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 60
	config.SidIntervals = []CdmSidInterval{{
		Airport: "EKCH",
		Runway:  "04L",
		Sid1:    "MIKLA1A",
		Sid2:    "NEXEN1A",
		Value:   5,
	}}

	result := Calculate(CalcInput{
		Callsign: "SAS464",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "MIKLA1B",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS995",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Sid:      "MIKLA1A",
		Ttot:     "101300",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_SkipsRateWindowWhenOnlyOneFlightHasManualCtot(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 20

	result := Calculate(CalcInput{
		Callsign: "SAS465",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign:   "SAS994",
		Origin:     "EKCH",
		DepRwy:     "04L",
		Ttot:       "101100",
		HasManCtot: true,
		ManCtot:    "1015",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_ResolvesExactConflictInThirtySecondSteps(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.DefaultRate = 60

	result := Calculate(CalcInput{
		Callsign: "SAS466",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1001",
		TaxiMin:  10,
	}, []SlotEntry{{
		Callsign: "SAS993",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Ttot:     "101100",
	}}, config, time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "100200", "101200")
}

func TestCalculate_KeepsTsatWhenItIsWithinFiveMinutesInThePast(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS467",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 10, 5, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_ReturnsEmptyWhenTobtIsMoreThanFiveMinutesPastWithoutStartup(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS468",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 10, 6, 0, 0, time.UTC))

	assertClockResult(t, result, "", "")
}

func TestCalculate_ReturnsEmptyWhenTsatIsMoreThanFiveMinutesPastWithoutStartup(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS469",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Eobt:     "1000",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 10, 6, 0, 0, time.UTC))

	assertClockResult(t, result, "", "")
}

func TestCalculate_PreservesPastTimesAfterStartup(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS470",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "1000",
		Asat:     "1004",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 10, 6, 0, 0, time.UTC))

	assertClockResult(t, result, "100000", "101000")
}

func TestCalculate_WrapsAcrossMidnight(t *testing.T) {
	t.Parallel()

	result := Calculate(CalcInput{
		Callsign: "SAS471",
		Origin:   "EKCH",
		DepRwy:   "04L",
		Tobt:     "2358",
		TaxiMin:  10,
	}, nil, NewDefaultAirportConfig("EKCH"), time.Date(2026, 3, 25, 22, 0, 0, 0, time.UTC))

	assertClockResult(t, result, "235800", "000800")
}

func assertClockResult(t *testing.T, result CalcResult, expectedTsat, expectedTtot string) {
	t.Helper()

	if result.Tsat != expectedTsat {
		t.Fatalf("expected TSAT %q, got %q", expectedTsat, result.Tsat)
	}
	if result.Ttot != expectedTtot {
		t.Fatalf("expected TTOT %q, got %q", expectedTtot, result.Ttot)
	}
}
