package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetRunwayRegionForPosition_MatchesEkchRunway1230Midpoint(t *testing.T) {
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

	region, ok := GetRunwayRegionForPosition(55.62002, 12.65029)
	if !ok {
		t.Fatal("expected RWY_1230 midpoint to match a runway region")
	}
	if region.Name != "RWY_1230" {
		t.Fatalf("expected RWY_1230, got %q", region.Name)
	}
}

func TestGetFinalApproachRegionForRunway_MatchesEkch22LFromMapGeometry(t *testing.T) {
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

	region, ok := GetFinalApproachRegionForRunway("22L", 55.637980, 12.686934)
	if !ok {
		t.Fatal("expected FINAL_22L map funnel centroid to match a final approach region")
	}
	if region.Name != "FINAL_22L" {
		t.Fatalf("expected FINAL_22L, got %q", region.Name)
	}
}
