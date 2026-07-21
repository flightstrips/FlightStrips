package navdata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// CanonicalSchemaVersion is bumped when a persisted canonical payload changes.
// It deliberately does not describe a provider, transport or parser version.
const CanonicalSchemaVersion = "navdata/v1"

type ValidationState string

const (
	ValidationCandidate ValidationState = "candidate"
	ValidationValidated ValidationState = "validated"
)

func (s ValidationState) Valid() bool { return s == ValidationCandidate || s == ValidationValidated }

// CandidateAirportFragment is the complete airport/runway fragment selected by
// a manifest. The explicit Runways slice keeps airport data separate from
// procedures and fixes.
type CandidateAirportFragment struct {
	SchemaVersion string
	Version       DatasetVersion
	Airport       Airport
	Runways       []Runway
	Provenance    Provenance
	ImportedAt    time.Time
	ValidatedAt   *time.Time
	State         ValidationState
	Digest        string
}

type CandidateProcedureFragment struct {
	SchemaVersion string
	Version       DatasetVersion
	Airport       AirportID
	Kind          ProcedureKind
	Procedures    []Procedure
	Coverage      Coverage
	Provenance    Provenance
	ImportedAt    time.Time
	ValidatedAt   *time.Time
	State         ValidationState
	Digest        string
}

// CandidateFixFragment contains the fixes referenced by the candidate
// manifest. It is intentionally not scoped to one airport: en-route fixes may
// be shared by several airport datasets.
type CandidateFixFragment struct {
	SchemaVersion string
	Version       DatasetVersion
	Fixes         []Fix
	Coverage      Coverage
	Provenance    Provenance
	ImportedAt    time.Time
	ValidatedAt   *time.Time
	State         ValidationState
	Digest        string
}

// CandidateTerminalFragment is only a persistence seam for official terminal
// geometry/configuration. Selection policy and EKCH content belong elsewhere.
type CandidateTerminalFragment struct {
	SchemaVersion string
	Version       DatasetVersion
	Airport       AirportID
	ConfigVersion string
	Paths         []TerminalPath
	// Holdings contains only official-AIP fallback definitions that were not
	// present in the canonical procedure fragments for this dataset.
	Holdings    []HoldingPattern
	Provenance  Provenance
	ImportedAt  time.Time
	ValidatedAt *time.Time
	State       ValidationState
	Digest      string
}

// RouteCandidate stores a resolver output independently from catalog
// fragments. ResolverVersion is part of its persistence key so changing the
// resolver deterministically produces a cache miss.
type RouteCandidate struct {
	Query           RouteQuery
	ResolverVersion string
	SchemaVersion   string
	Geometry        RouteGeometry
	CreatedAt       time.Time
}

// PersistenceKey is the storage key, derived from the #311 semantic RouteKey
// before any provider transformation plus resolver and schema versions.
func (c RouteCandidate) PersistenceKey() (RouteKey, error) {
	if err := c.Query.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(c.ResolverVersion) == "" || strings.TrimSpace(c.SchemaVersion) == "" {
		return "", invalid("route candidate resolver and schema versions are required")
	}
	key, err := c.Query.Key()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{string(key), c.ResolverVersion, c.SchemaVersion}, "\x1f")))
	return RouteKey(hex.EncodeToString(sum[:])), nil
}

func (c RouteCandidate) Validate() error {
	if err := c.Query.Validate(); err != nil {
		return err
	}
	if err := c.Geometry.Validate(); err != nil {
		return err
	}
	if !c.Query.Version.Equal(c.Geometry.Version) {
		return &aman.DomainError{Class: ErrorDatasetMismatch, Message: "route candidate query and geometry dataset versions differ"}
	}
	if err := utc("route created at", c.CreatedAt); err != nil {
		return err
	}
	want, err := RouteGeometryDigest(c.Query, c.Geometry)
	if err != nil {
		return err
	}
	if c.Geometry.Digest != want {
		return invalid("route geometry digest does not match canonical route")
	}
	_, err = c.PersistenceKey()
	return err
}

// ManifestCandidate names exact validated fragment revisions. ExpectedRevision
// is a compare-and-swap token: zero creates the first manifest for an airport.
type ManifestCandidate struct {
	Airport          AirportID
	Version          DatasetVersion
	AirportDigest    string
	ProcedureDigests []string
	FixDigest        string
	TerminalDigest   string
	ExpectedRevision int64
}

