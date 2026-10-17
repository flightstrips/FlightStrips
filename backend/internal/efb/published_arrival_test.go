package efb

import (
	"FlightStrips/internal/aman/terminal"
	"context"
	"path/filepath"
	"testing"
)

func TestConfiguredArrivalDataWithoutNavigationService(t *testing.T) {
	config, err := terminal.LoadFile(filepath.Join("..", "..", "config", "aman", "ekch-terminal-2610.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		star, runway, terminal string
		heading                int32
	}{
		{"TUDLO4A", "04L", "ERPUK", 217},
		{"TUDLO4A", "04R", "ERPUK", 217},
		{"TUDLO4C", "22L", "ABEGI", 37},
		{"TUDLO4C", "22R", "ABEGI", 37},
	} {
		t.Run(tc.runway, func(t *testing.T) {
			ctx := context.Background()
			fix := terminalFix(config, "EKCH", &tc.runway, &tc.star)
			heading, holding, detail := publishedArrivalData(ctx, nil, config, "EKCH", &tc.runway, &tc.star)
			if fix == nil || *fix != tc.terminal || heading == nil || *heading != tc.heading ||
				holding == nil || *holding != "LUGAS" || detail == nil || *detail != "077/LEFT" {
				t.Fatalf("unexpected configured arrival: terminal=%v heading=%v holding=%v detail=%v", fix, heading, holding, detail)
			}
		})
	}
	star, runway := "UNKNOWN1A", "04L"
	heading, holding, detail := publishedArrivalData(context.Background(), nil, config, "EKCH", &runway, &star)
	if heading != nil || holding != nil || detail != nil || terminalFix(config, "EKCH", &runway, &star) != nil {
		t.Fatal("unknown STAR must not acquire another procedure's published data")
	}
}
