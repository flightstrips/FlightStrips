// Package terminal owns versioned, airport-specific AMAN terminal geometry.
// It is intentionally a cache consumer: acquiring navigation data belongs to
// navdata sources and activation orchestration belongs to the materializer.
package terminal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
)

const SchemaVersion = "aman-terminal/v1"

// ReferenceReader is the small cache-only boundary required to validate an
// airport configuration. It intentionally exposes no acquisition operations.
type ReferenceReader interface {
	ActiveTerminalReferences(context.Context, navdata.AirportID) (ReferenceSet, error)
}

// ReferenceSet is one active manifest's terminal-validation view.
type ReferenceSet struct {
	Version    navdata.DatasetVersion
	Airport    navdata.Airport
	Runways    []navdata.Runway
	Fixes      []navdata.Fix
	Procedures []navdata.Procedure
}

type Source struct {
	ID             string    `json:"id"`
	Document       string    `json:"document"`
	EffectiveFrom  time.Time `json:"effectiveFrom"`
	EffectiveUntil time.Time `json:"effectiveUntil"`
}

type DatasetCompatibility struct {
	Cycle          string    `json:"cycle"`
	EffectiveFrom  time.Time `json:"effectiveFrom"`
	EffectiveUntil time.Time `json:"effectiveUntil"`
}

// CoordinateDefinition is the terminal configuration's explicit wire shape.
// It intentionally does not reuse navdata.Coordinate so the checked-in schema
// remains stable when canonical domain types evolve.
type CoordinateDefinition struct {
	LatitudeDeg  float64 `json:"latitudeDeg"`
	LongitudeDeg float64 `json:"longitudeDeg"`
}

type ThresholdDefinition struct {
	Position      CoordinateDefinition `json:"position"`
	ElevationFt   *int                 `json:"elevationFt,omitempty"`
	CourseTrueDeg *float64             `json:"courseTrueDeg,omitempty"`
}

type ProvenanceDefinition struct {
	SourceID       string    `json:"sourceId"`
	SourceRevision string    `json:"sourceRevision"`
	ImportedAt     time.Time `json:"importedAt"`
	EffectiveFrom  time.Time `json:"effectiveFrom"`
	EffectiveUntil time.Time `json:"effectiveUntil"`
}

type FinalApproachDefinition struct {
	Runway        navdata.RunwayID    `json:"runway"`
	Threshold     ThresholdDefinition `json:"threshold"`
	CourseTrueDeg float64             `json:"courseTrueDeg"`
	// PhysicalLengthM is the published physical runway length in metres from
	// the official aerodrome chart. It is converted once at this boundary to
	// the canonical nautical-mile representation.
	PhysicalLengthM float64              `json:"physicalLengthM"`
	Provenance      ProvenanceDefinition `json:"provenance"`
}

// HoldingDefinition is the terminal-owned wire contract for an official AIP
// fallback holding. It is converted to navdata only after configuration
// validation reaches the canonical-domain boundary.
type HoldingDefinition struct {
	ID                   navdata.HoldingID          `json:"id"`
	Fix                  navdata.FixID              `json:"fix"`
	InboundCourseTrueDeg float64                    `json:"inboundCourseTrueDeg"`
	TurnDirection        navdata.TurnDirection      `json:"turnDirection"`
	LegLengthNM          *float64                   `json:"legLengthNm,omitempty"`
	LegTimeSeconds       *int64                     `json:"legTimeSeconds,omitempty"`
	MinimumAltitudeFt    *int                       `json:"minimumAltitudeFt,omitempty"`
	MaximumAltitudeFt    *int                       `json:"maximumAltitudeFt,omitempty"`
	MaximumSpeedKt       *int                       `json:"maximumSpeedKt,omitempty"`
	Termination          navdata.HoldingTermination `json:"termination"`
	Provenance           ProvenanceDefinition       `json:"provenance"`
}

type RunwayGroup struct {
	ID              aman.RunwayGroupID        `json:"id"`
	Aliases         []aman.RunwayGroupID      `json:"aliases"`
	Runways         []navdata.RunwayID        `json:"runways"`
	FinalApproaches []FinalApproachDefinition `json:"finalApproaches"`
}

