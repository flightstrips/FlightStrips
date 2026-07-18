package materializer

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestRefreshActivatesPartialProcedureCandidateAndTypedSTARKeepsOtherFragments(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, source, cache, configFor(data), &clock)

	require.NoError(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	first := cache.active
	require.Len(t, first.Candidate.ProcedureDigests, 3)
	partial := false
	for _, digest := range first.Candidate.ProcedureDigests {
		partial = partial || cache.procedures[digest].Coverage == navdata.CoveragePartial
	}
	require.True(t, partial, "unrelated partial procedure coverage remains explicit while terminal overlay stays usable")
	calls := source.Calls()

	require.NoError(t, m.Refresh(context.Background(), Request{Airport: "EKCH", ProcedureKinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}}))
	require.Equal(t, calls+5, source.Calls(), "typed STAR refresh queries only cycle, airport, runway, STAR and fixes")
	require.Len(t, cache.active.Candidate.ProcedureDigests, 3)
	require.Equal(t, int64(2), cache.active.Revision)
	require.True(t, m.Health("EKCH").CacheReady)
}

func TestFailedRefreshRetainsActiveManifestAndBadHoldingCannotActivate(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, source, cache, configFor(data), &clock)
	require.NoError(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	active := cache.active

	bad := configFor(data)
	bad.Paths[0].SelectedHolding = "MISSING"
	m.deps.Terminal = bad
	err := m.Refresh(context.Background(), Request{Airport: "EKCH"})
	require.Error(t, err)
	require.Equal(t, active.Candidate, cache.active.Candidate)
	require.Equal(t, ReasonTerminalGeometryBad, m.Health("EKCH").Reason)

	badData := completeEKCH()
	badData.Procedures[1].Holdings[0].Fix = "MISSING"
	badSource := fixture.New(badData)
	m.deps.Cycles, m.deps.Airports, m.deps.Runways, m.deps.Procedures, m.deps.Fixes = badSource, badSource, badSource, badSource, badSource
	err = m.Refresh(context.Background(), Request{Airport: "EKCH"})
	require.Error(t, err)
	require.Equal(t, active.Candidate, cache.active.Candidate, "bad published holding cannot replace the active manifest")
}

func TestStartupSourceFailureReportsUsableActiveCache(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, source, cache, configFor(data), &clock)
	require.NoError(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	m.deps.Cycles = unavailableCycle{}
	require.Error(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	health := m.Health("EKCH")
	require.Equal(t, ReasonSourceUnavailable, health.Reason)
	require.True(t, health.CacheReady)
	require.Equal(t, data.Version, *health.ActiveVersion)
}

func TestStartupEmptyAndExpiredDatasetsHaveStableHealth(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Version.EffectiveUntil
	m := newMaterializer(t, source, cache, configFor(data), &clock)
	require.Error(t, m.Refresh(context.Background(), Request{Airport: "EKCH"}))
	health := m.Health("EKCH")
	require.Equal(t, ReasonDatasetExpired, health.Reason)
	require.False(t, health.CacheReady)
	require.Nil(t, health.ActiveVersion)
	require.Equal(t, data.Version, *health.AvailableVersion)
}

func TestRouteMaterializationIsExplicitAndWarmCacheSurvivesResolverOutage(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, source, cache, configFor(data), &clock)
	procedure := navdata.ProcedureID("SOK1P")
	runway := navdata.RunwayID("22L")
	group := aman.RunwayGroupID("SOUTH")
	query := navdata.RouteQuery{Version: data.Version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX", ArrivalProcedure: &procedure, Runway: &runway, RunwayGroup: &group}
	key, err := m.MaterializeRoute(context.Background(), query, "fixture-v1")
	require.NoError(t, err)
	require.NotEmpty(t, key)
	before := source.Calls()
	_, ok := cache.routes[key]
	require.True(t, ok, "restart/outage readers use persisted route cache without resolver calls")
	require.Equal(t, before, source.Calls())
}

func TestConcurrentRefreshUsesCompareAndSwapWithoutPartialActivation(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	cache := &memoryCache{}
	clock := data.Provenance.ImportedAt.Add(time.Hour)
	m := newMaterializer(t, source, cache, configFor(data), &clock)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() { <-start; errs <- m.Refresh(context.Background(), Request{Airport: "EKCH"}) }()
	}
	close(start)
	first, second := <-errs, <-errs
	require.True(t, (first == nil) != (second == nil), "one CAS activation wins deterministically")
	require.Equal(t, int64(1), cache.active.Revision)
}

func TestNoEuroScopeOrHTTPDependency(t *testing.T) {
	data := completeEKCH()
	source := fixture.New(data)
	m := newMaterializer(t, source, &memoryCache{}, configFor(data), ptrTime(data.Provenance.ImportedAt.Add(time.Hour)))
	require.NotNil(t, m)
	var _ navdata.RunwaySource = terminal.Configuration{}
	var _ navdata.AirportSource = source
}