func (m ManifestCandidate) Validate() error {
	if !validIdentifier(string(m.Airport)) || m.ExpectedRevision < 0 {
		return invalid("manifest airport or expected revision is invalid")
	}
	if err := m.Version.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.AirportDigest) == "" || strings.TrimSpace(m.FixDigest) == "" {
		return invalid("manifest requires airport and fix fragments")
	}
	seen := map[string]struct{}{}
	for _, digest := range m.ProcedureDigests {
		if strings.TrimSpace(digest) == "" {
			return invalid("manifest procedure digest is required")
		}
		if _, exists := seen[digest]; exists {
			return invalid("manifest contains duplicate procedure fragment")
		}
		seen[digest] = struct{}{}
	}
	return nil
}

// NavigationCandidateWriter and NavigationManifestActivator are intentionally
// narrow consumer-owned seams. They avoid a catch-all Store that would blur
// import, validation, activation and runtime reads.
type NavigationCandidateWriter interface {
	PutAirportFragment(context.Context, CandidateAirportFragment) (string, error)
	PutProcedureFragment(context.Context, CandidateProcedureFragment) (string, error)
	PutFixFragment(context.Context, CandidateFixFragment) (string, error)
	PutTerminalFragment(context.Context, CandidateTerminalFragment) (string, error)
	PutRoute(context.Context, RouteCandidate) (RouteKey, error)
}

type NavigationManifestActivator interface {
	ActivateManifest(context.Context, ManifestCandidate) (int64, error)
	PruneNavigationCache(context.Context, AirportID) error
}

// ActiveManifestReader exposes the current complete manifest only to
// materializers. Runtime consumers remain on GeometryReader and cannot reach
// source acquisition or mutable candidate state.
type ActiveManifestReader interface {
	ActiveManifest(context.Context, AirportID) (ActiveManifest, error)
}

type ActiveManifest struct {
	Candidate  ManifestCandidate
	Revision   int64
	Procedures []ActiveProcedureFragment
}

type ActiveProcedureFragment struct {
	Kind       ProcedureKind
	Digest     string
	Procedures []Procedure
}

