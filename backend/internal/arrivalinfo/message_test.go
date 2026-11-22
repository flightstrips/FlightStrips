package arrivalinfo

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/models"
)

func TestDelayTextBandBoundaries(t *testing.T) {
	over := time.Second
	cases := []struct {
		delay time.Duration
		want  string
	}{
		{-10 * time.Minute, "NO DELAY EXPECTED."},
		{0, "NO DELAY EXPECTED."},
		{5*time.Minute - over, "NO DELAY EXPECTED."},
		{5 * time.Minute, "DELAY LESS THAN 5MIN."},
		{5*time.Minute + over, "DELAY LESS THAN 5MIN."},
		{7 * time.Minute, "DELAY LESS THAN 5MIN."},
		{7*time.Minute + over, "DELAY 5-10MIN."},
		{12 * time.Minute, "DELAY 5-10MIN."},
		{12*time.Minute + over, "DELAY 10-20MIN."},
		{22 * time.Minute, "DELAY 10-20MIN."},
		{22*time.Minute + over, "DELAY MORE THAN 20MIN."},
		{90 * time.Minute, "DELAY MORE THAN 20MIN."},
	}
	for _, tc := range cases {
		if got := DelayText(tc.delay); got != tc.want {
			t.Errorf("DelayText(%v) = %q, want %q", tc.delay, got, tc.want)
		}
	}
}

func TestDelayIsSlotMinusOperationalTETAUnrounded(t *testing.T) {
	teta := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	flight := aman.AMANFlight{
		Prediction: &aman.Prediction{OperationalTETA: teta},
		Slot:       &aman.Slot{Time: teta.Add(7*time.Minute + 500*time.Millisecond)},
	}
	delay, ok := Delay(flight)
	if !ok || delay != 7*time.Minute+500*time.Millisecond {
		t.Fatalf("Delay = %v, %v", delay, ok)
	}
	if got := DelayText(delay); got != "DELAY 5-10MIN." {
		t.Fatalf("7m00.5s delay = %q", got)
	}
	if _, ok := Delay(aman.AMANFlight{Prediction: flight.Prediction}); ok {
		t.Fatal("Delay without slot must be unknown")
	}
}

func TestArrivalInfoTextFormat(t *testing.T) {
	cases := []struct {
		name, stand, star string
		delay             time.Duration
		want              string
	}{
		{"full", "C27", "MONAK5C", 10 * time.Minute, "EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY 5-10MIN."},
		{"no stand", "", "MONAK5C", 0, "EKCH ARR INFO: MONAK5C. NO DELAY EXPECTED."},
		{"no star", "C27", "", 30 * time.Minute, "EKCH ARR INFO: EXP STAND C27. DELAY MORE THAN 20MIN."},
		{"neither", "", "", 6 * time.Minute, "EKCH ARR INFO: DELAY LESS THAN 5MIN."},
		{"lowercase input", " c27 ", "monak5c", 15 * time.Minute, "EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY 10-20MIN."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ArrivalInfoText("ekch", tc.stand, tc.star, tc.delay); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStandUpdateText(t *testing.T) {
	if got := StandUpdateText("EKCH", "d2"); got != "EKCH ARR INFO: NEW STAND D2." {
		t.Fatalf("got %q", got)
	}
}

func testTerminal() *terminal.Configuration {
	return &terminal.Configuration{
		RunwayGroups: []terminal.RunwayGroup{
			{ID: "ARRIVAL-22L", Aliases: []aman.RunwayGroupID{"22L"}},
			{ID: "ARRIVAL-04L"},
		},
		Paths: []terminal.Path{
			{Feeder: "MONAK", STARFamily: "MONAK", FeederFix: "NEKSO", RunwayGroup: "ARRIVAL-22L", STARSuffix: "5C"},
			{Feeder: "MONAK", STARFamily: "MONAK", FeederFix: "NEKSO", RunwayGroup: "ARRIVAL-04L", STARSuffix: "5A"},
			{Feeder: "TUDLO", STARFamily: "TUDLO", FeederFix: "KOR", RunwayGroup: "ARRIVAL-04L", STARSuffix: "4A"},
			{Feeder: "TUDLO", STARFamily: "TUDLO", FeederFix: "KOR", RunwayGroup: "ARRIVAL-22L", STARSuffix: "4A"},
		},
	}
}

func ptr[T any](value T) *T { return &value }

func TestExpectedSTAR(t *testing.T) {
	config := testTerminal()
	group := aman.RunwayGroupID("ARRIVAL-22L")
	cases := []struct {
		name   string
		strip  *models.Strip
		flight aman.AMANFlight
		want   string
	}{
		{"assigned on strip wins", &models.Strip{Star: ptr("lango2a")}, aman.AMANFlight{SelectedSTARFamily: ptr("MONAK"), SelectedRunwayGroup: &group}, "LANGO2A"},
		{"assigned on observation", &models.Strip{}, aman.AMANFlight{LatestObservation: &aman.FlightObservation{AssignedSTAR: ptr("MONAK5C")}}, "MONAK5C"},
		{"derived from family and runway group", &models.Strip{Star: ptr(" ")}, aman.AMANFlight{SelectedSTARFamily: ptr("MONAK"), SelectedRunwayGroup: &group}, "MONAK5C"},
		{"derived through runway group alias", nil, aman.AMANFlight{SelectedSTARFamily: ptr("MONAK"), SelectedRunwayGroup: ptr(aman.RunwayGroupID("22L"))}, "MONAK5C"},
		{"derived from legacy feeder field", nil, aman.AMANFlight{SelectedFeeder: ptr("MONAK"), SelectedRunwayGroup: &group}, "MONAK5C"},
		{"same suffix on all runways", nil, aman.AMANFlight{SelectedSTARFamily: ptr("TUDLO")}, "TUDLO4A"},
		{"ambiguous without runway group", nil, aman.AMANFlight{SelectedSTARFamily: ptr("MONAK")}, ""},
		{"unknown family", nil, aman.AMANFlight{SelectedSTARFamily: ptr("LANGO"), SelectedRunwayGroup: &group}, ""},
		{"no family", nil, aman.AMANFlight{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExpectedSTAR(tc.strip, tc.flight, config); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := ExpectedSTAR(nil, aman.AMANFlight{SelectedSTARFamily: ptr("MONAK"), SelectedRunwayGroup: &group}, nil); got != "" {
		t.Fatalf("without terminal config got %q", got)
	}
}

func TestExpectedStand(t *testing.T) {
	if got := ExpectedStand(&models.Strip{Stand: ptr("c27")}); got != "C27" {
		t.Fatalf("got %q", got)
	}
	if got := ExpectedStand(&models.Strip{}); got != "" {
		t.Fatalf("got %q", got)
	}
}