func newMaterializer(t *testing.T, source *fixture.Source, cache *memoryCache, config terminal.Configuration, clock *time.Time) *Materializer {
	t.Helper()
	m, err := New(Dependencies{Cycles: source, Airports: source, Runways: source, Procedures: source, Fixes: source, Routes: source, Cache: cache, Terminal: config, Now: func() time.Time { return *clock }})
	require.NoError(t, err)
	return m
}

func completeEKCH() fixture.Dataset {
	return fixture.EKCH()
}

func configFor(data fixture.Dataset) terminal.Configuration {
	from, until, imported := data.Version.EffectiveFrom, data.Version.EffectiveUntil, data.Provenance.ImportedAt
	course := 221.2
	return terminal.Configuration{SchemaVersion: terminal.SchemaVersion, ConfigVersion: "fixture-terminal", Airport: "EKCH", ApplicabilityFrom: from, ApplicabilityUntil: until, Dataset: terminal.DatasetCompatibility{Cycle: data.Version.Cycle, EffectiveFrom: from, EffectiveUntil: until}, Sources: []terminal.Source{{ID: "fixture-aip", Document: "official fixture", EffectiveFrom: from, EffectiveUntil: until}}, RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254111111, LongitudeDeg: 12.6675805556}, CourseTrueDeg: &course}, CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: terminal.ProvenanceDefinition{SourceID: "fixture", SourceRevision: "fixture-r1", ImportedAt: imported, EffectiveFrom: from, EffectiveUntil: until}}}}}, Feeders: []terminal.Feeder{{ID: "SOK"}}, Paths: []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}}}
}

type memoryCache struct {
	mu         sync.Mutex
	active     navdata.ActiveManifest
	hasActive  bool
	routes     map[navdata.RouteKey]navdata.RouteGeometry
	procedures map[string]navdata.CandidateProcedureFragment
}

func (c *memoryCache) PutAirportFragment(_ context.Context, value navdata.CandidateAirportFragment) (string, error) {
	return value.Digest, value.Validate()
}
func (c *memoryCache) PutProcedureFragment(_ context.Context, value navdata.CandidateProcedureFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.procedures == nil {
		c.procedures = map[string]navdata.CandidateProcedureFragment{}
	}
	c.procedures[value.Digest] = value
	return value.Digest, nil
}
func (c *memoryCache) PutFixFragment(_ context.Context, value navdata.CandidateFixFragment) (string, error) {
	return value.Digest, value.Validate()
}
func (c *memoryCache) PutTerminalFragment(_ context.Context, value navdata.CandidateTerminalFragment) (string, error) {
	return value.Digest, value.Validate()
}
func (c *memoryCache) PutRoute(_ context.Context, value navdata.RouteCandidate) (navdata.RouteKey, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	key, _ := value.PersistenceKey()
	c.mu.Lock()
	if c.routes == nil {
		c.routes = map[navdata.RouteKey]navdata.RouteGeometry{}
	}
	c.routes[key] = value.Geometry
	c.mu.Unlock()
	return key, nil
}
func (c *memoryCache) ActivateManifest(_ context.Context, value navdata.ManifestCandidate) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := int64(0)
	if c.hasActive {
		current = c.active.Revision
	}
	if value.ExpectedRevision != current {
		return 0, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "revision conflict"}
	}
	c.active = navdata.ActiveManifest{Candidate: value, Revision: current + 1}
	for _, digest := range value.ProcedureDigests {
		fragment := c.procedures[digest]
		c.active.Procedures = append(c.active.Procedures, navdata.ActiveProcedureFragment{Kind: fragment.Kind, Digest: digest, Procedures: fragment.Procedures})
	}
	c.hasActive = true
	return c.active.Revision, nil
}
func (c *memoryCache) PruneNavigationCache(context.Context, navdata.AirportID) error { return nil }
func (c *memoryCache) ActiveManifest(_ context.Context, _ navdata.AirportID) (navdata.ActiveManifest, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasActive {
		return navdata.ActiveManifest{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "missing"}
	}
	return c.active, nil
}
func (c *memoryCache) ActiveVersion(_ context.Context, _ navdata.AirportID) (navdata.DatasetVersion, error) {
	value, err := c.ActiveManifest(context.Background(), "EKCH")
	return value.Candidate.Version, err
}
func (c *memoryCache) Route(_ context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, found := c.routes[key]
	if !found {
		return navdata.RouteGeometry{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "route missing"}
	}
	return value, nil
}
func (c *memoryCache) TerminalPath(context.Context, navdata.AirportID, navdata.FeederID, aman.RunwayGroupID) (navdata.TerminalPath, error) {
	return navdata.TerminalPath{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "terminal missing"}
}
func ptrTime(value time.Time) *time.Time { return &value }

type unavailableCycle struct{}

func (unavailableCycle) LatestVersion(context.Context) (navdata.DatasetVersion, error) {
	return navdata.DatasetVersion{}, &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "offline"}
}
