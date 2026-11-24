package gsx

import (
	"net/http"
	"testing"
)

func targetFixture() Sceneries {
	return Sceneries{"EKCH": &SceneryConfig{
		ICAO:                     "EKCH",
		ReleasePointRadiusMeters: 40,
		ReleasePoints: map[string]PointPosition{
			"Y1": {Lat: 55.626254, Lon: 12.64125},
			"S1": {Lat: 55.62603, Lon: 12.65322},
		},
		Facings: map[string]map[string]string{"A15": {"Y1": "E"}},
	}}
}

func TestTargetCarriesThePointPosition(t *testing.T) {
	target := targetFixture().Target("EKCH", "D2", "S1")
	if target == nil || target.Lat == nil || target.Lon == nil {
		t.Fatalf("want a position, got %+v", target)
	}
	if *target.Lat != 55.62603 || *target.Lon != 12.65322 {
		t.Fatalf("position: got %v,%v", *target.Lat, *target.Lon)
	}
	if target.Heading != nil {
		t.Fatalf("no facing rule at D2, got heading %v", *target.Heading)
	}
	if target.RadiusMeters != 40 {
		t.Fatalf("radius: got %v", target.RadiusMeters)
	}
	if len(target.Avoid) != 1 || target.Avoid[0] != (PointPosition{Lat: 55.626254, Lon: 12.64125}) {
		t.Fatalf("want only the other release point in avoid, got %+v", target.Avoid)
	}
}

func TestTargetAppliesTheStandsFacingRule(t *testing.T) {
	target := targetFixture().Target("EKCH", "a15", "y1")
	if target == nil || target.Heading == nil || *target.Heading != 90 {
		t.Fatalf("A15 Y1 faces east, got %+v", target)
	}
}

func TestTargetForADirectionIsAHeadingOnly(t *testing.T) {
	// The pushback map's N/E/S/W buttons: how a bare taxiway is assigned.
	for point, want := range map[string]float64{"N": 0, "E": 90, "S": 180, "W": 270} {
		target := targetFixture().Target("EKCH", "F1", point)
		if target == nil || target.Heading == nil || *target.Heading != want {
			t.Fatalf("%s: want heading %v, got %+v", point, want, target)
		}
		if target.Lat != nil || target.Lon != nil {
			t.Fatalf("%s: a direction has no position", point)
		}
	}
	// The named points come along, so the handler can prefer a route that ends
	// on the bare taxiway over one that ends at a point.
	if target := targetFixture().Target("EKCH", "F1", "W"); len(target.Avoid) != 2 {
		t.Fatalf("want both named points to avoid, got %+v", target.Avoid)
	}
	// Even at an airport with no GSX config at all.
	if target := (Sceneries{}).Target("ESSA", "1", "W"); target == nil || *target.Heading != 270 {
		t.Fatalf("a direction needs no config, got %+v", target)
	}
}

func TestTargetForAnUnknownPointIsNil(t *testing.T) {
	if target := targetFixture().Target("EKCH", "D2", "K1"); target != nil {
		t.Fatalf("no position for K1, got %+v", target)
	}
	if target := targetFixture().Target("ESSA", "D2", "S1"); target != nil {
		t.Fatalf("another airport's points are not EKCH's, got %+v", target)
	}
	if target := targetFixture().Target("EKCH", "D2", ""); target != nil {
		t.Fatalf("no point, no target, got %+v", target)
	}
}

func TestDepartureCarriesItsTarget(t *testing.T) {
	api := NewWebAPI(liveEKCH(), departing("D2", "S1"), targetFixture(), true)
	for _, path := range []string{"/gsx/stand?callsign=SAS1401", "/gsx/stand?callsign=SAS1401&icao=EKCH"} {
		response := decode(t, get(t, api, path, nil))
		if response.Target == nil || response.Target.Lat == nil || *response.Target.Lat != 55.62603 {
			t.Fatalf("%s: want S1's position, got %+v", path, response.Target)
		}
	}
}

func TestTargetChangesInvalidateDepartureETag(t *testing.T) {
	changes := []struct {
		name   string
		point  string
		update func(*SceneryConfig)
	}{
		{"position", "Y1", func(cfg *SceneryConfig) { cfg.ReleasePoints["Y1"] = PointPosition{Lat: 55.627, Lon: 12.641} }},
		{"radius", "Y1", func(cfg *SceneryConfig) { cfg.ReleasePointRadiusMeters = 30 }},
		{"facing", "Y1", func(cfg *SceneryConfig) { cfg.Facings["A15"]["Y1"] = "W" }},
		{"nearby point", "Y1", func(cfg *SceneryConfig) { delete(cfg.ReleasePoints, "S1") }},
		{"direction avoidance", "W", func(cfg *SceneryConfig) { delete(cfg.ReleasePoints, "S1") }},
		{"withdrawn target", "Y1", func(cfg *SceneryConfig) { delete(cfg.ReleasePoints, "Y1") }},
	}
	const path = "/gsx/stand?callsign=SAS1401"
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			api := NewWebAPI(liveEKCH(), departing("A15", change.point), targetFixture(), true)
			original := get(t, api, path, nil)
			etag := original.Header().Get("ETag")
			previous := decode(t, original)
			headers := map[string]string{"If-None-Match": etag}
			if same := get(t, api, path, headers); same.Code != http.StatusNotModified {
				t.Fatalf("unchanged target: want 304, got %d", same.Code)
			}
			change.update(api.sceneries["EKCH"])
			updated := get(t, api, path, headers)
			if updated.Code != http.StatusOK || updated.Header().Get("ETag") == etag {
				t.Fatalf("changed target: want 200 and a new ETag, got %d and %q",
					updated.Code, updated.Header().Get("ETag"))
			}
			if response := decode(t, updated); response.Revision == previous.Revision {
				t.Fatalf("changed target kept the same revision %q", response.Revision)
			}
		})
	}
}

func TestShippedReleasePoints(t *testing.T) {
	cfg, err := LoadSceneryConfig("../../config/ekch/gsx_sceneries.json")
	if err != nil || cfg == nil {
		t.Fatalf("load shipped config: %v", err)
	}
	if len(cfg.ReleasePoints) < 40 {
		t.Fatalf("expected the shipped release points, got %d", len(cfg.ReleasePoints))
	}
	radius := cfg.ReleasePointRadiusMeters
	if radius <= 0 {
		t.Fatal("the shipped config must set release_point_radius_m")
	}
	sceneries := Sceneries{"EKCH": cfg}
	if target := sceneries.Target("EKCH", "B19", "Y3"); target != nil {
		t.Fatalf("Y3 needs its surveyed positions reconciled before publishing a target: %+v", target)
	}
	if _, labels := sceneries.Resolve("EKCH", "B19", "Simnord-Sonnich", "Y3"); len(labels) == 0 {
		t.Fatal("B19 Y3 should still carry the mapped labels")
	}
	for gate, rules := range cfg.Facings {
		for point, facing := range rules {
			if _, ok := compassHeadings[normalizeKey(facing)]; !ok {
				t.Errorf("%s %s: facing %q is not a compass direction", gate, point, facing)
			}
			if _, ok := lookup(cfg.ReleasePoints, point); !ok {
				t.Errorf("%s: facing rule for %s, which has no position", gate, point)
			}
		}
	}
}
