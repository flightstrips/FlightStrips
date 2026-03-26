package cdm

import (
	"testing"
)

func makeDeiceConfig() CdmDeiceConfig {
	return CdmDeiceConfig{
		Light:  3,
		Medium: 5,
		Heavy:  7,
		Super:  9,
		Platform: []CdmDeicePlatformConfig{
			{Name: "A", Time: 5},
			{Name: "B", Time: 6},
			{Name: "V", Time: 10},
		},
	}
}

func makeAirportConfigWithDeice() *CdmAirportConfig {
	cfg := NewDefaultAirportConfig("EKCH")
	cfg.DeiceConfig = makeDeiceConfig()
	return cfg
}

// ---- DeiceMinutesForWtc ----

func TestDeiceMinutesForWtc_Light(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("L"); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestDeiceMinutesForWtc_LightLowercase(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("l"); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
}

func TestDeiceMinutesForWtc_Medium(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("M"); got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestDeiceMinutesForWtc_Heavy(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("H"); got != 7 {
		t.Errorf("got %d, want 7", got)
	}
}

func TestDeiceMinutesForWtc_Super(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("J"); got != 9 {
		t.Errorf("got %d, want 9", got)
	}
}

func TestDeiceMinutesForWtc_Unknown_ReturnsZero(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	if got := cfg.DeiceMinutesForWtc("X"); got != 0 {
		t.Errorf("got %d, want 0 for unknown WTC", got)
	}
}

func TestDeiceMinutesForWtc_NilReceiver_ReturnsZero(t *testing.T) {
	var cfg *CdmAirportConfig
	if got := cfg.DeiceMinutesForWtc("M"); got != 0 {
		t.Errorf("got %d, want 0 for nil receiver", got)
	}
}

// ---- DeiceMinutesForPlatform ----

func TestDeiceMinutesForPlatform_Found(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	got, ok := cfg.DeiceMinutesForPlatform("A")
	if !ok {
		t.Fatal("expected platform A to be found")
	}
	if got != 5 {
		t.Errorf("got %d, want 5", got)
	}
}

func TestDeiceMinutesForPlatform_CaseInsensitive(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	got, ok := cfg.DeiceMinutesForPlatform("v")
	if !ok {
		t.Fatal("expected platform V to be found case-insensitively")
	}
	if got != 10 {
		t.Errorf("got %d, want 10", got)
	}
}

func TestDeiceMinutesForPlatform_NotFound(t *testing.T) {
	cfg := makeAirportConfigWithDeice()
	got, ok := cfg.DeiceMinutesForPlatform("Z")
	if ok {
		t.Error("expected platform Z not to be found")
	}
	if got != 0 {
		t.Errorf("got %d, want 0 for unknown platform", got)
	}
}

func TestDeiceMinutesForPlatform_NilReceiver_ReturnsZeroFalse(t *testing.T) {
	var cfg *CdmAirportConfig
	got, ok := cfg.DeiceMinutesForPlatform("A")
	if ok || got != 0 {
		t.Errorf("expected (0, false) for nil receiver, got (%d, %v)", got, ok)
	}
}

// ---- Clone deep-copies DeiceConfig ----

func TestClone_DeepCopiesDeiceConfig(t *testing.T) {
	original := makeAirportConfigWithDeice()
	clone := original.Clone()

	// Mutate original's platform slice — clone must be unaffected.
	original.DeiceConfig.Platform[0].Time = 999
	original.DeiceConfig.Platform = append(original.DeiceConfig.Platform, CdmDeicePlatformConfig{Name: "X", Time: 1})

	if clone.DeiceConfig.Platform[0].Time == 999 {
		t.Error("clone was affected by mutation of original.Platform[0].Time")
	}
	if len(clone.DeiceConfig.Platform) != 3 {
		t.Errorf("clone platform count = %d, want 3", len(clone.DeiceConfig.Platform))
	}
}

func TestClone_DeiceScalarFieldsCopied(t *testing.T) {
	original := makeAirportConfigWithDeice()
	clone := original.Clone()

	if clone.DeiceConfig.Light != 3 {
		t.Errorf("DeiceConfig.Light = %d, want 3", clone.DeiceConfig.Light)
	}
	if clone.DeiceConfig.Super != 9 {
		t.Errorf("DeiceConfig.Super = %d, want 9", clone.DeiceConfig.Super)
	}
}
