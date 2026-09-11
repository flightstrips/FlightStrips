package aman

import (
	"slices"
	"testing"
)

func TestWarningIdentityIsStableAndIgnoresPresentation(t *testing.T) {
	component := "navigation"
	warning := Warning{
		Source: WarningSourceTechnicalHealth, Component: &component,
		Severity: WarningSeverityWarning, Code: "terminal_geometry_invalid", Message: "original wording",
	}
	localized := warning
	localized.Message = "localized wording"
	localized.Severity = WarningSeverityError

	if warning.Identity() != localized.Identity() {
		t.Fatalf("presentation changed identity: %q != %q", warning.Identity(), localized.Identity())
	}
	if warning.Identity() != `warning:"technical_health"/"navigation"/"terminal_geometry_invalid"/-/-/-` {
		t.Fatalf("identity = %q", warning.Identity())
	}
}

func TestWarningIdentityIncludesSourceAndOptionalIdentities(t *testing.T) {
	component := "navigation"
	group := RunwayGroupID("north")
	flight := FlightID("flight-1")
	related := FlightID("flight-2")
	warnings := []Warning{
		{Source: WarningSourceTechnicalHealth, Code: "same"},
		{Source: WarningSourceSequence, Code: "same"},
		{Source: WarningSourceTechnicalHealth, Component: &component, Code: "same"},
		{Source: WarningSourceTechnicalHealth, Code: "same", RunwayGroupID: &group},
		{Source: WarningSourceTechnicalHealth, Code: "same", FlightID: &flight},
		{Source: WarningSourceTechnicalHealth, Code: "same", RelatedFlightID: &related},
	}
	identities := make(map[string]struct{}, len(warnings))
	for _, warning := range warnings {
		identities[warning.Identity()] = struct{}{}
	}
	if len(identities) != len(warnings) {
		t.Fatalf("optional identities collapsed: %#v", identities)
	}
}

func TestCurrentWarningSnapshotCombinesDeduplicatesAndOrders(t *testing.T) {
	technical := TechnicalHealth{
		Enabled: true, EffectiveMode: EffectiveBlocked,
		VATSIM:     ComponentHealth{Status: HealthReady},
		Navigation: ComponentHealth{Status: HealthDegraded, Reason: "terminal_geometry_invalid"},
		Weather:    ComponentHealth{Status: HealthUnavailable, Reason: "weather_fetch_failed"},
		Repository: ComponentHealth{Status: HealthReady}, Predictor: ComponentHealth{Status: HealthReady},
		ReplayValidation: ComponentHealth{Status: HealthReady},
		BlockedReasons:   []string{"validation_evidence_missing", "navigation:terminal_geometry_invalid"},
	}
	conflict := RunwayGroupSequenceWarning{
		Code: "protected_same_star_spacing", FlightID: "trailing", RelatedFlightID: "leader", STARFamily: "TUDLO",
	}
	state := AirportState{RunwayGroups: []RunwayGroupPolicy{
		{ID: "south", SequenceWarnings: []RunwayGroupSequenceWarning{conflict, conflict}},
	}}

	snapshot := CurrentWarningSnapshot(technical, state)
	if len(snapshot.Warnings) != 5 {
		t.Fatalf("warnings = %#v", snapshot.Warnings)
	}
	if snapshot.Warnings[4].Severity != WarningSeverityWarning || snapshot.Warnings[4].Component == nil || *snapshot.Warnings[4].Component != "navigation" {
		t.Fatalf("degraded warning must follow errors: %#v", snapshot.Warnings)
	}
	for index := 1; index < 4; index++ {
		if snapshot.Warnings[index-1].Severity != WarningSeverityError || snapshot.Warnings[index-1].ID >= snapshot.Warnings[index].ID {
			t.Fatalf("errors are not ordered by identity: %#v", snapshot.Warnings)
		}
	}
	sources := []WarningSource{}
	for _, warning := range snapshot.Warnings {
		if !slices.Contains(sources, warning.Source) {
			sources = append(sources, warning.Source)
		}
	}
	if !slices.Contains(sources, WarningSourceTechnicalHealth) || !slices.Contains(sources, WarningSourceSequence) {
		t.Fatalf("sources = %#v", sources)
	}
}

func TestCurrentWarningSnapshotUsesCompleteReplacementForResolution(t *testing.T) {
	degraded := TechnicalHealth{
		Enabled: true, EffectiveMode: EffectiveBlocked,
		VATSIM:     ComponentHealth{Status: HealthDegraded, Reason: "snapshot_stale"},
		Navigation: ComponentHealth{Status: HealthReady}, Weather: ComponentHealth{Status: HealthReady},
		Repository: ComponentHealth{Status: HealthReady}, Predictor: ComponentHealth{Status: HealthReady},
		ReplayValidation: ComponentHealth{Status: HealthReady},
		BlockedReasons:   []string{"vatsim:snapshot_stale"},
	}
	state := AirportState{RunwayGroups: []RunwayGroupPolicy{{ID: "north", SequenceWarnings: []RunwayGroupSequenceWarning{{
		Code: "protected_same_star_spacing", FlightID: "flight-1", RelatedFlightID: "flight-2", STARFamily: "TESPI",
	}}}}}
	if got := CurrentWarningSnapshot(degraded, state); len(got.Warnings) != 3 {
		t.Fatalf("initial warnings = %#v", got.Warnings)
	}

	healthy := TechnicalHealth{
		Enabled: true, EffectiveMode: EffectiveAuthoritative,
		VATSIM: ComponentHealth{Status: HealthReady}, Navigation: ComponentHealth{Status: HealthReady},
		Weather: ComponentHealth{Status: HealthReady}, Repository: ComponentHealth{Status: HealthReady},
		Predictor: ComponentHealth{Status: HealthReady}, ReplayValidation: ComponentHealth{Status: HealthReady},
	}
	resolved := CurrentWarningSnapshot(healthy, AirportState{RunwayGroups: []RunwayGroupPolicy{{ID: "north"}}})
	if resolved.Warnings == nil || len(resolved.Warnings) != 0 {
		t.Fatalf("resolved replacement must be a non-nil empty snapshot: %#v", resolved)
	}
}

func TestCurrentWarningSnapshotIsIndependentOfInputOrder(t *testing.T) {
	first := AirportState{RunwayGroups: []RunwayGroupPolicy{
		{ID: "south", SequenceWarnings: []RunwayGroupSequenceWarning{{Code: "protected_same_star_spacing", FlightID: "b", RelatedFlightID: "c", STARFamily: "B"}}},
		{ID: "north", SequenceWarnings: []RunwayGroupSequenceWarning{{Code: "protected_same_star_spacing", FlightID: "a", RelatedFlightID: "b", STARFamily: "A"}}},
	}}
	second := AirportState{RunwayGroups: slices.Clone(first.RunwayGroups)}
	slices.Reverse(second.RunwayGroups)

	one := CurrentWarningSnapshot(TechnicalHealth{}, first)
	two := CurrentWarningSnapshot(TechnicalHealth{}, second)
	if !slices.EqualFunc(one.Warnings, two.Warnings, func(left, right Warning) bool { return left.ID == right.ID && left.Message == right.Message }) {
		t.Fatalf("input order changed snapshot:\n%#v\n%#v", one.Warnings, two.Warnings)
	}
}
