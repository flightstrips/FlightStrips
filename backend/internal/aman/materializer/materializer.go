// Package materializer imports provider-neutral navigation data into the
// canonical cache. It is deliberately the only layer that sees acquisition
// interfaces; prediction and normal TETA processing receive GeometryReader.
package materializer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
)

const (
	ReasonNoActiveDataset     = "no active dataset"
	ReasonSourceUnavailable   = "source unavailable"
	ReasonCandidateInvalid    = "candidate invalid"
	ReasonDatasetExpired      = "dataset expired"
	ReasonTerminalGeometryBad = "terminal geometry invalid"
)

// Dependencies are explicit so replacing a provider only changes application
// composition. None of these interfaces imply an HTTP transport.
type Dependencies struct {
	Cycles     navdata.CycleSource
	Airports   navdata.AirportSource
	Runways    navdata.RunwaySource
	Procedures navdata.ProcedureSource
	Fixes      navdata.FixSource
	Routes     navdata.RouteResolver
	Cache      interface {
		navdata.NavigationCandidateWriter
		navdata.NavigationManifestActivator
		navdata.ActiveManifestReader
		navdata.GeometryReader
	}
	Terminal terminal.Configuration
	Now      func() time.Time
}

type Request struct {
	Airport        navdata.AirportID
	ProcedureKinds []navdata.ProcedureKind
	// FixIDs lets a route/procedure change declare newly required fixes. The
	// resulting immutable fix fragment still includes every terminal reference.
	FixIDs []navdata.FixID
}

// Health is technical readiness only. It intentionally contains no provider
// approval or rollout-policy decision.
type Health struct {
	Airport          navdata.AirportID
	ActiveVersion    *navdata.DatasetVersion
	AvailableVersion *navdata.DatasetVersion
	LastAttempt      time.Time
	LastSuccess      time.Time
	FragmentsValid   bool
	TerminalValid    bool
	CacheReady       bool
	Reason           string
}

type Materializer struct {
	deps   Dependencies
	mu     sync.RWMutex
	health map[navdata.AirportID]Health
}

func New(deps Dependencies) (*Materializer, error) {
	if deps.Cycles == nil || deps.Airports == nil || deps.Runways == nil || deps.Procedures == nil || deps.Fixes == nil || deps.Routes == nil || deps.Cache == nil {
		return nil, errors.New("navigation materializer requires all source, resolver and cache dependencies")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Materializer{deps: deps, health: map[navdata.AirportID]Health{}}, nil
}

func (*Materializer) Name() string { return "AMAN navigation materializer" }

// GeometryReader is the only navigation dependency supplied to normal
// prediction, validation, and TETA-tick consumers. It is cache-only.
func (m *Materializer) GeometryReader() navdata.GeometryReader { return m.deps.Cache }

// Run refreshes configured airports immediately, then on each interval. It is
// cancellation-aware and makes no assumptions about the source transport.
func (m *Materializer) Run(ctx context.Context, interval time.Duration, airports []navdata.AirportID) {
	refresh := func() {
		for _, airport := range airports {
			_ = m.Refresh(ctx, Request{Airport: airport})
		}
	}
	refresh()
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refresh()
		}
	}
}