func (f CandidateAirportFragment) Validate() error {
	if err := validateFragment(f.SchemaVersion, f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.payload()); err != nil {
		return err
	}
	if f.Airport.ID == "" || f.Airport.Provenance != f.Provenance {
		return invalid("airport fragment airport provenance is inconsistent")
	}
	if err := f.Airport.Validate(); err != nil {
		return err
	}
	seen := map[RunwayID]struct{}{}
	for _, runway := range f.Runways {
		if err := runway.Validate(); err != nil {
			return err
		}
		// Airport reference metadata and physical threshold geometry may come
		// from separate authoritative publications. Their individual immutable
		// provenance is retained in the canonical payload and bound by this
		// fragment digest; forcing a shared source would fabricate one of them.
		if runway.Airport != f.Airport.ID {
			return invalid("runway is not owned by fragment airport")
		}
		if _, ok := seen[runway.ID]; ok {
			return invalid("airport fragment contains duplicate runway")
		}
		seen[runway.ID] = struct{}{}
	}
	return nil
}
func (f CandidateProcedureFragment) Validate() error {
	if !f.Kind.Valid() || !validIdentifier(string(f.Airport)) || !f.Coverage.Valid() {
		return invalid("procedure fragment airport or kind is invalid")
	}
	if err := validateFragment(f.SchemaVersion, f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.payload()); err != nil {
		return err
	}
	seen := map[ProcedureID]struct{}{}
	for _, p := range f.Procedures {
		if err := p.Validate(); err != nil {
			return err
		}
		if p.Airport != f.Airport || p.Kind != f.Kind || p.Provenance != f.Provenance {
			return invalid("procedure fragment contents are inconsistent")
		}
		if _, ok := seen[p.ID]; ok {
			return invalid("procedure fragment contains duplicate procedure")
		}
		seen[p.ID] = struct{}{}
	}
	return nil
}
func (f CandidateFixFragment) Validate() error {
	if !f.Coverage.Valid() {
		return invalid("fix fragment coverage is invalid")
	}
	if err := validateFragment(f.SchemaVersion, f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.payload()); err != nil {
		return err
	}
	seen := map[FixID]struct{}{}
	for _, fix := range f.Fixes {
		if err := fix.Validate(); err != nil {
			return err
		}
		if fix.Provenance != f.Provenance {
			return invalid("fix fragment provenance is inconsistent")
		}
		if _, ok := seen[fix.ID]; ok {
			return invalid("fix fragment contains duplicate fix")
		}
		seen[fix.ID] = struct{}{}
	}
	return nil
}
func (f CandidateTerminalFragment) Validate() error {
	if !validIdentifier(string(f.Airport)) || strings.TrimSpace(f.ConfigVersion) == "" {
		return invalid("terminal fragment airport or configuration version is invalid")
	}
	if err := validateFragment(f.SchemaVersion, f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.payload()); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	holdings := map[HoldingID]string{}
	for _, holding := range f.Holdings {
		if err := holding.Validate(); err != nil {
			return err
		}
		digest, err := HoldingDigest(holding)
		if err != nil {
			return err
		}
		if previous, found := holdings[holding.ID]; found && previous != digest {
			return invalid("terminal fragment has conflicting holding ID")
		}
		holdings[holding.ID] = digest
	}
	for _, path := range f.Paths {
		if err := path.Validate(); err != nil {
			return err
		}
		if path.Airport != f.Airport || !path.Version.Equal(f.Version) || path.Provenance != f.Provenance {
			return invalid("terminal fragment contents are inconsistent")
		}
		key := string(path.Feeder) + "\x1f" + string(path.RunwayGroup)
		if _, ok := seen[key]; ok {
			return invalid("terminal fragment contains duplicate path")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateFragment(schema string, version DatasetVersion, provenance Provenance, imported time.Time, validated *time.Time, state ValidationState, digest string, payload any) error {
	if schema != CanonicalSchemaVersion || !state.Valid() || strings.TrimSpace(digest) == "" {
		return invalid("fragment schema, state or digest is invalid")
	}
	if err := version.Validate(); err != nil {
		return err
	}
	if err := provenance.Validate(); err != nil {
		return err
	}
	if err := utc("fragment imported at", imported); err != nil {
		return err
	}
	if !imported.Equal(provenance.ImportedAt) {
		return invalid("fragment imported timestamp differs from provenance")
	}
	if state == ValidationValidated && validated == nil {
		return invalid("validated fragment requires validation timestamp")
	}
	if validated != nil {
		if err := utc("fragment validated at", *validated); err != nil {
			return err
		}
		if validated.Before(imported) {
			return invalid("fragment validated before import")
		}
	}
	want, err := CanonicalFragmentDigest(schema, version, provenance, payload)
	if err != nil {
		return err
	}
	if digest != want {
		return invalid("fragment digest does not match canonical payload")
	}
	return nil
}

// CanonicalFragmentDigest binds a provider-neutral payload to its canonical
// schema, dataset version and generic provenance. Validation timestamps and
// state are deliberately excluded so a candidate can become validated without
// changing the fragment revision.
func CanonicalFragmentDigest(schema string, version DatasetVersion, provenance Provenance, payload any) (string, error) {
	return CanonicalPayloadDigest(struct {
		SchemaVersion string
		Version       DatasetVersion
		Provenance    Provenance
		Payload       any
	}{schema, version, provenance, payload})
}

// CanonicalPayloadDigest is restricted to provider-neutral payload structs;
// callers never pass headers, URLs, vendor DTOs or retry/checkpoint state.
func CanonicalPayloadDigest(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode canonical payload: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (f CandidateAirportFragment) payload() any {
	return struct {
		Airport Airport
		Runways []Runway
	}{f.Airport, f.Runways}
}
func (f CandidateProcedureFragment) payload() any {
	return struct {
		Airport    AirportID
		Kind       ProcedureKind
		Procedures []Procedure
		Coverage   Coverage
	}{f.Airport, f.Kind, f.Procedures, f.Coverage}
}
func (f CandidateFixFragment) payload() any {
	return struct {
		Fixes    []Fix
		Coverage Coverage
	}{f.Fixes, f.Coverage}
}
func (f CandidateTerminalFragment) payload() any {
	return struct {
		Airport       AirportID
		ConfigVersion string
		Paths         []TerminalPath
		Holdings      []HoldingPattern
	}{f.Airport, f.ConfigVersion, f.Paths, f.Holdings}
}

func cloneProcedures(value []Procedure) []Procedure { return slices.Clone(value) }
