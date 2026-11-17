package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"FlightStrips/internal/aman/navdata"
	"github.com/jackc/pgx/v5"
)

// The scope belongs to one cache instance and one synchronous event. It holds
// no connection or transaction while predictions and publications run.
type geometryReadScope struct {
	mu       sync.Mutex
	airport  navdata.AirportID
	snapshot *navdata.ActiveGeometrySnapshot
	routes   map[navdata.RouteKey]json.RawMessage
	decoded  map[navdata.RouteKey]navdata.RouteGeometry
}

func (r *navigationCache) geometryReadScope(ctx context.Context, airport navdata.AirportID) *geometryReadScope {
	scope, _ := ctx.Value(r).(*geometryReadScope)
	if scope != nil && (airport == "" || scope.airport == airport) {
		return scope
	}
	return nil
}

func (r *navigationCache) invalidateGeometryReadScope(ctx context.Context) {
	if scope := r.geometryReadScope(ctx, ""); scope != nil {
		scope.mu.Lock()
		scope.snapshot = nil
		scope.mu.Unlock()
	}
}

func (r *navigationCache) WithGeometryReadScope(ctx context.Context, airport navdata.AirportID, keys []navdata.RouteKey) (context.Context, error) {
	snapshot, routes, err := r.loadGeometryReadScope(ctx, airport, keys)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, r, &geometryReadScope{airport: airport, snapshot: &snapshot, routes: routes, decoded: make(map[navdata.RouteKey]navdata.RouteGeometry)}), nil
}

func (scope *geometryReadScope) route(key navdata.RouteKey) (navdata.RouteGeometry, bool, error) {
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if route, ok := scope.decoded[key]; ok {
		return route, true, nil
	}
	raw, ok := scope.routes[key]
	if !ok {
		return navdata.RouteGeometry{}, false, nil
	}
	route, err := decodeScopedRoute(key, raw)
	if err == nil {
		scope.decoded[key] = route
	}
	return route, true, err
}

func decodeScopedRoute(key navdata.RouteKey, raw json.RawMessage) (navdata.RouteGeometry, error) {
	var candidate navdata.RouteCandidate
	if err := json.Unmarshal(raw, &candidate); err != nil {
		return navdata.RouteGeometry{}, cacheCorrupt("decode route geometry")
	}
	candidate.CreatedAt = candidate.CreatedAt.UTC()
	if err := candidate.Validate(); err != nil {
		return navdata.RouteGeometry{}, cacheCorrupt("validate route geometry")
	}
	got, _ := candidate.PersistenceKey()
	if got != key {
		return navdata.RouteGeometry{}, cacheCorrupt("stored route cache key mismatch")
	}
	return candidate.Geometry, nil
}

// All fragment rows and requested immutable routes share one MVCC statement
// snapshot. Keep the full canonical validation: batching must not turn corrupt
// or missing fragment bytes into usable navigation geometry.
const geometryReadScopeSQL = `WITH manifest AS (
 SELECT m.* FROM aman_nav_active_manifests a
 JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport=$1
), fragments AS (
 SELECT 'airport' AS kind, to_jsonb(f) AS record FROM manifest m JOIN aman_nav_airport_fragments f ON f.digest=m.airport_digest
 UNION ALL
 SELECT 'fix', to_jsonb(f) FROM manifest m JOIN aman_nav_fix_fragments f ON f.digest=m.fix_digest
 UNION ALL
 SELECT 'procedure', to_jsonb(f) FROM manifest m CROSS JOIN LATERAL jsonb_array_elements_text(CASE WHEN jsonb_typeof(m.procedure_digests)='array' THEN m.procedure_digests ELSE '[]'::jsonb END) d(digest)
 JOIN aman_nav_procedure_fragments f ON f.digest=d.digest
 UNION ALL
 SELECT 'terminal', to_jsonb(f) FROM manifest m JOIN aman_nav_terminal_fragments f ON f.digest=m.terminal_digest
)
SELECT m.revision,m.cycle,m.source_revision,m.effective_from,m.effective_until,m.airport_digest,m.procedure_digests,m.fix_digest,COALESCE(m.terminal_digest,''),
 (SELECT jsonb_agg(jsonb_build_object('Kind',kind,'Value',record->'payload' || jsonb_build_object(
  'Digest',record->'digest','SchemaVersion',record->'schema_version',
  'Version',jsonb_build_object('Cycle',record->'cycle','SourceRevision',record->'source_revision','EffectiveFrom',record->'effective_from','EffectiveUntil',record->'effective_until'),
  'Provenance',record->'provenance','State',record->'validation_state','ImportedAt',record->'imported_at','ValidatedAt',record->'validated_at'))) FROM fragments),
 COALESCE((SELECT jsonb_object_agg(cache_key,jsonb_build_object('Query',query,'Geometry',payload,'ResolverVersion',resolver_version,'SchemaVersion',schema_version,'CreatedAt',created_at))
 FROM aman_nav_route_cache WHERE cache_key=ANY($2::text[])), '{}'::jsonb)
FROM manifest m`