type Feeder struct {
	ID      navdata.FeederID   `json:"id"`
	Aliases []navdata.FeederID `json:"aliases"`
}

// FixAlias preserves an officially superseded identifier while keeping
// terminal paths and holding selection on the current canonical identifier.
type FixAlias struct {
	Alias     navdata.FixID `json:"alias"`
	Canonical navdata.FixID `json:"canonical"`
	Source    Source        `json:"source"`
}

// Path is an official, ordered STAR terminal fragment. SelectedHolding is an
// operational configuration selection, not an instruction to fly a circuit.
type Path struct {
	Feeder          navdata.FeederID   `json:"feeder"`
	RunwayGroup     aman.RunwayGroupID `json:"runwayGroup"`
	Fixes           []navdata.FixID    `json:"fixes"`
	MergeFix        navdata.FixID      `json:"mergeFix"`
	SelectedHolding navdata.HoldingID  `json:"selectedHolding"`
}

type Configuration struct {
	SchemaVersion      string               `json:"schemaVersion"`
	ConfigVersion      string               `json:"configVersion"`
	Airport            navdata.AirportID    `json:"airport"`
	ApplicabilityFrom  time.Time            `json:"applicabilityFrom"`
	ApplicabilityUntil time.Time            `json:"applicabilityUntil"`
	Dataset            DatasetCompatibility `json:"dataset"`
	Sources            []Source             `json:"sources"`
	RunwayGroups       []RunwayGroup        `json:"runwayGroups"`
	Feeders            []Feeder             `json:"feeders"`
	FixAliases         []FixAlias           `json:"fixAliases"`
	Paths              []Path               `json:"paths"`
	OverlayHoldings    []HoldingDefinition  `json:"overlayHoldings"`
}

func (c CoordinateDefinition) canonical() navdata.Coordinate {
	return navdata.Coordinate{LatitudeDeg: c.LatitudeDeg, LongitudeDeg: c.LongitudeDeg}
}

func (t ThresholdDefinition) canonical() navdata.Threshold {
	return navdata.Threshold{Position: t.Position.canonical(), ElevationFt: t.ElevationFt, CourseTrueDeg: t.CourseTrueDeg}
}

func (p ProvenanceDefinition) canonical() navdata.Provenance {
	return navdata.Provenance{SourceID: p.SourceID, SourceRevision: p.SourceRevision, ImportedAt: p.ImportedAt, EffectiveFrom: p.EffectiveFrom, EffectiveUntil: p.EffectiveUntil}
}

func (f FinalApproachDefinition) canonical() navdata.FinalApproach {
	return navdata.FinalApproach{Runway: f.Runway, Threshold: f.Threshold.canonical(), CourseTrueDeg: f.CourseTrueDeg, Provenance: f.Provenance.canonical()}
}

// Runways is a non-HTTP, official-terminal-configuration-backed RunwaySource.
// It deliberately complements, rather than extends, AirportSource: airport
// reference metadata and authoritative thresholds need not share a provider.
func (c Configuration) Runways(_ context.Context, version navdata.DatasetVersion, airport navdata.AirportID) ([]navdata.Runway, error) {
	if airport != c.Airport || version.Cycle != c.Dataset.Cycle || !version.EffectiveFrom.Equal(c.Dataset.EffectiveFrom) || !version.EffectiveUntil.Equal(c.Dataset.EffectiveUntil) {
		return nil, fmt.Errorf("terminal runway source does not cover requested airport dataset")
	}
	seen := map[navdata.RunwayID]struct{}{}
	result := make([]navdata.Runway, 0)
	for _, group := range c.RunwayGroups {
		for _, definition := range group.FinalApproaches {
			if definition.PhysicalLengthM <= 0 {
				return nil, fmt.Errorf("runway %s physicalLengthM must be positive", definition.Runway)
			}
			if _, exists := seen[definition.Runway]; exists {
				return nil, fmt.Errorf("runway %s is duplicated", definition.Runway)
			}
			seen[definition.Runway] = struct{}{}
			runway := navdata.Runway{ID: definition.Runway, Airport: airport, Threshold: definition.Threshold.canonical(), LengthNM: definition.PhysicalLengthM / 1852.0, Provenance: definition.Provenance.canonical()}
			if err := runway.Validate(); err != nil {
				return nil, fmt.Errorf("validate runway %s: %w", definition.Runway, err)
			}
			result = append(result, runway)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("terminal runway source has no runways")
	}
	return result, nil
}

func (h HoldingDefinition) canonical() navdata.HoldingPattern {
	return navdata.HoldingPattern{ID: h.ID, Fix: h.Fix, InboundCourseTrueDeg: h.InboundCourseTrueDeg, TurnDirection: h.TurnDirection, LegLengthNM: h.LegLengthNM, LegTimeSeconds: h.LegTimeSeconds, MinimumAltitudeFt: h.MinimumAltitudeFt, MaximumAltitudeFt: h.MaximumAltitudeFt, MaximumSpeedKt: h.MaximumSpeedKt, Termination: h.Termination, Provenance: h.Provenance.canonical()}
}

func LoadFile(path string) (Configuration, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return Configuration{}, fmt.Errorf("read terminal configuration: %w", err)
	}
	var value Configuration
	if err := json.Unmarshal(encoded, &value); err != nil {
		return Configuration{}, fmt.Errorf("decode terminal configuration: %w", err)
	}
	return value, nil
}

