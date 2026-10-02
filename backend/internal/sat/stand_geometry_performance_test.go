package sat

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func bruteStandAtPosition(registry *StandCapabilityRegistry, airport string, lat, lon float64) (Stand, bool) {
	nearest := math.MaxFloat64
	var match Stand
	found := false
	for _, stand := range registry.byAirport[airport] {
		if stand.Radius <= 0 {
			continue
		}
		distance := greatCircleMetres(lat, lon, stand.Latitude, stand.Longitude)
		if distance <= stand.Radius && distance < nearest {
			match, nearest, found = stand, distance, true
		}
	}
	return match, found
}

func TestStandGeometryPrefilterPreservesExactSphericalDecisions(t *testing.T) {
	registry := &StandCapabilityRegistry{byAirport: map[string]map[string]Stand{"TEST": {}}}
	random := rand.New(rand.NewSource(23))
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("S%03d", i)
		registry.byAirport["TEST"][name] = Stand{Name: name, Latitude: 55.62 + random.Float64()*.03, Longitude: 12.62 + random.Float64()*.04, Radius: 30}
	}
	registry.byAirport["TEST"]["NORTH"] = Stand{Name: "NORTH", Latitude: 89.999, Longitude: 179.9, Radius: 1500}
	registry.byAirport["TEST"]["SOUTH"] = Stand{Name: "SOUTH", Latitude: -89.999, Longitude: -179.9, Radius: 1500}
	registry.byAirport["TEST"]["DATE"] = Stand{Name: "DATE", Latitude: 0, Longitude: 179.999, Radius: 500}
	verify := func(lat, lon, maxDistance float64) {
		t.Helper()
		want, expected := bruteStandAtPosition(registry, "TEST", lat, lon)
		got, found := registry.StandAtPosition("TEST", lat, lon)
		if expected != found || found && greatCircleMetres(lat, lon, got.Latitude, got.Longitude) != greatCircleMetres(lat, lon, want.Latitude, want.Longitude) {
			t.Fatalf("nearest stand differs at (%g,%g): got %v/%v want %v/%v", lat, lon, got.Name, found, want.Name, expected)
		}
		near := false
		for _, stand := range registry.byAirport["TEST"] {
			near = near || greatCircleMetres(lat, lon, stand.Latitude, stand.Longitude) <= maxDistance
		}
		if registry.PositionNearAirport("TEST", lat, lon, maxDistance) != near {
			t.Fatalf("airport proximity differs at (%g,%g), radius %g", lat, lon, maxDistance)
		}
	}
	for i := 0; i < 1000; i++ {
		verify(55.6+random.Float64()*.1, 12.6+random.Float64()*.1, random.Float64()*2000)
	}
	for _, stand := range registry.byAirport["TEST"] {
		if math.Abs(stand.Latitude) > 89 {
			continue
		}
		for _, multiplier := range []float64{1 - 1e-10, 1, 1 + 1e-10} {
			delta := stand.Radius / (earthRadiusMetres * math.Pi / 180) * multiplier
			verify(stand.Latitude+delta, stand.Longitude, stand.Radius)
			verify(stand.Latitude-delta, stand.Longitude, stand.Radius)
		}
	}
	for _, position := range [][2]float64{{89.999, -179.9}, {90, 0}, {-89.999, 179.9}, {-90, 0}, {0, -179.999}, {0, 179.999}, {55.63, 12.63}, {-90, -180}, {90, 180}} {
		verify(position[0], position[1], 1000)
	}
	// Overlapping centres retain the closest exact distance, including equal ties.
	registry.byAirport["TEST"]["TIE1"] = Stand{Name: "TIE1", Latitude: 55, Longitude: 12, Radius: 100}
	registry.byAirport["TEST"]["TIE2"] = Stand{Name: "TIE2", Latitude: 55, Longitude: 12, Radius: 100}
	verify(55, 12, 100)
}

func BenchmarkStandGeometryFarFleet(b *testing.B) {
	registry := &StandCapabilityRegistry{byAirport: map[string]map[string]Stand{"EKCH": {}}}
	for i := 0; i < 120; i++ {
		name := fmt.Sprintf("S%03d", i)
		registry.byAirport["EKCH"][name] = Stand{Name: name, Latitude: 55.625 + float64(i)*.0002, Longitude: 12.645, Radius: 30}
	}
	for _, optimized := range []bool{false, true} {
		name := "brute"
		if optimized {
			name = "latitude-bound"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				lat := 55.63 + float64(i%200)*.0005
				if optimized {
					registry.StandAtPosition("EKCH", lat, 12.65)
				} else {
					bruteStandAtPosition(registry, "EKCH", lat, 12.65)
				}
			}
		})
	}
}
