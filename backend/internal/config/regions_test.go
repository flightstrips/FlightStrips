package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetRunwayRegionForPosition_MatchesEkchRunway1230Midpoint(t *testing.T) {
	loadEkchRegions(t)

	region, ok := GetRunwayRegionForPosition(55.62002, 12.65029)
	if !ok {
		t.Fatal("expected RWY_1230 midpoint to match a runway region")
	}
	if region.Name != "RWY_1230" {
		t.Fatalf("expected RWY_1230, got %q", region.Name)
	}
}

func TestGetFinalApproachRegionForRunway_MatchesEkch22LFromMapGeometry(t *testing.T) {
	loadEkchRegions(t)

	region, ok := GetFinalApproachRegionForRunway("22L", 55.637980, 12.686934)
	if !ok {
		t.Fatal("expected FINAL_22L map funnel centroid to match a final approach region")
	}
	if region.Name != "FINAL_22L" {
		t.Fatalf("expected FINAL_22L, got %q", region.Name)
	}
}

func TestGetFinalApproachRegionForRunway_DoesNotMatchEkch04FarOutPoint(t *testing.T) {
	loadEkchRegions(t)

	tests := []struct {
		name   string
		runway string
	}{
		{name: "FINAL_04L", runway: "04L"},
		{name: "FINAL_04R", runway: "04R"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, ok := GetFinalApproachRegionForRunway(tt.runway, 54.945556, 12.313611)
			if ok {
				t.Fatalf("expected MONAK to be outside %s, got %q", tt.name, region.Name)
			}
		})
	}
}

func TestGetFinalApproachRegionForRunway_MatchesEkch04InsidePoint(t *testing.T) {
	loadEkchRegions(t)

	tests := []struct {
		name   string
		runway string
		lat    float64
		lon    float64
	}{
		{name: "FINAL_04L", runway: "04L", lat: 55.529073, lon: 12.506543},
		{name: "FINAL_04R", runway: "04R", lat: 55.539976, lon: 12.536029},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, ok := GetFinalApproachRegionForRunway(tt.runway, tt.lat, tt.lon)
			if !ok {
				t.Fatalf("expected inside point to match %s", tt.name)
			}
			if region.Name != tt.name {
				t.Fatalf("expected %s, got %q", tt.name, region.Name)
			}
		})
	}
}

func TestLoadRegions_RejectsInvalidPolygonAndNamesRegion(t *testing.T) {
	oldRegions := regions
	oldRunwayRegions := runwayRegions
	oldFinalApproachRegions := finalApproachRegions
	t.Cleanup(func() {
		regions = oldRegions
		runwayRegions = oldRunwayRegions
		finalApproachRegions = oldFinalApproachRegions
	})

	err := loadRegions(strings.NewReader(`{
		"type": "FeatureCollection",
		"features": [
			{
				"type": "Feature",
				"properties": {
					"name": "FINAL_BAD"
				},
				"geometry": {
					"type": "Polygon",
					"coordinates": [[
						[12.0, 55.0],
						[12.1, 55.0],
						[12.1, 55.0],
						[12.0, 55.1],
						[12.0, 55.0]
					]]
				}
			}
		]
	}`))
	if err == nil {
		t.Fatal("expected invalid polygon to be rejected")
	}
	if !strings.Contains(err.Error(), "FINAL_BAD") {
		t.Fatalf("expected error to name FINAL_BAD, got %q", err)
	}
}

func loadEkchRegions(t *testing.T) {
	t.Helper()

	oldRegions := regions
	oldRunwayRegions := runwayRegions
	oldFinalApproachRegions := finalApproachRegions
	t.Cleanup(func() {
		regions = oldRegions
		runwayRegions = oldRunwayRegions
		finalApproachRegions = oldFinalApproachRegions
	})

	f, err := os.Open(filepath.Join("..", "..", "config", "ekch_regions.json"))
	if err != nil {
		t.Fatalf("open ekch regions: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := loadRegions(f); err != nil {
		t.Fatalf("load ekch regions: %v", err)
	}
}
