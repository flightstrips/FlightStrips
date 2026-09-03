package cdm

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
)

func TestApplyTobtRecalculationPolicyBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		tobt             string
		wantRecalculate  bool
		wantMode         string
		wantTimesCleared bool
	}{
		{name: "six minutes earlier improves only", tobt: "0954", wantRecalculate: true, wantMode: models.CdmRecalculationImproveOnly},
		{name: "minus five is protected", tobt: "0955"},
		{name: "same time is protected", tobt: "1000"},
		{name: "three minutes after is protected", tobt: "1003"},
		{name: "plus five is protected", tobt: "1005"},
		{name: "six minutes later resequences", tobt: "1006", wantRecalculate: true, wantMode: models.CdmRecalculationRequired, wantTimesCleared: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tsat, ttot, ctot := "1000", "1010", "1020"
			ctotSource := models.CtotSourceATFCM
			data := &models.CdmData{
				Tobt:       stringPtr(tc.tobt),
				Tsat:       &tsat,
				Ttot:       &ttot,
				Ctot:       &ctot,
				CtotSource: &ctotSource,
			}

			got := applyTobtRecalculationPolicy(data, tc.tobt)
			if got != tc.wantRecalculate {
				t.Fatalf("recalculation = %v, want %v", got, tc.wantRecalculate)
			}
			if data.RecalculationMode != tc.wantMode {
				t.Fatalf("recalculation mode = %q, want %q", data.RecalculationMode, tc.wantMode)
			}
			if valueOrEmpty(data.Tobt) != tc.tobt {
				t.Fatalf("TOBT = %q, want %q", valueOrEmpty(data.Tobt), tc.tobt)
			}
			if tc.wantTimesCleared {
				if data.Tsat != nil || data.Ttot != nil || data.Ctot != nil || data.CtotSource != nil {
					t.Fatalf("expected automatic assignment to be cleared, got %#v", data)
				}
				return
			}
			if valueOrEmpty(data.Tsat) != tsat || valueOrEmpty(data.Ttot) != ttot || valueOrEmpty(data.Ctot) != ctot || valueOrEmpty(data.CtotSource) != ctotSource {
				t.Fatalf("expected assignment to be preserved, got %#v", data)
			}
		})
	}
}

func TestApplyTobtRecalculationPolicyProtectsAcrossMidnight(t *testing.T) {
	tsat, ttot, ctot := "2358", "0008", "0015"
	ctotSource := models.CtotSourceATFCM
	data := &models.CdmData{
		Tobt:       stringPtr("0002"),
		Tsat:       &tsat,
		Ttot:       &ttot,
		Ctot:       &ctot,
		CtotSource: &ctotSource,
	}

	if applyTobtRecalculationPolicy(data, "0002") {
		t.Fatal("expected four-minute midnight crossing to remain protected")
	}
	if valueOrEmpty(data.Tsat) != tsat || valueOrEmpty(data.Ttot) != ttot || valueOrEmpty(data.Ctot) != ctot {
		t.Fatalf("expected midnight assignment to be preserved, got %#v", data)
	}
}

func TestApplyTobtRecalculationPolicyPreservesManualCtotForResequence(t *testing.T) {
	tsat, ttot, ctot := "1000", "1010", "1020"
	ctotSource := models.CtotSourceManual
	data := &models.CdmData{
		Tobt:       stringPtr("1006"),
		Tsat:       &tsat,
		Ttot:       &ttot,
		Ctot:       &ctot,
		CtotSource: &ctotSource,
	}

	if !applyTobtRecalculationPolicy(data, "1006") {
		t.Fatal("expected later TOBT to require resequencing")
	}
	if data.Tsat != nil || data.Ttot != nil {
		t.Fatalf("expected old TSAT/TTOT to be cleared, got %#v", data)
	}
	if valueOrEmpty(data.Ctot) != ctot || valueOrEmpty(data.CtotSource) != ctotSource {
		t.Fatalf("expected manual CTOT to remain as a calculation constraint, got %#v", data)
	}
}

func TestApplyTobtRecalculationPolicyPreservesRequiredRecalculation(t *testing.T) {
	tests := []struct {
		name string
		tobt string
	}{
		{name: "protected TOBT keeps unrelated recalculation", tobt: "1003"},
		{name: "earlier TOBT does not downgrade recalculation", tobt: "0954"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := &models.CdmData{
				Tobt:              stringPtr(tc.tobt),
				Tsat:              stringPtr("1000"),
				Ttot:              stringPtr("1010"),
				Recalculate:       true,
				RecalculationMode: models.CdmRecalculationRequired,
			}

			if !applyTobtRecalculationPolicy(data, tc.tobt) {
				t.Fatal("expected the existing recalculation to remain scheduled")
			}
			if !data.NeedsLocalRecalculation() || data.RecalculationMode != models.CdmRecalculationRequired {
				t.Fatalf("expected required recalculation to be preserved, got %#v", data)
			}
		})
	}
}

func TestApplyTobtRecalculationPolicyCancelsSupersededImprovementInsideWindow(t *testing.T) {
	data := &models.CdmData{
		Tobt:              stringPtr("0955"),
		Tsat:              stringPtr("1000"),
		Ttot:              stringPtr("1010"),
		Recalculate:       true,
		RecalculationMode: models.CdmRecalculationImproveOnly,
	}

	if applyTobtRecalculationPolicy(data, "0955") {
		t.Fatal("expected the superseded improvement to be cancelled inside the protected window")
	}
	if data.NeedsLocalRecalculation() || data.RecalculationMode != "" {
		t.Fatalf("expected no pending improvement, got %#v", data)
	}
	if valueOrEmpty(data.Tsat) != "1000" || valueOrEmpty(data.Ttot) != "1010" {
		t.Fatalf("expected the protected assignment to remain frozen, got %#v", data)
	}
}

func TestPrepareTobtUpdateClearsManualCtotWhenDeicingMakesItUnachievable(t *testing.T) {
	ctotSource := models.CtotSourceManual
	stored := (&models.CdmData{
		Tobt:       stringPtr("1000"),
		Tsat:       stringPtr("1000"),
		Ttot:       stringPtr("1010"),
		Ctot:       stringPtr("1020"),
		CtotSource: &ctotSource,
		DeIce:      stringPtr("M"),
	}).Normalize()

	service := newTestCdmService(
		newTestClientWithAirportMasters(nil),
		&testutil.MockStripRepository{
			GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
				return &models.Strip{Callsign: "SASDEICE", Origin: "EKCH", Runway: stringPtr("22R")}, nil
			},
			GetCdmDataForCallsignFn: func(context.Context, int32, string) (*models.CdmData, error) {
				return stored.Clone(), nil
			},
		},
		&testutil.MockSessionRepository{},
		&testutil.MockControllerRepository{},
	)
	service.SetConfigProvider(&stubConfigProvider{config: &CdmAirportConfig{
		Airport:            "EKCH",
		DefaultTaxiMinutes: 10,
		DeiceConfig: CdmDeiceConfig{
			Medium: 10,
		},
	}})

	_, _, updated, _, changed, recalculate, err := service.actionService.prepareTobtUpdate(
		context.Background(), 7, "SASDEICE", "1006", "EKCH_DEL", "ATC", time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("prepareTobtUpdate returned error: %v", err)
	}
	if !changed || !recalculate {
		t.Fatal("expected the later TOBT to require resequencing")
	}
	if updated.Ctot != nil || updated.CtotSource != nil {
		t.Fatalf("expected deicing to make the manual CTOT unachievable, got %#v", updated)
	}
}
