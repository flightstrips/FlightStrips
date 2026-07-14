package cdm

import (
	"FlightStrips/internal/models"
	"testing"
)

func TestApplyConfirmedTobtUpdateMarksPilotSource(t *testing.T) {
	data := &models.CdmData{}
	applyConfirmedTobtUpdate(data, "1234", "PILOT:1234567", "pilot")
	if data.TobtConfirmedBy == nil || *data.TobtConfirmedBy != models.TobtConfirmedByPilot {
		t.Fatalf("expected pilot confirmation source, got %v", data.TobtConfirmedBy)
	}
	if data.TobtSetBy == nil || *data.TobtSetBy != "PILOT:1234567" {
		t.Fatalf("expected pilot actor metadata, got %v", data.TobtSetBy)
	}
}
