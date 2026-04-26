package config

import (
	"slices"
	"strings"

	"github.com/golang/geo/s2"
)

// SetRunwayRegionsForTest replaces the package-level runwayRegions slice for testing.
// Returns a cleanup function that restores the original value.
func SetRunwayRegionsForTest(r []Region) func() {
	old := runwayRegions
	runwayRegions = r
	return func() { runwayRegions = old }
}

// SetFinalApproachRegionsForTest replaces the package-level finalApproachRegions slice for testing.
// Returns a cleanup function that restores the original value.
func SetFinalApproachRegionsForTest(r []Region) func() {
	old := finalApproachRegions
	finalApproachRegions = r
	return func() { finalApproachRegions = old }
}

// MakeTestRunwayRegion builds a Region from a list of [lon, lat] coordinate pairs,
// matching the GeoJSON convention used in ekch_regions.json.
// runways lists the individual runway identifiers covered by this polygon (e.g. ["22L", "04R"]).
func MakeTestRunwayRegion(name string, runways []string, coords [][2]float64) Region {
	points := make([]s2.Point, len(coords))
	for i, c := range coords {
		points[i] = s2.PointFromLatLng(s2.LatLngFromDegrees(c[1], c[0]))
	}

	loop := s2.LoopFromPoints(points)
	loop.Normalize()
	region := Region{Name: name, Runways: runways, Region: loop}
	if strings.HasPrefix(name, "FINAL_") && len(coords) >= 2 {
		region.ThresholdLon = (coords[0][0] + coords[1][0]) / 2
		region.ThresholdLat = (coords[0][1] + coords[1][1]) / 2
		region.GlideslopeDegrees = defaultFinalApproachGlideslopeDegrees
		region.MaxAboveGlideslopeFt = defaultFinalApproachMaxAboveGlideslopeFt
	}

	return region
}

// SetPositionsForTest replaces the package-level positions slice for testing.
// Returns a cleanup function that restores the original value.
func SetPositionsForTest(ps []Position) func() {
	old := positions
	positions = ps
	return func() { positions = old }
}

// SetOwnerCallsignPrefixesForTest replaces the package-level ownerCallsignPrefixes slice for testing.
// Returns a cleanup function that restores the original value.
func SetOwnerCallsignPrefixesForTest(prefixes []string) func() {
	old := slices.Clone(ownerCallsignPrefixes)
	ownerCallsignPrefixes = normalizeOwnerCallsignPrefixes(prefixes)
	return func() { ownerCallsignPrefixes = old }
}

// SetSectorsForTest replaces the package-level sectors slice for testing.
// Returns a cleanup function that restores the original value.
func SetSectorsForTest(ss []Sector) func() {
	old := sectors
	sectors = ss
	return func() { sectors = old }
}

// SetAirborneOwnersForTest replaces the package-level airborneOwners slice for testing.
// Returns a cleanup function that restores the original value.
func SetAirborneOwnersForTest(owners []string) func() {
	old := airborneOwners
	airborneOwners = owners
	return func() { airborneOwners = old }
}

// SetAirborneFallbackLayoutForTest replaces the package-level airborneFallbackLayout for testing.
// Returns a cleanup function that restores the original value.
func SetAirborneFallbackLayoutForTest(layout string) func() {
	old := airborneFallbackLayout
	airborneFallbackLayout = layout
	return func() { airborneFallbackLayout = old }
}

// SetTaxiwayTypeValidationConfigForTest replaces taxiway-type validation rules for testing.
// Returns a cleanup function that restores the original value.
func SetTaxiwayTypeValidationConfigForTest(cfg TaxiwayTypeValidationConfig) func() {
	old := taxiwayTypeValidationConfig
	taxiwayTypeValidationConfig = normalizeTaxiwayTypeValidationConfig(cfg)
	return func() { taxiwayTypeValidationConfig = old }
}

// SetLayoutsForTest replaces the package-level layouts map for testing.
// Returns a cleanup function that restores the original value.
func SetLayoutsForTest(next map[string][]LayoutVariant) func() {
	old := layouts
	layouts = next
	return func() { layouts = old }
}

// SetMissedApproachHandoverForTest replaces the package-level missedApproachHandover map for testing.
// Returns a cleanup function that restores the original value.
func SetMissedApproachHandoverForTest(m map[string]string) func() {
	old := missedApproachHandover
	missedApproachHandover = m
	return func() { missedApproachHandover = old }
}
