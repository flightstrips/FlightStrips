package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// navigationCache persists canonical navigation data only. It deliberately
// has no acquisition source or adapter dependency: runtime reads are cache
// reads and cannot turn into network/import requests.
type navigationCache struct{ pool *pgxpool.Pool }

func NewNavigationCache(pool *pgxpool.Pool) *navigationCache { return &navigationCache{pool: pool} }
func (*navigationCache) Name() string                        { return "postgres AMAN navigation cache" }

func (r *navigationCache) PutAirportFragment(ctx context.Context, value navdata.CandidateAirportFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Airport navdata.Airport
		Runways []navdata.Runway
	}{value.Airport, value.Runways})
	if err != nil {
		return "", err
	}
	provenance, _ := json.Marshal(value.Provenance)
	_, err = r.pool.Exec(ctx, `INSERT INTO aman_nav_airport_fragments (digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,provenance,validation_state,imported_at,validated_at,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (digest) DO UPDATE SET validation_state=EXCLUDED.validation_state, validated_at=EXCLUDED.validated_at WHERE aman_nav_airport_fragments.validation_state='candidate' AND EXCLUDED.validation_state='validated'`, value.Digest, value.SchemaVersion, value.Version.Cycle, value.Version.SourceRevision, value.Version.EffectiveFrom, value.Version.EffectiveUntil, value.Airport.ID, provenance, string(value.State), value.ImportedAt, value.ValidatedAt, payload)
	return value.Digest, err
}
func (r *navigationCache) PutProcedureFragment(ctx context.Context, value navdata.CandidateProcedureFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Airport    navdata.AirportID
		Kind       navdata.ProcedureKind
		Procedures []navdata.Procedure
		Coverage   navdata.Coverage
	}{value.Airport, value.Kind, value.Procedures, value.Coverage})
	if err != nil {
		return "", err
	}
	provenance, _ := json.Marshal(value.Provenance)
	_, err = r.pool.Exec(ctx, `INSERT INTO aman_nav_procedure_fragments (digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,procedure_kind,provenance,validation_state,imported_at,validated_at,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (digest) DO UPDATE SET validation_state=EXCLUDED.validation_state, validated_at=EXCLUDED.validated_at WHERE aman_nav_procedure_fragments.validation_state='candidate' AND EXCLUDED.validation_state='validated'`, value.Digest, value.SchemaVersion, value.Version.Cycle, value.Version.SourceRevision, value.Version.EffectiveFrom, value.Version.EffectiveUntil, value.Airport, string(value.Kind), provenance, string(value.State), value.ImportedAt, value.ValidatedAt, payload)
	return value.Digest, err
}
func (r *navigationCache) PutFixFragment(ctx context.Context, value navdata.CandidateFixFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Fixes    []navdata.Fix
		Coverage navdata.Coverage
	}{value.Fixes, value.Coverage})
	if err != nil {
		return "", err
	}
	provenance, _ := json.Marshal(value.Provenance)
	_, err = r.pool.Exec(ctx, `INSERT INTO aman_nav_fix_fragments (digest,schema_version,cycle,source_revision,effective_from,effective_until,provenance,validation_state,imported_at,validated_at,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (digest) DO UPDATE SET validation_state=EXCLUDED.validation_state, validated_at=EXCLUDED.validated_at WHERE aman_nav_fix_fragments.validation_state='candidate' AND EXCLUDED.validation_state='validated'`, value.Digest, value.SchemaVersion, value.Version.Cycle, value.Version.SourceRevision, value.Version.EffectiveFrom, value.Version.EffectiveUntil, provenance, string(value.State), value.ImportedAt, value.ValidatedAt, payload)
	return value.Digest, err
}
func (r *navigationCache) PutTerminalFragment(ctx context.Context, value navdata.CandidateTerminalFragment) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Airport       navdata.AirportID
		ConfigVersion string
		Paths         []navdata.TerminalPath
		Holdings      []navdata.HoldingPattern
	}{value.Airport, value.ConfigVersion, value.Paths, value.Holdings})
	if err != nil {
		return "", err
	}
	provenance, _ := json.Marshal(value.Provenance)
	_, err = r.pool.Exec(ctx, `INSERT INTO aman_nav_terminal_fragments (digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,config_version,provenance,validation_state,imported_at,validated_at,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT (digest) DO UPDATE SET validation_state=EXCLUDED.validation_state, validated_at=EXCLUDED.validated_at WHERE aman_nav_terminal_fragments.validation_state='candidate' AND EXCLUDED.validation_state='validated'`, value.Digest, value.SchemaVersion, value.Version.Cycle, value.Version.SourceRevision, value.Version.EffectiveFrom, value.Version.EffectiveUntil, value.Airport, value.ConfigVersion, provenance, string(value.State), value.ImportedAt, value.ValidatedAt, payload)
	return value.Digest, err
}

