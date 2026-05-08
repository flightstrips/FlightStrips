package cdm

import (
	"testing"
	"time"
)

func TestDelayFloorForRunway_IgnoresFarFutureStaleDelay(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.Delays = []CdmDelay{{
		Airport: "EKCH",
		Runway:  "04L",
		Time:    "1800",
		Type:    "ADVERSE",
	}}

	_, ok := config.DelayFloorForRunway("04L", time.Date(2026, 3, 25, 8, 0, 0, 0, time.UTC))
	if ok {
		t.Fatal("expected far-future delay floor to be ignored")
	}
}

func TestDelayFloorForRunway_RollsAcrossMidnightWithinLookAhead(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.Delays = []CdmDelay{{
		Airport: "EKCH",
		Runway:  "04L",
		Time:    "0030",
		Type:    "ADVERSE",
	}}

	floor, ok := config.DelayFloorForRunway("04L", time.Date(2026, 3, 25, 23, 50, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected delay floor to roll into the next day")
	}
	if floor != "003000" {
		t.Fatalf("expected midnight rollover floor 003000, got %q", floor)
	}
}

func TestDelayFloorForRunway_ChoosesLatestMatchingFloor(t *testing.T) {
	t.Parallel()

	config := NewDefaultAirportConfig("EKCH")
	config.Delays = []CdmDelay{
		{
			Airport: "EKCH",
			Runway:  "*",
			Time:    "1015",
			Type:    "GLOBAL",
		},
		{
			Airport: "EKCH",
			Runway:  "04L",
			Time:    "1030",
			Type:    "RUNWAY",
		},
		{
			Airport: "ESSA",
			Runway:  "04L",
			Time:    "1100",
			Type:    "OTHER",
		},
	}

	floor, ok := config.DelayFloorForRunway("04L", time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC))
	if !ok {
		t.Fatal("expected matching delay floor")
	}
	if floor != "103000" {
		t.Fatalf("expected latest matching floor 103000, got %q", floor)
	}
}