func (r *navigationCache) loadGeometryReadScope(ctx context.Context, airport navdata.AirportID, keys []navdata.RouteKey) (navdata.ActiveGeometrySnapshot, map[navdata.RouteKey]json.RawMessage, error) {
	var result navdata.ActiveGeometrySnapshot
	var digests, fragmentsJSON, routesJSON []byte
	err := r.pool.QueryRow(ctx, geometryReadScopeSQL, airport, keys).Scan(&result.ManifestRevision, &result.Manifest.Version.Cycle, &result.Manifest.Version.SourceRevision, &result.Manifest.Version.EffectiveFrom, &result.Manifest.Version.EffectiveUntil, &result.Manifest.AirportDigest, &digests, &result.Manifest.FixDigest, &result.Manifest.TerminalDigest, &fragmentsJSON, &routesJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil, cacheNotFound("active geometry snapshot was not found")
	}
	if err != nil {
		return result, nil, err
	}
	result.Manifest.Airport, result.Manifest.ExpectedRevision = airport, result.ManifestRevision
	result.Manifest.Version.EffectiveFrom = result.Manifest.Version.EffectiveFrom.UTC()
	result.Manifest.Version.EffectiveUntil = result.Manifest.Version.EffectiveUntil.UTC()
	if err := json.Unmarshal(digests, &result.Manifest.ProcedureDigests); err != nil {
		return result, nil, cacheCorrupt("decode active snapshot procedure digests")
	}
	var fragments []struct {
		Kind  string
		Value json.RawMessage
	}
	if err := json.Unmarshal(fragmentsJSON, &fragments); err != nil {
		return result, nil, cacheCorrupt("decode snapshot fragments")
	}
	var airportFragment navdata.CandidateAirportFragment
	var fixFragment navdata.CandidateFixFragment
	var terminalFragment *navdata.CandidateTerminalFragment
	procedures := make(map[string]navdata.CandidateProcedureFragment)
	for _, fragment := range fragments {
		switch fragment.Kind {
		case "airport":
			err = decodeSnapshotFragment(fragment.Value, &airportFragment)
		case "fix":
			err = decodeSnapshotFragment(fragment.Value, &fixFragment)
		case "procedure":
			var value navdata.CandidateProcedureFragment
			err = decodeSnapshotFragment(fragment.Value, &value)
			procedures[value.Digest] = value
		case "terminal":
			terminalFragment = new(navdata.CandidateTerminalFragment)
			err = decodeSnapshotFragment(fragment.Value, terminalFragment)
		}
		if err != nil {
			return result, nil, err
		}
	}
	if airportFragment.Digest != result.Manifest.AirportDigest || fixFragment.Digest != result.Manifest.FixDigest {
		return result, nil, cacheCorrupt("manifest references missing geometry fragment")
	}
	ordered := make([]navdata.CandidateProcedureFragment, 0, len(result.Manifest.ProcedureDigests))
	for _, digest := range result.Manifest.ProcedureDigests {
		fragment, ok := procedures[digest]
		if !ok {
			return result, nil, cacheCorrupt("manifest references missing procedure fragment")
		}
		ordered = append(ordered, fragment)
		result.Procedures = append(result.Procedures, fragment.Procedures...)
		for _, procedure := range fragment.Procedures {
			result.Holdings = append(result.Holdings, procedure.Holdings...)
		}
	}
	if err := validateManifestFragments(result.Manifest, airportFragment, fixFragment, ordered, terminalFragment); err != nil {
		return result, nil, err
	}
	result.Airport, result.Runways, result.Fixes = airportFragment.Airport, airportFragment.Runways, fixFragment.Fixes
	if terminalFragment != nil {
		result.TerminalPaths, result.TerminalVersion = terminalFragment.Paths, terminalFragment.ConfigVersion
		result.TimelineMappings = terminalFragment.TimelineMappings
		result.Holdings = append(result.Holdings, terminalFragment.Holdings...)
	}
	// Validation above rejects conflicts; return each identical holding once.
	seen := make(map[navdata.HoldingID]bool)
	holdings := result.Holdings[:0]
	for _, holding := range result.Holdings {
		if !seen[holding.ID] {
			holdings = append(holdings, holding)
			seen[holding.ID] = true
		}
	}
	result.Holdings = holdings
	var routes map[navdata.RouteKey]json.RawMessage
	if err := json.Unmarshal(routesJSON, &routes); err != nil {
		return result, nil, cacheCorrupt("decode snapshot routes")
	}
	return result, routes, nil
}

func decodeSnapshotFragment(raw json.RawMessage, target interface{ Validate() error }) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return cacheCorrupt("decode snapshot fragment")
	}
	// PostgreSQL JSON timestamps carry a numeric UTC offset. Canonical domain
	// timestamps require time.UTC, including on hosts whose local zone is UTC.
	switch value := target.(type) {
	case *navdata.CandidateAirportFragment:
		normalizeFragmentTimes(&value.Version, &value.ImportedAt, &value.ValidatedAt)
	case *navdata.CandidateFixFragment:
		normalizeFragmentTimes(&value.Version, &value.ImportedAt, &value.ValidatedAt)
	case *navdata.CandidateProcedureFragment:
		normalizeFragmentTimes(&value.Version, &value.ImportedAt, &value.ValidatedAt)
	case *navdata.CandidateTerminalFragment:
		normalizeFragmentTimes(&value.Version, &value.ImportedAt, &value.ValidatedAt)
	}
	if err := target.Validate(); err != nil {
		return cacheCorrupt("validate snapshot fragment")
	}
	return nil
}

func normalizeFragmentTimes(version *navdata.DatasetVersion, imported *time.Time, validated **time.Time) {
	version.EffectiveFrom, version.EffectiveUntil = version.EffectiveFrom.UTC(), version.EffectiveUntil.UTC()
	*imported = imported.UTC()
	if *validated != nil {
		at := (*validated).UTC()
		*validated = &at
	}
}
