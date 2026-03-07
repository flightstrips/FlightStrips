package services

import (
	"strings"
	"testing"
)

func TestParseRegistration_FromRemarks(t *testing.T) {
	remarks := "PBN/A1B1C1D1O1S1 DOF/260301 REG/N320SB EET/EKDK0027 EDVV0042 OPR/SAS PER/C RMK/TCAS SIMBRIEF /V/"
	got := ParseRegistration("SAS123", remarks)
	if got != "N320SB" {
		t.Errorf("expected N320SB, got %s", got)
	}
}

func TestParseRegistration_FromCallsign(t *testing.T) {
	got := ParseRegistration("OYFSR", "")
	if got != "OYFSR" {
		t.Errorf("expected OYFSR, got %s", got)
	}
}

func TestParseRegistration_CallsignWithDigits(t *testing.T) {
	// Callsign with digits → not a registration, should fall through to random
	got := ParseRegistration("SAS123", "")
	// Just check it is non-empty and not the input callsign
	if got == "" || got == "SAS123" {
		t.Errorf("unexpected registration %q for callsign SAS123", got)
	}
}

func TestParseRegistration_RemarksOverCallsign(t *testing.T) {
	// Even if callsign looks like a reg, remarks take priority
	remarks := "REG/ABCDE"
	got := ParseRegistration("XYZAB", remarks)
	if got != "ABCDE" {
		t.Errorf("expected ABCDE (from remarks), got %s", got)
	}
}

func TestParseRegistration_CaseInsensitiveRemarks(t *testing.T) {
	remarks := "reg/g-abcd"
	got := ParseRegistration("SAS1", remarks)
	if !strings.EqualFold(got, "G-ABCD") {
		t.Errorf("expected G-ABCD, got %s", got)
	}
}
