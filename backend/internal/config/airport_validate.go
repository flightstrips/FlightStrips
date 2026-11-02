package config

import (
	"fmt"
	"slices"
)

func canonicalAirportRunway(value string) string {
	value = normalizeRunway(value)
	if len(value) == 1 && value[0] >= '1' && value[0] <= '9' {
		return "0" + value
	}
	return value
}

func normalizeAirportRunwaySelectors(doc *AirportDocument) {
	normalize := func(values []string) {
		for i := range values {
			values[i] = canonicalAirportRunway(values[i])
		}
	}
	for i := range doc.Coverage.Responsibilities {
		normalize(doc.Coverage.Responsibilities[i].Active)
	}
	for i := range doc.Routing.Ground {
		route := &doc.Routing.Ground[i]
		route.ForRunway = canonicalAirportRunway(route.ForRunway)
		normalize(route.Active)
	}
	for _, variants := range doc.View.Layouts {
		for i := range variants {
			normalize(variants[i].Active)
		}
	}
	if doc.Services.PDC != nil && doc.Services.PDC.Validation != nil {
		normalize(doc.Services.PDC.Validation.HeavyRunwayRestriction.AllowedRunways)
	}
	if doc.Validation.Clearance != nil {
		for i := range doc.Validation.Clearance.AircraftRunwayRules {
			normalize(doc.Validation.Clearance.AircraftRunwayRules[i].RestrictedRunways)
		}
	}
}

func validateAirportDomains(doc *AirportDocument) error {
	positions, runways, roles := map[string]bool{}, map[string]bool{}, map[string]bool{}
	unique := func(set map[string]bool, id, field string) error {
		if set[id] {
			return fmt.Errorf("%s: duplicate %q", field, id)
		}
		set[id] = true
		return nil
	}
	reference := func(set map[string]bool, id, field string) error {
		if !set[id] {
			return fmt.Errorf("%s: unknown reference %q", field, id)
		}
		return nil
	}
	for _, p := range doc.Positions {
		if err := unique(positions, p.Name, "positions"); err != nil {
			return err
		}
	}
	for _, r := range doc.Geography.Runways {
		runways[r] = true
	}
	position := func(id, field string) error { return reference(positions, id, field) }
	runway := func(id, field string) error { return reference(runways, normalizeRunway(id), field) }
	for r := range doc.Geography.InitialCFLFT {
		if err := runway(r, "geography.initial_cfl_ft"); err != nil {
			return err
		}
	}
	dutyIDs := map[string]bool{}
	for _, duty := range doc.Coverage.Responsibilities {
		if err := unique(dutyIDs, duty.ID, "coverage.responsibilities.id"); err != nil {
			return err
		}
		roles[duty.Name] = true
		if duty.Key != "" {
			roles[duty.Key] = true
		}
		for _, p := range duty.Providers {
			if err := position(p, "coverage.responsibilities.providers"); err != nil {
				return err
			}
		}
		for _, r := range duty.Active {
			if err := runway(r, "coverage.responsibilities.active"); err != nil {
				return err
			}
		}
		if len(duty.Region) > 0 && doc.Geography.Regions == nil {
			return fmt.Errorf("coverage.responsibilities.region: geography.regions is required")
		}
	}
	for _, p := range doc.Coverage.AirborneFallback {
		if err := position(p, "coverage.airborne_fallback"); err != nil {
			return err
		}
	}
	priorities := map[string]bool{}
	for _, p := range doc.Master.Priorities {
		if err := position(p.Position, "master.priorities"); err != nil {
			return err
		}
		if err := unique(priorities, p.Position, "master.priorities"); err != nil {
			return err
		}
	}
	routeIDs := map[string]bool{}
	for _, route := range doc.Routing.Ground {
		if err := unique(routeIDs, route.ID, "routing.ground.id"); err != nil {
			return err
		}
		if route.ForRunway != "" {
			if err := runway(route.ForRunway, "routing.ground.forRunway"); err != nil {
				return err
			}
		}
		for _, r := range route.Active {
			if err := runway(r, "routing.ground.active"); err != nil {
				return err
			}
		}
		for _, role := range route.Path {
			if err := reference(roles, role, "routing.ground.path"); err != nil {
				return err
			}
		}
		for from, to := range route.OwnerOverrides {
			if err := reference(roles, from, "routing.ground.owner_overrides"); err != nil {
				return err
			}
			if err := reference(roles, to, "routing.ground.owner_overrides"); err != nil {
				return err
			}
		}
	}
	airborne := map[string]bool{}
	defaults := 0
	for _, route := range doc.Routing.Airborne {
		if err := reference(roles, route.Name, "routing.airborne.name"); err != nil {
			return err
		}
		if err := unique(airborne, route.Name, "routing.airborne.name"); err != nil {
			return err
		}
		if route.UseAsDefault {
			defaults++
		}
	}
	if len(airborne) > 0 && defaults != 1 {
		return fmt.Errorf("routing.airborne: require exactly one default")
	}
	for r, p := range doc.Routing.MissedApproach {
		if err := runway(r, "routing.missed_approach"); err != nil {
			return err
		}
		if err := position(p, "routing.missed_approach"); err != nil {
			return err
		}
	}
	matchers := []string{"_GND", "_TWR", "_APP", "_DEL", "_DEP", "_CTR"}
	for p, variants := range doc.View.Layouts {
		if err := position(p, "view.layouts"); err != nil {
			return err
		}
		for _, v := range variants {
			for _, r := range v.Active {
				if err := runway(r, "view.layouts.active"); err != nil {
					return err
				}
			}
			for _, p := range append(slices.Clone(v.Online), v.Offline...) {
				if !slices.Contains(matchers, p) {
					if err := position(p, "view.layouts.matcher"); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, providers := range doc.View.MessageAreas {
		for _, p := range providers {
			if err := position(p, "view.message_areas"); err != nil {
				return err
			}
		}
	}
	bays := map[string]bool{}
	for _, bay := range doc.Workflow.Bays {
		if err := unique(bays, bay.ID, "workflow.bays.id"); err != nil {
			return err
		}
	}
	_, _, err := buildRouteIndex(doc.DomainConfig())
	return err
}