// ValidationErrors holds every configuration problem, preserving JSON-style
// field paths so an operator can correct a candidate in one edit cycle.
type ValidationErrors []error

func (e ValidationErrors) Error() string {
	parts := make([]string, len(e))
	for i := range e {
		parts[i] = e[i].Error()
	}
	return strings.Join(parts, "; ")
}
func (e ValidationErrors) Unwrap() []error { return []error(e) }
func add(errs *ValidationErrors, path, message string) {
	*errs = append(*errs, fmt.Errorf("%s: %s", path, message))
}

// Validate checks a complete candidate against exactly one active cache
// manifest. It never calls a source or derives a runway group from surveillance.
func (c Configuration) Validate(refs ReferenceSet) error {
	var errs ValidationErrors
	if c.SchemaVersion != SchemaVersion {
		add(&errs, "schemaVersion", "must equal "+SchemaVersion)
	}
	if strings.TrimSpace(c.ConfigVersion) == "" {
		add(&errs, "configVersion", "is required")
	}
	if c.Airport == "" {
		add(&errs, "airport", "is required")
	}
	if c.ApplicabilityFrom.IsZero() || c.ApplicabilityFrom.Location() != time.UTC {
		add(&errs, "applicabilityFrom", "must be UTC")
	}
	if c.ApplicabilityUntil.IsZero() || c.ApplicabilityUntil.Location() != time.UTC || !c.ApplicabilityUntil.After(c.ApplicabilityFrom) {
		add(&errs, "applicabilityUntil", "must be UTC and after applicabilityFrom")
	}
	if c.Dataset.Cycle == "" || c.Dataset.EffectiveFrom.IsZero() || c.Dataset.EffectiveUntil.IsZero() || !c.Dataset.EffectiveUntil.After(c.Dataset.EffectiveFrom) {
		add(&errs, "dataset", "requires cycle and a valid effective interval")
	}
	if refs.Airport.ID != c.Airport {
		add(&errs, "airport", "does not match active terminal references")
	}
	if refs.Version.Cycle != c.Dataset.Cycle || !refs.Version.EffectiveFrom.Equal(c.Dataset.EffectiveFrom) || !refs.Version.EffectiveUntil.Equal(c.Dataset.EffectiveUntil) {
		add(&errs, "dataset", "does not match active dataset")
	}
	if !c.ApplicabilityFrom.Equal(c.Dataset.EffectiveFrom) || !c.ApplicabilityUntil.Equal(c.Dataset.EffectiveUntil) {
		add(&errs, "applicability", "must match dataset effective interval")
	}
	if len(c.Sources) == 0 {
		add(&errs, "sources", "requires authoritative provenance")
	}
	for i, source := range c.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Document) == "" || source.EffectiveFrom.IsZero() || source.EffectiveUntil.IsZero() || !source.EffectiveUntil.After(source.EffectiveFrom) {
			add(&errs, fmt.Sprintf("sources[%d]", i), "requires identifier, document and effective interval")
		}
	}

	runways := map[navdata.RunwayID]navdata.Runway{}
	for _, r := range refs.Runways {
		runways[r.ID] = r
	}
	fixes := map[navdata.FixID]bool{}
	for _, f := range refs.Fixes {
		fixes[f.ID] = true
	}
	aliases := map[navdata.FixID]navdata.FixID{}
	aliasNames := map[navdata.FixID]string{}
	for i, alias := range c.FixAliases {
		if alias.Alias == "" || alias.Canonical == "" || alias.Alias == alias.Canonical {
			add(&errs, fmt.Sprintf("fixAliases[%d]", i), "requires distinct alias and canonical identifiers")
			continue
		}
		if _, found := aliases[alias.Alias]; found {
			add(&errs, fmt.Sprintf("fixAliases[%d].alias", i), "is duplicated")
		}
		if previous, found := aliasNames[alias.Alias]; found {
			add(&errs, fmt.Sprintf("fixAliases[%d].alias", i), "collides with "+previous)
		}
		aliasNames[alias.Alias] = fmt.Sprintf("fixAliases[%d].alias", i)
		if previous, found := aliasNames[alias.Canonical]; found {
			add(&errs, fmt.Sprintf("fixAliases[%d].canonical", i), "collides with "+previous)
		}
		aliasNames[alias.Canonical] = fmt.Sprintf("fixAliases[%d].canonical", i)
		aliases[alias.Alias] = alias.Canonical
		if !fixes[alias.Canonical] {
			add(&errs, fmt.Sprintf("fixAliases[%d].canonical", i), "is absent from active dataset")
		}
		if strings.TrimSpace(alias.Source.ID) == "" || strings.TrimSpace(alias.Source.Document) == "" || alias.Source.EffectiveFrom.IsZero() || alias.Source.EffectiveUntil.IsZero() || !alias.Source.EffectiveUntil.After(alias.Source.EffectiveFrom) {
			add(&errs, fmt.Sprintf("fixAliases[%d].source", i), "requires authoritative provenance")
		}
	}
	holdings := map[navdata.HoldingID]navdata.HoldingPattern{}
	for _, p := range refs.Procedures {
		for _, h := range p.Holdings {
			if old, ok := holdings[h.ID]; ok {
				oldDigest, _ := navdata.HoldingDigest(old)
				newDigest, _ := navdata.HoldingDigest(h)
				if oldDigest != newDigest {
					add(&errs, "references.procedures", "contains conflicting holding "+string(h.ID))
				}
			} else {
				holdings[h.ID] = h
			}
		}
	}
	overlayIDs := map[navdata.HoldingID]bool{}
	for i, definition := range c.OverlayHoldings {
		h := definition.canonical()
		if overlayIDs[h.ID] {
			add(&errs, fmt.Sprintf("overlayHoldings[%d].id", i), "is duplicated")
		}
		overlayIDs[h.ID] = true
		if err := h.Validate(); err != nil {
			add(&errs, fmt.Sprintf("overlayHoldings[%d]", i), err.Error())
			continue
		}
		if old, ok := holdings[h.ID]; ok {
			oldDigest, _ := navdata.HoldingDigest(old)
			newDigest, _ := navdata.HoldingDigest(h)
			if oldDigest != newDigest {
				add(&errs, fmt.Sprintf("overlayHoldings[%d]", i), "conflicts with canonical holding "+string(h.ID))
			}
		} else {
			holdings[h.ID] = h
		}
		if !fixes[h.Fix] {
			add(&errs, fmt.Sprintf("overlayHoldings[%d].fix", i), "is absent from active dataset")
		}
	}

	groups := map[aman.RunwayGroupID]RunwayGroup{}
	groupNames := map[aman.RunwayGroupID]string{}
	for i, group := range c.RunwayGroups {
		if group.ID == "" {
			add(&errs, fmt.Sprintf("runwayGroups[%d].id", i), "is required")
			continue
		}
		if _, exists := groups[group.ID]; exists {
			add(&errs, fmt.Sprintf("runwayGroups[%d].id", i), "is duplicated")
		}
		groups[group.ID] = group
		if previous, exists := groupNames[group.ID]; exists {
			add(&errs, fmt.Sprintf("runwayGroups[%d].id", i), "collides with "+previous)
		} else {
			groupNames[group.ID] = fmt.Sprintf("runwayGroups[%d].id", i)
		}
		for j, alias := range group.Aliases {
			if alias == "" {
				add(&errs, fmt.Sprintf("runwayGroups[%d].aliases[%d]", i, j), "is required")
				continue
			}
			if previous, exists := groupNames[alias]; exists {
				add(&errs, fmt.Sprintf("runwayGroups[%d].aliases[%d]", i, j), "collides with "+previous)
			} else {
				groupNames[alias] = fmt.Sprintf("runwayGroups[%d].aliases[%d]", i, j)
			}
		}
		if len(group.Runways) == 0 || len(group.FinalApproaches) == 0 {
			add(&errs, fmt.Sprintf("runwayGroups[%d]", i), "requires runways and final approaches")
		}
		groupRunways := map[navdata.RunwayID]bool{}
		for j, id := range group.Runways {
			if groupRunways[id] {
				add(&errs, fmt.Sprintf("runwayGroups[%d].runways[%d]", i, j), "is duplicated")
			}
			groupRunways[id] = true
			if _, ok := runways[id]; !ok {
				add(&errs, fmt.Sprintf("runwayGroups[%d].runways[%d]", i, j), "is absent from active dataset")
			}
		}
		finalRunways := map[navdata.RunwayID]int{}
		for j, definition := range group.FinalApproaches {
			final := definition.canonical()
			if !finitePositive(definition.PhysicalLengthM) {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d].physicalLengthM", i, j), "must be a positive published metre value")
			}
			finalRunways[final.Runway]++
			if finalRunways[final.Runway] > 1 {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d].runway", i, j), "is duplicated")
			}
			if err := final.Validate(); err != nil {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d]", i, j), err.Error())
				continue
			}
			runway, ok := runways[final.Runway]
			if !ok {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d].runway", i, j), "is absent from active dataset")
				continue
			}
			if !slices.Contains(group.Runways, final.Runway) {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d].runway", i, j), "is not in group runways")
			}
			if !sameThreshold(final.Threshold, runway.Threshold) || !sameCourse(final.CourseTrueDeg, runway.Threshold.CourseTrueDeg) {
				add(&errs, fmt.Sprintf("runwayGroups[%d].finalApproaches[%d]", i, j), "does not match active runway threshold/final course")
			}
		}
		for j, runway := range group.Runways {
			if finalRunways[runway] != 1 {
				add(&errs, fmt.Sprintf("runwayGroups[%d].runways[%d]", i, j), "requires exactly one final approach")
			}
		}
	}
	feeders := map[navdata.FeederID]bool{}
	feederNames := map[navdata.FeederID]string{}
	for i, f := range c.Feeders {
		if f.ID == "" {
			add(&errs, fmt.Sprintf("feeders[%d].id", i), "is required")
		} else if feeders[f.ID] {
			add(&errs, fmt.Sprintf("feeders[%d].id", i), "is duplicated")
		} else {
			feeders[f.ID] = true
		}
		if f.ID != "" {
			if previous, exists := feederNames[f.ID]; exists {
				add(&errs, fmt.Sprintf("feeders[%d].id", i), "collides with "+previous)
			} else {
				feederNames[f.ID] = fmt.Sprintf("feeders[%d].id", i)
			}
		}
		for j, alias := range f.Aliases {
			if alias == "" {
				add(&errs, fmt.Sprintf("feeders[%d].aliases[%d]", i, j), "is required")
				continue
			}
			if previous, exists := feederNames[alias]; exists {
				add(&errs, fmt.Sprintf("feeders[%d].aliases[%d]", i, j), "collides with "+previous)
			} else {
				feederNames[alias] = fmt.Sprintf("feeders[%d].aliases[%d]", i, j)
			}
		}
	}
	seenPaths := map[string]bool{}
	for i, path := range c.Paths {
		key := string(path.Feeder) + "/" + string(path.RunwayGroup)
		if !feeders[path.Feeder] {
			add(&errs, fmt.Sprintf("paths[%d].feeder", i), "is not configured")
		}
		if _, ok := groups[path.RunwayGroup]; !ok {
			add(&errs, fmt.Sprintf("paths[%d].runwayGroup", i), "is not configured")
		}
		if seenPaths[key] {
			add(&errs, fmt.Sprintf("paths[%d]", i), "duplicates feeder/runway group")
		}
		seenPaths[key] = true
		if len(path.Fixes) < 2 {
			add(&errs, fmt.Sprintf("paths[%d].fixes", i), "requires connected terminal fixes")
		}
		if len(path.Fixes) > 0 && canonicalFix(path.Fixes[0], aliases) != canonicalFix(navdata.FixID(path.Feeder), aliases) {
			add(&errs, fmt.Sprintf("paths[%d].fixes[0]", i), "must equal the configured feeder after normalization")
		}
		if len(path.Fixes) > 0 && path.Fixes[len(path.Fixes)-1] != path.MergeFix {
			add(&errs, fmt.Sprintf("paths[%d].mergeFix", i), "must be final path fix")
		}
		unique := map[navdata.FixID]bool{}
		for j, fix := range path.Fixes {
			fix = canonicalFix(fix, aliases)
			if !fixes[fix] {
				add(&errs, fmt.Sprintf("paths[%d].fixes[%d]", i, j), "is absent from active dataset")
			}
			if unique[fix] {
				add(&errs, fmt.Sprintf("paths[%d].fixes[%d]", i, j), "forms a cycle")
			}
			unique[fix] = true
		}
		holding, ok := holdings[path.SelectedHolding]
		if !ok {
			add(&errs, fmt.Sprintf("paths[%d].selectedHolding", i), "is missing or ambiguous in active/overlay holdings")
		} else if !containsCanonicalFix(path.Fixes, holding.Fix, aliases) {
			add(&errs, fmt.Sprintf("paths[%d].selectedHolding", i), "holding fix must occur on the terminal path")
		}
		if group, ok := groups[path.RunwayGroup]; ok && len(path.Fixes) > 0 {
			merge, found := refFix(refs.Fixes, canonicalFix(path.MergeFix, aliases))
			if found {
				for _, definition := range group.FinalApproaches {
					final := definition.canonical()
					if !plausibleIntercept(merge.Position, final.Threshold.Position, final.CourseTrueDeg) {
						add(&errs, fmt.Sprintf("paths[%d].mergeFix", i), "does not connect plausibly to final approach "+string(final.Runway))
					}
				}
			}
		}
	}
	for feeder := range feeders {
		for group := range groups {
			if !seenPaths[string(feeder)+"/"+string(group)] {
				add(&errs, "paths", fmt.Sprintf("missing enabled path for %s/%s", feeder, group))
			}
		}
	}
	if len(errs) > 0 {
		sort.Slice(errs, func(i, j int) bool { return errs[i].Error() < errs[j].Error() })
		return errs
	}
	return nil
}