// Refresh materializes a complete candidate before writing it. Candidate
// writes are inert until the single manifest activation transaction succeeds.
func (m *Materializer) Refresh(ctx context.Context, request Request) error {
	if request.Airport == "" {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, errors.New("refresh airport is required"))
	}
	m.markAttempt(request.Airport)
	current, currentErr := m.deps.Cache.ActiveManifest(ctx, request.Airport)
	if currentErr == nil {
		m.recordActive(request.Airport, current.Candidate.Version)
	} else if !isNotFound(currentErr) {
		m.invalidateActive(request.Airport, ReasonCandidateInvalid)
		if len(request.ProcedureKinds) > 0 || len(request.FixIDs) > 0 {
			return currentErr // typed refresh never retains a corrupt revision.
		}
	}
	version, err := m.deps.Cycles.LatestVersion(ctx)
	if err != nil {
		return m.failed(request.Airport, ReasonSourceUnavailable, false, false, err)
	}
	if err := version.Validate(); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	m.recordAvailable(request.Airport, version)
	if !m.deps.Now().UTC().Before(version.EffectiveUntil) {
		return m.failed(request.Airport, ReasonDatasetExpired, false, false, fmt.Errorf("dataset %s is expired", version.Cycle))
	}
	if currentErr == nil && !current.Candidate.Version.Equal(version) && len(request.ProcedureKinds) > 0 {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, errors.New("typed refresh cannot combine fragment revisions across datasets"))
	}
	kinds, err := normalizedKinds(request.ProcedureKinds)
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	airport, err := m.deps.Airports.Airport(ctx, version, request.Airport)
	if err != nil {
		return m.failed(request.Airport, ReasonSourceUnavailable, false, false, err)
	}
	runways, err := m.deps.Runways.Runways(ctx, version, request.Airport)
	if err != nil {
		return m.failed(request.Airport, ReasonTerminalGeometryBad, false, false, err)
	}
	sets := make(map[navdata.ProcedureKind]navdata.ProcedureSet, len(kinds))
	for _, kind := range kinds {
		set, err := m.deps.Procedures.Procedures(ctx, navdata.ProcedureQuery{Version: version, Airport: request.Airport, Kinds: []navdata.ProcedureKind{kind}})
		if err != nil {
			return m.failed(request.Airport, ReasonSourceUnavailable, false, false, err)
		}
		if err := set.Validate(); err != nil {
			return m.failed(request.Airport, ReasonCandidateInvalid, false, false, fmt.Errorf("validate %s procedures: %w", kind, err))
		}
		if set.Coverage == navdata.CoverageUnavailable {
			return m.failed(request.Airport, ReasonCandidateInvalid, false, false, fmt.Errorf("%s procedures are unavailable", kind))
		}
		sets[kind] = set
	}
	allProcedures := flattenProcedures(sets)
	if currentErr == nil && len(request.ProcedureKinds) > 0 {
		// Exact old revisions remain in the new manifest; the full candidate is
		// still validated before activation, never piecemeal replaced.
		for _, fragment := range current.Procedures {
			if !slices.Contains(kinds, fragment.Kind) {
				allProcedures = append(allProcedures, fragment.Procedures...)
			}
		}
	}
	fixes, err := m.deps.Fixes.Fixes(ctx, navdata.FixQuery{Version: version, Identifiers: requiredFixes(m.deps.Terminal, allProcedures, request.FixIDs)})
	if err != nil {
		return m.failed(request.Airport, ReasonSourceUnavailable, false, false, err)
	}
	if err := fixes.Validate(); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, fmt.Errorf("validate fixes: %w", err))
	}
	if fixes.Coverage != navdata.CoverageComplete {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, errors.New("fixes are not complete"))
	}
	refs := terminal.ReferenceSet{Version: version, Airport: airport, Runways: runways, Fixes: fixes.Fixes, Procedures: allProcedures}
	terminalFragment, err := m.deps.Terminal.Candidate(refs, m.deps.Now().UTC())
	if err != nil {
		return m.failed(request.Airport, ReasonTerminalGeometryBad, true, false, err)
	}
	if err := airport.Validate(); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	validated := m.deps.Now().UTC()
	airportFragment := navdata.CandidateAirportFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Airport: airport, Runways: runways, Provenance: airport.Provenance, ImportedAt: airport.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated}
	airportFragment.Digest, err = navdata.CanonicalFragmentDigest(airportFragment.SchemaVersion, version, airportFragment.Provenance, struct {
		Airport navdata.Airport
		Runways []navdata.Runway
	}{airport, runways})
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	if err = airportFragment.Validate(); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	fixFragment := navdata.CandidateFixFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Fixes: fixes.Fixes, Coverage: fixes.Coverage, Provenance: fixes.Provenance, ImportedAt: fixes.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated}
	fixFragment.Digest, err = navdata.CanonicalFragmentDigest(fixFragment.SchemaVersion, version, fixFragment.Provenance, struct {
		Fixes    []navdata.Fix
		Coverage navdata.Coverage
	}{fixFragment.Fixes, fixFragment.Coverage})
	if err != nil || fixFragment.Validate() != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, first(err, fixFragment.Validate()))
	}
	procedureFragments, err := procedureCandidates(version, request.Airport, sets, validated)
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	if _, err = m.deps.Cache.PutAirportFragment(ctx, airportFragment); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	fixDigest, err := m.deps.Cache.PutFixFragment(ctx, fixFragment)
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, false, false, err)
	}
	terminalDigest, err := m.deps.Cache.PutTerminalFragment(ctx, terminalFragment)
	if err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, true, false, err)
	}
	digests := retainedDigests(current, currentErr, kinds)
	for _, fragment := range procedureFragments {
		digest, putErr := m.deps.Cache.PutProcedureFragment(ctx, fragment)
		if putErr != nil {
			return m.failed(request.Airport, ReasonCandidateInvalid, false, false, putErr)
		}
		digests = append(digests, digest)
	}
	sort.Strings(digests)
	revision := current.Revision
	if _, err = m.deps.Cache.ActivateManifest(ctx, navdata.ManifestCandidate{Airport: request.Airport, Version: version, AirportDigest: airportFragment.Digest, ProcedureDigests: digests, FixDigest: fixDigest, TerminalDigest: terminalDigest, ExpectedRevision: revision}); err != nil {
		return m.failed(request.Airport, ReasonCandidateInvalid, true, false, err)
	}
	_ = m.deps.Cache.PruneNavigationCache(ctx, request.Airport)
	m.succeeded(request.Airport, version)
	return nil
}