func (r *navigationCache) PutRoute(ctx context.Context, value navdata.RouteCandidate) (navdata.RouteKey, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	key, err := value.PersistenceKey()
	if err != nil {
		return "", err
	}
	semantic, _ := value.Query.Key()
	query, _ := json.Marshal(value.Query)
	payload, _ := json.Marshal(value.Geometry)
	provenance, _ := json.Marshal(value.Geometry.Provenance)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `INSERT INTO aman_nav_route_cache (cache_key,semantic_key,resolver_version,schema_version,route_digest,provenance,created_at,query,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (cache_key) DO NOTHING`, key, semantic, value.ResolverVersion, value.SchemaVersion, value.Geometry.Digest, provenance, value.CreatedAt, query, payload)
	if err != nil {
		return "", err
	}
	var digest string
	if err := tx.QueryRow(ctx, `SELECT route_digest FROM aman_nav_route_cache WHERE cache_key = $1`, key).Scan(&digest); err != nil {
		return "", err
	}
	if digest != value.Geometry.Digest {
		return "", cacheCorrupt("route cache key resolves to a different canonical digest")
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return key, nil
}

func (r *navigationCache) ActivateManifest(ctx context.Context, candidate navdata.ManifestCandidate) (int64, error) {
	if err := candidate.Validate(); err != nil {
		return 0, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	err = tx.QueryRow(ctx, `SELECT revision FROM aman_nav_active_manifests WHERE airport=$1 FOR UPDATE`, candidate.Airport).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		current = 0
	} else if err != nil {
		return 0, err
	}
	if current != candidate.ExpectedRevision {
		return 0, revisionConflict()
	}
	if err := r.validateManifest(ctx, tx, candidate); err != nil {
		return 0, err
	}
	digests, err := json.Marshal(candidate.ProcedureDigests)
	if err != nil {
		return 0, err
	}
	next := current + 1
	var manifestID int64
	err = tx.QueryRow(ctx, `INSERT INTO aman_nav_manifests (airport,revision,cycle,source_revision,effective_from,effective_until,airport_digest,procedure_digests,fix_digest,terminal_digest) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')) RETURNING manifest_id`, candidate.Airport, next, candidate.Version.Cycle, candidate.Version.SourceRevision, candidate.Version.EffectiveFrom, candidate.Version.EffectiveUntil, candidate.AirportDigest, digests, candidate.FixDigest, candidate.TerminalDigest).Scan(&manifestID)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, revisionConflict()
		}
		return 0, err
	}
	if current == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO aman_nav_active_manifests (airport,manifest_id,revision) VALUES ($1,$2,$3)`, candidate.Airport, manifestID, next)
	} else {
		_, err = tx.Exec(ctx, `UPDATE aman_nav_active_manifests SET manifest_id=$2, revision=$3 WHERE airport=$1 AND revision=$4`, candidate.Airport, manifestID, next, current)
	}
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return next, nil
}

func (r *navigationCache) ActiveVersion(ctx context.Context, airport navdata.AirportID) (navdata.DatasetVersion, error) {
	var v navdata.DatasetVersion
	var airportDigest string
	err := r.pool.QueryRow(ctx, `SELECT m.cycle,m.source_revision,m.effective_from,m.effective_until,m.airport_digest FROM aman_nav_active_manifests a JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport=$1`, airport).Scan(&v.Cycle, &v.SourceRevision, &v.EffectiveFrom, &v.EffectiveUntil, &airportDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, cacheNotFound("active dataset was not found")
	}
	if err != nil {
		return v, err
	}
	v.EffectiveFrom = v.EffectiveFrom.UTC()
	v.EffectiveUntil = v.EffectiveUntil.UTC()
	if err := v.Validate(); err != nil {
		return navdata.DatasetVersion{}, cacheCorrupt("stored active dataset is invalid")
	}
	fragment, err := loadAirportFragment(ctx, r.pool, airportDigest)
	if err != nil || fragment.State != navdata.ValidationValidated || !fragment.Version.Equal(v) {
		return navdata.DatasetVersion{}, cacheCorrupt("active airport fragment is invalid")
	}
	return v, nil
}

func (r *navigationCache) ActiveManifest(ctx context.Context, airport navdata.AirportID) (navdata.ActiveManifest, error) {
	var result navdata.ActiveManifest
	var digests []byte
	err := r.pool.QueryRow(ctx, `SELECT m.revision,m.cycle,m.source_revision,m.effective_from,m.effective_until,m.airport_digest,m.procedure_digests,m.fix_digest,COALESCE(m.terminal_digest,'') FROM aman_nav_active_manifests a JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport=$1`, airport).Scan(&result.Revision, &result.Candidate.Version.Cycle, &result.Candidate.Version.SourceRevision, &result.Candidate.Version.EffectiveFrom, &result.Candidate.Version.EffectiveUntil, &result.Candidate.AirportDigest, &digests, &result.Candidate.FixDigest, &result.Candidate.TerminalDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, cacheNotFound("active manifest was not found")
	}
	if err != nil {
		return result, err
	}
	result.Candidate.Airport = airport
	result.Candidate.ExpectedRevision = result.Revision
	result.Candidate.Version.EffectiveFrom = result.Candidate.Version.EffectiveFrom.UTC()
	result.Candidate.Version.EffectiveUntil = result.Candidate.Version.EffectiveUntil.UTC()
	if err := json.Unmarshal(digests, &result.Candidate.ProcedureDigests); err != nil {
		return navdata.ActiveManifest{}, cacheCorrupt("decode active manifest procedure digests")
	}
	for _, digest := range result.Candidate.ProcedureDigests {
		fragment, err := loadProcedureFragment(ctx, r.pool, digest)
		if err != nil {
			return navdata.ActiveManifest{}, err
		}
		result.Procedures = append(result.Procedures, navdata.ActiveProcedureFragment{Kind: fragment.Kind, Digest: digest, Procedures: fragment.Procedures})
	}
	return result, nil
}
func (r *navigationCache) Route(ctx context.Context, key navdata.RouteKey) (navdata.RouteGeometry, error) {
	var queryJSON, payload []byte
	var resolver, schema string
	var created time.Time
	err := r.pool.QueryRow(ctx, `SELECT query,payload,resolver_version,schema_version,created_at FROM aman_nav_route_cache WHERE cache_key=$1`, key).Scan(&queryJSON, &payload, &resolver, &schema, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return navdata.RouteGeometry{}, cacheNotFound("route geometry was not found")
	}
	if err != nil {
		return navdata.RouteGeometry{}, err
	}
	var candidate navdata.RouteCandidate
	if err := json.Unmarshal(queryJSON, &candidate.Query); err != nil {
		return navdata.RouteGeometry{}, cacheCorrupt("decode route query")
	}
	if err := json.Unmarshal(payload, &candidate.Geometry); err != nil {
		return navdata.RouteGeometry{}, cacheCorrupt("decode route geometry")
	}
	candidate.ResolverVersion = resolver
	candidate.SchemaVersion = schema
	candidate.CreatedAt = created.UTC()
	if err := candidate.Validate(); err != nil {
		return navdata.RouteGeometry{}, cacheCorrupt("validate route geometry")
	}
	got, _ := candidate.PersistenceKey()
	if got != key {
		return navdata.RouteGeometry{}, cacheCorrupt("stored route cache key mismatch")
	}
	return candidate.Geometry, nil
}
func (r *navigationCache) TerminalPath(ctx context.Context, airport navdata.AirportID, feeder navdata.FeederID, group aman.RunwayGroupID) (navdata.TerminalPath, error) {
	var digest string
	err := r.pool.QueryRow(ctx, `SELECT m.terminal_digest FROM aman_nav_active_manifests a JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport=$1`, airport).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return navdata.TerminalPath{}, cacheNotFound("terminal path was not found")
	}
	if err != nil {
		return navdata.TerminalPath{}, err
	}
	if digest == "" {
		return navdata.TerminalPath{}, cacheNotFound("terminal path was not found")
	}
	fragment, err := loadTerminalFragment(ctx, r.pool, digest)
	if err != nil || fragment.State != navdata.ValidationValidated {
		return navdata.TerminalPath{}, cacheCorrupt("active terminal fragment is invalid")
	}
	for _, path := range fragment.Paths {
		if path.Airport == airport && path.Feeder == feeder && path.RunwayGroup == group {
			if err := path.Validate(); err != nil {
				return navdata.TerminalPath{}, cacheCorrupt("validate terminal path")
			}
			return path, nil
		}
	}
	return navdata.TerminalPath{}, cacheNotFound("terminal path was not found")
}

// ActiveTerminalReferences supplies the small manifest-consistent cache view
// required by terminal configuration validation. It cannot acquire data.
func (r *navigationCache) ActiveTerminalReferences(ctx context.Context, airport navdata.AirportID) (terminal.ReferenceSet, error) {
	var result terminal.ReferenceSet
	var airportDigest, fixDigest string
	var procedureDigests []byte
	err := r.pool.QueryRow(ctx, `SELECT m.cycle,m.source_revision,m.effective_from,m.effective_until,m.airport_digest,m.fix_digest,m.procedure_digests FROM aman_nav_active_manifests a JOIN aman_nav_manifests m ON m.manifest_id=a.manifest_id WHERE a.airport=$1`, airport).Scan(&result.Version.Cycle, &result.Version.SourceRevision, &result.Version.EffectiveFrom, &result.Version.EffectiveUntil, &airportDigest, &fixDigest, &procedureDigests)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, cacheNotFound("active terminal references were not found")
	}
	if err != nil {
		return result, err
	}
	result.Version.EffectiveFrom = result.Version.EffectiveFrom.UTC()
	result.Version.EffectiveUntil = result.Version.EffectiveUntil.UTC()
	if err := result.Version.Validate(); err != nil {
		return terminal.ReferenceSet{}, cacheCorrupt("active terminal reference version is invalid")
	}
	airportFragment, err := loadAirportFragment(ctx, r.pool, airportDigest)
	if err != nil || airportFragment.State != navdata.ValidationValidated || !airportFragment.Version.Equal(result.Version) {
		return terminal.ReferenceSet{}, cacheCorrupt("active terminal airport fragment is invalid")
	}
	result.Airport = airportFragment.Airport
	result.Runways = airportFragment.Runways
	fixFragment, err := loadFixFragment(ctx, r.pool, fixDigest)
	if err != nil || fixFragment.State != navdata.ValidationValidated || fixFragment.Coverage != navdata.CoverageComplete || !fixFragment.Version.Equal(result.Version) {
		return terminal.ReferenceSet{}, cacheCorrupt("active terminal fix fragment is invalid")
	}
	result.Fixes = fixFragment.Fixes
	var digests []string
	if err := json.Unmarshal(procedureDigests, &digests); err != nil {
		return terminal.ReferenceSet{}, cacheCorrupt("decode terminal procedure digests")
	}
	for _, digest := range digests {
		fragment, err := loadProcedureFragment(ctx, r.pool, digest)
		if err != nil || fragment.State != navdata.ValidationValidated || fragment.Coverage == navdata.CoverageUnavailable || !fragment.Version.Equal(result.Version) {
			return terminal.ReferenceSet{}, cacheCorrupt("active terminal procedure fragment is invalid")
		}
		result.Procedures = append(result.Procedures, fragment.Procedures...)
	}
	return result, nil
}

func (r *navigationCache) PruneNavigationCache(ctx context.Context, airport navdata.AirportID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var current int64
	if err := tx.QueryRow(ctx, `SELECT revision FROM aman_nav_active_manifests WHERE airport=$1 FOR UPDATE`, airport).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM aman_nav_manifests WHERE airport=$1 AND revision < $2`, airport, current-1); err != nil {
		return err
	}
	for _, table := range []string{"aman_nav_airport_fragments", "aman_nav_fix_fragments", "aman_nav_terminal_fragments"} {
		if _, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s f WHERE NOT EXISTS (SELECT 1 FROM aman_nav_manifests m WHERE m.airport_digest=f.digest OR m.fix_digest=f.digest OR m.terminal_digest=f.digest)`, table)); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM aman_nav_procedure_fragments f WHERE NOT EXISTS (SELECT 1 FROM aman_nav_manifests m WHERE m.procedure_digests ? f.digest)`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *navigationCache) validateManifest(ctx context.Context, tx pgx.Tx, m navdata.ManifestCandidate) error {
	airport, err := loadAirportFragment(ctx, tx, m.AirportDigest)
	if err != nil {
		return err
	}
	if airport.State != navdata.ValidationValidated || airport.Airport.ID != m.Airport || !airport.Version.Equal(m.Version) {
		return cacheCorrupt("manifest airport fragment is not validated or does not match")
	}
	fixes, err := loadFixFragment(ctx, tx, m.FixDigest)
	if err != nil {
		return err
	}
	if fixes.State != navdata.ValidationValidated || fixes.Coverage != navdata.CoverageComplete || !fixes.Version.Equal(m.Version) {
		return cacheCorrupt("manifest fix fragment is not validated or does not match")
	}
	fixSet := map[navdata.FixID]bool{}
	for _, fix := range fixes.Fixes {
		fixSet[fix.ID] = true
	}
	runways := map[navdata.RunwayID]bool{}
	for _, runway := range airport.Runways {
		runways[runway.ID] = true
	}
	holdings := map[navdata.HoldingID]string{}
	procedureKinds := map[navdata.ProcedureKind]struct{}{}
	for _, digest := range m.ProcedureDigests {
		fragment, err := loadProcedureFragment(ctx, tx, digest)
		if err != nil {
			return err
		}
		if fragment.State != navdata.ValidationValidated || fragment.Coverage == navdata.CoverageUnavailable || fragment.Airport != m.Airport || !fragment.Version.Equal(m.Version) {
			return cacheCorrupt("manifest procedure fragment is not validated or does not match")
		}
		if _, found := procedureKinds[fragment.Kind]; found {
			return cacheCorrupt("manifest contains competing procedure fragments for one kind")
		}
		procedureKinds[fragment.Kind] = struct{}{}
		for _, procedure := range fragment.Procedures {
			for _, runway := range procedure.Runways {
				if !runways[runway] {
					return cacheCorrupt("procedure references missing runway")
				}
			}
			for _, holding := range procedure.Holdings {
				if !fixSet[holding.Fix] {
					return cacheCorrupt("holding references missing fix")
				}
				holdingDigest, err := navdata.HoldingDigest(holding)
				if err != nil {
					return cacheCorrupt("calculate holding digest")
				}
				if previous, found := holdings[holding.ID]; found && previous != holdingDigest {
					return cacheCorrupt("holding ID has conflicting canonical definitions")
				}
				holdings[holding.ID] = holdingDigest
			}
			for _, leg := range procedure.Legs {
				if leg.FromFix != nil && !fixSet[*leg.FromFix] {
					return cacheCorrupt("procedure references missing from fix")
				}
				if leg.ToFix != nil && !fixSet[*leg.ToFix] {
					return cacheCorrupt("procedure references missing to fix")
				}
			}
		}
	}
	if m.TerminalDigest != "" {
		terminal, err := loadTerminalFragment(ctx, tx, m.TerminalDigest)
		if err != nil {
			return err
		}
		if terminal.State != navdata.ValidationValidated || terminal.Airport != m.Airport || !terminal.Version.Equal(m.Version) {
			return cacheCorrupt("manifest terminal fragment is not validated or does not match")
		}
		for _, path := range terminal.Paths {
			if path.Coverage != navdata.CoverageComplete {
				return cacheCorrupt("terminal path is incomplete")
			}
			for _, leg := range path.Legs {
				if leg.FromFix != nil && !fixSet[*leg.FromFix] {
					return cacheCorrupt("terminal path references missing from fix")
				}
				if leg.ToFix != nil && !fixSet[*leg.ToFix] {
					return cacheCorrupt("terminal path references missing to fix")
				}
			}
		}
		for _, holding := range terminal.Holdings {
			if !fixSet[holding.Fix] {
				return cacheCorrupt("terminal holding references missing fix")
			}
			holdingDigest, err := navdata.HoldingDigest(holding)
			if err != nil {
				return cacheCorrupt("calculate terminal holding digest")
			}
			if previous, found := holdings[holding.ID]; found && previous != holdingDigest {
				return cacheCorrupt("terminal holding ID conflicts with canonical definition")
			}
			holdings[holding.ID] = holdingDigest
		}
		for _, path := range terminal.Paths {
			for _, holding := range path.HoldingIDs {
				if _, found := holdings[holding]; !found {
					return cacheCorrupt("terminal path references missing selected holding")
				}
			}
		}
	}
	return nil
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadAirportFragment(ctx context.Context, db rowQuerier, digest string) (navdata.CandidateAirportFragment, error) {
	var value navdata.CandidateAirportFragment
	var provenance, payload []byte
	err := db.QueryRow(ctx, `SELECT digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,provenance,validation_state,imported_at,validated_at,payload FROM aman_nav_airport_fragments WHERE digest=$1`, digest).Scan(&value.Digest, &value.SchemaVersion, &value.Version.Cycle, &value.Version.SourceRevision, &value.Version.EffectiveFrom, &value.Version.EffectiveUntil, &value.Airport.ID, &provenance, &value.State, &value.ImportedAt, &value.ValidatedAt, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, cacheCorrupt("manifest references missing airport fragment")
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(provenance, &value.Provenance); err != nil {
		return value, cacheCorrupt("decode airport provenance")
	}
	var body struct {
		Airport navdata.Airport
		Runways []navdata.Runway
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return value, cacheCorrupt("decode airport fragment")
	}
	value.Airport = body.Airport
	value.Runways = body.Runways
	value.Version.EffectiveFrom = value.Version.EffectiveFrom.UTC()
	value.Version.EffectiveUntil = value.Version.EffectiveUntil.UTC()
	value.ImportedAt = value.ImportedAt.UTC()
	if value.ValidatedAt != nil {
		t := value.ValidatedAt.UTC()
		value.ValidatedAt = &t
	}
	if err := value.Validate(); err != nil {
		return value, cacheCorrupt("validate airport fragment")
	}
	return value, nil
}
func loadProcedureFragment(ctx context.Context, db rowQuerier, digest string) (navdata.CandidateProcedureFragment, error) {
	var value navdata.CandidateProcedureFragment
	var provenance, payload []byte
	err := db.QueryRow(ctx, `SELECT digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,procedure_kind,provenance,validation_state,imported_at,validated_at,payload FROM aman_nav_procedure_fragments WHERE digest=$1`, digest).Scan(&value.Digest, &value.SchemaVersion, &value.Version.Cycle, &value.Version.SourceRevision, &value.Version.EffectiveFrom, &value.Version.EffectiveUntil, &value.Airport, &value.Kind, &provenance, &value.State, &value.ImportedAt, &value.ValidatedAt, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, cacheCorrupt("manifest references missing procedure fragment")
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(provenance, &value.Provenance); err != nil {
		return value, cacheCorrupt("decode procedure provenance")
	}
	var body struct {
		Airport    navdata.AirportID
		Kind       navdata.ProcedureKind
		Procedures []navdata.Procedure
		Coverage   navdata.Coverage
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return value, cacheCorrupt("decode procedure fragment")
	}
	value.Airport = body.Airport
	value.Kind = body.Kind
	value.Procedures = body.Procedures
	value.Coverage = body.Coverage
	value.Version.EffectiveFrom = value.Version.EffectiveFrom.UTC()
	value.Version.EffectiveUntil = value.Version.EffectiveUntil.UTC()
	value.ImportedAt = value.ImportedAt.UTC()
	if value.ValidatedAt != nil {
		t := value.ValidatedAt.UTC()
		value.ValidatedAt = &t
	}
	if err := value.Validate(); err != nil {
		return value, cacheCorrupt("validate procedure fragment")
	}
	return value, nil
}
func loadFixFragment(ctx context.Context, db rowQuerier, digest string) (navdata.CandidateFixFragment, error) {
	var value navdata.CandidateFixFragment
	var provenance, payload []byte
	err := db.QueryRow(ctx, `SELECT digest,schema_version,cycle,source_revision,effective_from,effective_until,provenance,validation_state,imported_at,validated_at,payload FROM aman_nav_fix_fragments WHERE digest=$1`, digest).Scan(&value.Digest, &value.SchemaVersion, &value.Version.Cycle, &value.Version.SourceRevision, &value.Version.EffectiveFrom, &value.Version.EffectiveUntil, &provenance, &value.State, &value.ImportedAt, &value.ValidatedAt, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, cacheCorrupt("manifest references missing fix fragment")
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(provenance, &value.Provenance); err != nil {
		return value, cacheCorrupt("decode fix provenance")
	}
	var body struct {
		Fixes    []navdata.Fix
		Coverage navdata.Coverage
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return value, cacheCorrupt("decode fix fragment")
	}
	value.Fixes = body.Fixes
	value.Coverage = body.Coverage
	value.Version.EffectiveFrom = value.Version.EffectiveFrom.UTC()
	value.Version.EffectiveUntil = value.Version.EffectiveUntil.UTC()
	value.ImportedAt = value.ImportedAt.UTC()
	if value.ValidatedAt != nil {
		t := value.ValidatedAt.UTC()
		value.ValidatedAt = &t
	}
	if err := value.Validate(); err != nil {
		return value, cacheCorrupt("validate fix fragment")
	}
	return value, nil
}
func loadTerminalFragment(ctx context.Context, db rowQuerier, digest string) (navdata.CandidateTerminalFragment, error) {
	var value navdata.CandidateTerminalFragment
	var provenance, payload []byte
	err := db.QueryRow(ctx, `SELECT digest,schema_version,cycle,source_revision,effective_from,effective_until,airport,config_version,provenance,validation_state,imported_at,validated_at,payload FROM aman_nav_terminal_fragments WHERE digest=$1`, digest).Scan(&value.Digest, &value.SchemaVersion, &value.Version.Cycle, &value.Version.SourceRevision, &value.Version.EffectiveFrom, &value.Version.EffectiveUntil, &value.Airport, &value.ConfigVersion, &provenance, &value.State, &value.ImportedAt, &value.ValidatedAt, &payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, cacheCorrupt("manifest references missing terminal fragment")
	}
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(provenance, &value.Provenance); err != nil {
		return value, cacheCorrupt("decode terminal provenance")
	}
	var body struct {
		Airport       navdata.AirportID
		ConfigVersion string
		Paths         []navdata.TerminalPath
		Holdings      []navdata.HoldingPattern
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return value, cacheCorrupt("decode terminal fragment")
	}
	value.Airport = body.Airport
	value.ConfigVersion = body.ConfigVersion
	value.Paths = body.Paths
	value.Holdings = body.Holdings
	value.Version.EffectiveFrom = value.Version.EffectiveFrom.UTC()
	value.Version.EffectiveUntil = value.Version.EffectiveUntil.UTC()
	value.ImportedAt = value.ImportedAt.UTC()
	if value.ValidatedAt != nil {
		t := value.ValidatedAt.UTC()
		value.ValidatedAt = &t
	}
	if err := value.Validate(); err != nil {
		return value, cacheCorrupt("validate terminal fragment")
	}
	return value, nil
}

func cacheNotFound(message string) error {
	return &aman.DomainError{Class: aman.ErrorNotFound, Message: "navigation cache: " + message}
}
func cacheCorrupt(message string) error {
	return &aman.DomainError{Class: aman.ErrorCorruptData, Message: "navigation cache: " + message}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var (
	_ navdata.NavigationCandidateWriter   = (*navigationCache)(nil)
	_ navdata.NavigationManifestActivator = (*navigationCache)(nil)
	_ navdata.ActiveManifestReader        = (*navigationCache)(nil)
	_ navdata.GeometryReader              = (*navigationCache)(nil)
	_ terminal.ReferenceReader            = (*navigationCache)(nil)
	_ aman.Component                      = (*navigationCache)(nil)
)