func sameThreshold(a, b navdata.Threshold) bool {
	return near(a.Position.LatitudeDeg, b.Position.LatitudeDeg) && near(a.Position.LongitudeDeg, b.Position.LongitudeDeg)
}
func canonicalFix(id navdata.FixID, aliases map[navdata.FixID]navdata.FixID) navdata.FixID {
	if canonical, found := aliases[id]; found {
		return canonical
	}
	return id
}
func containsCanonicalFix(values []navdata.FixID, want navdata.FixID, aliases map[navdata.FixID]navdata.FixID) bool {
	for _, value := range values {
		if canonicalFix(value, aliases) == want {
			return true
		}
	}
	return false
}
func sameCourse(value float64, expected *float64) bool {
	return expected != nil && near(value, *expected)
}
func near(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.000001
}
func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}
func refFix(fixes []navdata.Fix, id navdata.FixID) (navdata.Fix, bool) {
	for _, fix := range fixes {
		if fix.ID == id {
			return fix, true
		}
	}
	return navdata.Fix{}, false
}
func plausibleIntercept(from, to navdata.Coordinate, course float64) bool {
	lat := math.Pi / 180
	dLon := (to.LongitudeDeg - from.LongitudeDeg) * lat
	y := math.Sin(dLon) * math.Cos(to.LatitudeDeg*lat)
	x := math.Cos(from.LatitudeDeg*lat)*math.Sin(to.LatitudeDeg*lat) - math.Sin(from.LatitudeDeg*lat)*math.Cos(to.LatitudeDeg*lat)*math.Cos(dLon)
	bearing := math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
	diff := math.Abs(bearing - course)
	if diff > 180 {
		diff = 360 - diff
	}
	return diff <= 90
}