// MaterializeRoute is called by flight-plan/route-change handling, never by a
// normal TETA tick. A warm cached key remains usable during a source outage.
func (m *Materializer) MaterializeRoute(ctx context.Context, query navdata.RouteQuery, resolverVersion string) (navdata.RouteKey, error) {
	geometry, err := m.deps.Routes.Resolve(ctx, query)
	if err != nil {
		return "", err
	}
	key, err := m.deps.Cache.PutRoute(ctx, navdata.RouteCandidate{Query: query, ResolverVersion: resolverVersion, SchemaVersion: navdata.CanonicalSchemaVersion, Geometry: geometry, CreatedAt: m.deps.Now().UTC()})
	return key, err
}

func (m *Materializer) Health(airport navdata.AirportID) Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, found := m.health[airport]
	if !found {
		return Health{Airport: airport, Reason: ReasonNoActiveDataset}
	}
	if value.ActiveVersion != nil {
		clone := *value.ActiveVersion
		value.ActiveVersion = &clone
	}
	if value.AvailableVersion != nil {
		clone := *value.AvailableVersion
		value.AvailableVersion = &clone
	}
	return value
}
func (m *Materializer) markAttempt(airport navdata.AirportID) {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport = airport
	value.LastAttempt = m.deps.Now().UTC()
	m.health[airport] = value
	m.mu.Unlock()
}
func (m *Materializer) recordActive(airport navdata.AirportID, version navdata.DatasetVersion) {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport, value.ActiveVersion, value.CacheReady, value.FragmentsValid, value.TerminalValid = airport, &version, true, true, true
	m.health[airport] = value
	m.mu.Unlock()
}
func (m *Materializer) recordAvailable(airport navdata.AirportID, version navdata.DatasetVersion) {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport, value.AvailableVersion = airport, &version
	m.health[airport] = value
	m.mu.Unlock()
}
func (m *Materializer) failed(airport navdata.AirportID, reason string, fragments, terminalValid bool, err error) error {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport, value.Reason = airport, reason
	if !value.CacheReady {
		value.FragmentsValid, value.TerminalValid = fragments, terminalValid
	}
	m.health[airport] = value
	m.mu.Unlock()
	return err
}
func (m *Materializer) invalidateActive(airport navdata.AirportID, reason string) {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport, value.Reason, value.ActiveVersion, value.CacheReady, value.FragmentsValid, value.TerminalValid = airport, reason, nil, false, false, false
	m.health[airport] = value
	m.mu.Unlock()
}
func (m *Materializer) succeeded(airport navdata.AirportID, version navdata.DatasetVersion) {
	m.mu.Lock()
	value := m.health[airport]
	value.Airport, value.ActiveVersion, value.AvailableVersion, value.LastSuccess, value.FragmentsValid, value.TerminalValid, value.CacheReady, value.Reason = airport, &version, &version, m.deps.Now().UTC(), true, true, true, ""
	m.health[airport] = value
	m.mu.Unlock()
}

