package models

import "testing"

func TestCdmDataNormalize_DropsEmptyCalculationSnapshot(t *testing.T) {
	data := (&CdmData{Calculation: &CdmCalculation{}}).Normalize()

	if data.Calculation != nil {
		t.Fatalf("expected empty calculation snapshot to be removed, got %#v", data.Calculation)
	}
}

func TestCdmDataClone_DeepCopiesCalculation(t *testing.T) {
	minutes := 12
	runway := "04L"
	baseSource := CdmCalculationBaseTobt

	original := &CdmData{
		Calculation: &CdmCalculation{
			BaseSource:  &baseSource,
			TaxiMinutes: &minutes,
			TaxiRunway:  &runway,
		},
	}

	clone := original.Clone()
	*clone.Calculation.TaxiMinutes = 18
	*clone.Calculation.TaxiRunway = "22L"

	if *original.Calculation.TaxiMinutes != 12 {
		t.Fatalf("expected original calculation taxi minutes to remain 12, got %d", *original.Calculation.TaxiMinutes)
	}
	if *original.Calculation.TaxiRunway != "04L" {
		t.Fatalf("expected original calculation taxi runway to remain 04L, got %s", *original.Calculation.TaxiRunway)
	}
}