func (c Configuration) Candidate(refs ReferenceSet, importedAt time.Time) (navdata.CandidateTerminalFragment, error) {
	if err := c.Validate(refs); err != nil {
		return navdata.CandidateTerminalFragment{}, err
	}
	paths := make([]navdata.TerminalPath, 0, len(c.Paths))
	aliases := map[navdata.FixID]navdata.FixID{}
	for _, alias := range c.FixAliases {
		aliases[alias.Alias] = alias.Canonical
	}
	provenance := navdata.Provenance{SourceID: "terminal-config:" + c.ConfigVersion, SourceRevision: c.ConfigVersion, ImportedAt: importedAt, EffectiveFrom: c.ApplicabilityFrom, EffectiveUntil: c.ApplicabilityUntil}
	for _, value := range c.Paths {
		legs := make([]navdata.ProcedureLeg, 0, len(value.Fixes)-1)
		for i := 1; i < len(value.Fixes); i++ {
			from, to := canonicalFix(value.Fixes[i-1], aliases), canonicalFix(value.Fixes[i], aliases)
			legs = append(legs, navdata.ProcedureLeg{ID: fmt.Sprintf("%s-%s-%02d", value.Feeder, value.RunwayGroup, i), PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &to})
		}
		path := navdata.TerminalPath{Version: refs.Version, Airport: c.Airport, Feeder: value.Feeder, RunwayGroup: value.RunwayGroup, Legs: legs, HoldingIDs: []navdata.HoldingID{value.SelectedHolding}, Coverage: navdata.CoverageComplete, Provenance: provenance}
		path.Digest = terminalDigest(path)
		paths = append(paths, path)
	}
	published := map[navdata.HoldingID]navdata.HoldingPattern{}
	for _, procedure := range refs.Procedures {
		for _, holding := range procedure.Holdings {
			published[holding.ID] = holding
		}
	}
	overlays := make([]navdata.HoldingPattern, 0, len(c.OverlayHoldings))
	for _, definition := range c.OverlayHoldings {
		holding := definition.canonical()
		if _, found := published[holding.ID]; !found {
			overlays = append(overlays, holding)
		}
	}
	validated := importedAt
	fragment := navdata.CandidateTerminalFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: refs.Version, Airport: c.Airport, ConfigVersion: c.ConfigVersion, Paths: paths, Holdings: overlays, Provenance: provenance, ImportedAt: importedAt, ValidatedAt: &validated, State: navdata.ValidationValidated}
	digest, err := navdata.CanonicalFragmentDigest(fragment.SchemaVersion, fragment.Version, fragment.Provenance, struct {
		Airport       navdata.AirportID
		ConfigVersion string
		Paths         []navdata.TerminalPath
		Holdings      []navdata.HoldingPattern
	}{fragment.Airport, fragment.ConfigVersion, fragment.Paths, fragment.Holdings})
	if err != nil {
		return navdata.CandidateTerminalFragment{}, err
	}
	fragment.Digest = digest
	return fragment, fragment.Validate()
}
func terminalDigest(path navdata.TerminalPath) string {
	digest, _ := navdata.CanonicalPayloadDigest(struct {
		Version    navdata.DatasetVersion
		Airport    navdata.AirportID
		Feeder     navdata.FeederID
		Group      aman.RunwayGroupID
		Legs       []navdata.ProcedureLeg
		Holdings   []navdata.HoldingID
		Provenance navdata.Provenance
	}{path.Version, path.Airport, path.Feeder, path.RunwayGroup, path.Legs, path.HoldingIDs, path.Provenance})
	return digest
}