func normalizedKinds(values []navdata.ProcedureKind) ([]navdata.ProcedureKind, error) {
	if len(values) == 0 {
		return []navdata.ProcedureKind{navdata.ProcedureSID, navdata.ProcedureSTAR, navdata.ProcedureApproach}, nil
	}
	seen := map[navdata.ProcedureKind]bool{}
	for _, value := range values {
		if !value.Valid() || seen[value] {
			return nil, errors.New("refresh procedure kinds are invalid")
		}
		seen[value] = true
	}
	return slices.Clone(values), nil
}
func flattenProcedures(sets map[navdata.ProcedureKind]navdata.ProcedureSet) []navdata.Procedure {
	var result []navdata.Procedure
	for _, set := range sets {
		result = append(result, set.Procedures...)
	}
	return result
}
func requiredFixes(config terminal.Configuration, procedures []navdata.Procedure, requested []navdata.FixID) []navdata.FixID {
	seen := map[navdata.FixID]bool{}
	add := func(id navdata.FixID) {
		if id != "" {
			seen[id] = true
		}
	}
	for _, path := range config.Paths {
		for _, fix := range path.Fixes {
			add(fix)
		}
	}
	for _, holding := range config.OverlayHoldings {
		add(holding.Fix)
	}
	for _, fix := range requested {
		add(fix)
	}
	for _, procedure := range procedures {
		for _, holding := range procedure.Holdings {
			add(holding.Fix)
		}
		for _, leg := range procedure.Legs {
			if leg.FromFix != nil {
				add(*leg.FromFix)
			}
			if leg.ToFix != nil {
				add(*leg.ToFix)
			}
		}
	}
	result := make([]navdata.FixID, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
func procedureCandidates(version navdata.DatasetVersion, airport navdata.AirportID, sets map[navdata.ProcedureKind]navdata.ProcedureSet, validated time.Time) ([]navdata.CandidateProcedureFragment, error) {
	result := make([]navdata.CandidateProcedureFragment, 0, len(sets))
	for kind, set := range sets {
		fragment := navdata.CandidateProcedureFragment{SchemaVersion: navdata.CanonicalSchemaVersion, Version: version, Airport: airport, Kind: kind, Procedures: set.Procedures, Coverage: set.Coverage, Provenance: set.Provenance, ImportedAt: set.Provenance.ImportedAt, ValidatedAt: &validated, State: navdata.ValidationValidated}
		digest, err := navdata.CanonicalFragmentDigest(fragment.SchemaVersion, version, fragment.Provenance, struct {
			Airport    navdata.AirportID
			Kind       navdata.ProcedureKind
			Procedures []navdata.Procedure
			Coverage   navdata.Coverage
		}{airport, kind, fragment.Procedures, fragment.Coverage})
		if err != nil {
			return nil, err
		}
		fragment.Digest = digest
		if err = fragment.Validate(); err != nil {
			return nil, err
		}
		result = append(result, fragment)
	}
	return result, nil
}
func retainedDigests(current navdata.ActiveManifest, currentErr error, replaced []navdata.ProcedureKind) []string {
	if currentErr != nil {
		return nil
	}
	result := make([]string, 0, len(current.Procedures))
	for _, fragment := range current.Procedures {
		if !slices.Contains(replaced, fragment.Kind) {
			result = append(result, fragment.Digest)
		}
	}
	return result
}
func isNotFound(err error) bool {
	var domain *aman.DomainError
	return errors.As(err, &domain) && domain.Class == aman.ErrorNotFound
}
func first(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