// Store publishes only fully parsed and validated configurations. A failed
// reload leaves the last active configuration untouched.
type Store struct {
	mu     sync.RWMutex
	active Configuration
}

func (s *Store) Active() Configuration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConfiguration(s.active)
}
func (s *Store) Reload(path string, refs ReferenceSet) error {
	candidate, err := LoadFile(path)
	if err != nil {
		return err
	}
	if err = candidate.Validate(refs); err != nil {
		return err
	}
	s.mu.Lock()
	s.active = cloneConfiguration(candidate)
	s.mu.Unlock()
	return nil
}

func cloneConfiguration(value Configuration) Configuration {
	clone := value
	clone.Sources = slices.Clone(value.Sources)
	clone.RunwayGroups = make([]RunwayGroup, len(value.RunwayGroups))
	for i, group := range value.RunwayGroups {
		clone.RunwayGroups[i] = group
		clone.RunwayGroups[i].Aliases = slices.Clone(group.Aliases)
		clone.RunwayGroups[i].Runways = slices.Clone(group.Runways)
		clone.RunwayGroups[i].FinalApproaches = make([]FinalApproachDefinition, len(group.FinalApproaches))
		for j, final := range group.FinalApproaches {
			clone.RunwayGroups[i].FinalApproaches[j] = final
			clone.RunwayGroups[i].FinalApproaches[j].Threshold.ElevationFt = clonePointer(final.Threshold.ElevationFt)
			clone.RunwayGroups[i].FinalApproaches[j].Threshold.CourseTrueDeg = clonePointer(final.Threshold.CourseTrueDeg)
		}
	}
	clone.Feeders = make([]Feeder, len(value.Feeders))
	for i, feeder := range value.Feeders {
		clone.Feeders[i] = feeder
		clone.Feeders[i].Aliases = slices.Clone(feeder.Aliases)
	}
	clone.FixAliases = slices.Clone(value.FixAliases)
	clone.Paths = make([]Path, len(value.Paths))
	for i, path := range value.Paths {
		clone.Paths[i] = path
		clone.Paths[i].Fixes = slices.Clone(path.Fixes)
	}
	clone.OverlayHoldings = make([]HoldingDefinition, len(value.OverlayHoldings))
	for i, holding := range value.OverlayHoldings {
		clone.OverlayHoldings[i] = holding
		clone.OverlayHoldings[i].LegLengthNM = clonePointer(holding.LegLengthNM)
		clone.OverlayHoldings[i].LegTimeSeconds = clonePointer(holding.LegTimeSeconds)
		clone.OverlayHoldings[i].MinimumAltitudeFt = clonePointer(holding.MinimumAltitudeFt)
		clone.OverlayHoldings[i].MaximumAltitudeFt = clonePointer(holding.MaximumAltitudeFt)
		clone.OverlayHoldings[i].MaximumSpeedKt = clonePointer(holding.MaximumSpeedKt)
	}
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

type SelectionInput struct{ ExplicitFMP, SessionRunwayGroup *aman.RunwayGroupID }
type Selection struct {
	RunwayGroup    *aman.RunwayGroupID
	DegradedReason string
}

func (c Configuration) ResolveRunwayGroup(input SelectionInput) Selection {
	resolve := func(value *aman.RunwayGroupID) *aman.RunwayGroupID {
		if value == nil {
			return nil
		}
		for _, group := range c.RunwayGroups {
			if group.ID == *value || slices.Contains(group.Aliases, *value) {
				result := group.ID
				return &result
			}
		}
		return nil
	}
	if selected := resolve(input.ExplicitFMP); selected != nil {
		return Selection{RunwayGroup: selected}
	}
	if input.ExplicitFMP != nil {
		return Selection{DegradedReason: "explicit FMP runway group is not enabled"}
	}
	if selected := resolve(input.SessionRunwayGroup); selected != nil {
		return Selection{RunwayGroup: selected}
	}
	if input.SessionRunwayGroup != nil {
		return Selection{DegradedReason: "session runway group is not enabled"}
	}
	return Selection{DegradedReason: "no server-authoritative runway group configured"}
}
