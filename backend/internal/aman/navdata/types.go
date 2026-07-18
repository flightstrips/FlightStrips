package navdata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

type AirportID string
type RunwayID string
type FixID string
type AirwayID string
type ProcedureID string
type HoldingID string
type FeederID string
type RouteKey string

// DatasetVersion identifies a particular imported AIRAC dataset.  It contains
// only navigation-data identity, never transport/cache implementation state.
type DatasetVersion struct {
	Cycle          string
	SourceRevision string
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
}

func (v DatasetVersion) Validate() error {
	if !validIdentifier(v.Cycle) || strings.TrimSpace(v.SourceRevision) == "" {
		return invalid("dataset version requires cycle and source revision")
	}
	if err := utc("dataset effective from", v.EffectiveFrom); err != nil {
		return err
	}
	if err := utc("dataset effective until", v.EffectiveUntil); err != nil {
		return err
	}
	if !v.EffectiveUntil.After(v.EffectiveFrom) {
		return invalid("dataset effective interval is invalid")
	}
	return nil
}

func (v DatasetVersion) Equal(other DatasetVersion) bool {
	return v.Cycle == other.Cycle && v.SourceRevision == other.SourceRevision &&
		v.EffectiveFrom.Equal(other.EffectiveFrom) && v.EffectiveUntil.Equal(other.EffectiveUntil)
}

// Provenance is persisted alongside canonical data. Provider transport details
// (URLs, cursors, HTTP headers and retry state) are intentionally absent.
type Provenance struct {
	SourceID       string
	SourceRevision string
	ImportedAt     time.Time
	EffectiveFrom  time.Time
	EffectiveUntil time.Time
}

func (p Provenance) Validate() error {
	if strings.TrimSpace(p.SourceID) == "" || strings.TrimSpace(p.SourceRevision) == "" {
		return invalid("provenance source is incomplete")
	}
	if err := utc("provenance imported at", p.ImportedAt); err != nil {
		return err
	}
	if err := utc("provenance effective from", p.EffectiveFrom); err != nil {
		return err
	}
	if err := utc("provenance effective until", p.EffectiveUntil); err != nil {
		return err
	}
	if !p.EffectiveUntil.After(p.EffectiveFrom) {
		return invalid("provenance effective interval is invalid")
	}
	return nil
}

type Coverage string

const (
	CoverageComplete    Coverage = "complete"
	CoveragePartial     Coverage = "partial"
	CoverageUnsupported Coverage = "unsupported"
	CoverageUnavailable Coverage = "unavailable"
)

func (c Coverage) Valid() bool {
	return c == CoverageComplete || c == CoveragePartial || c == CoverageUnsupported || c == CoverageUnavailable
}
func (c Coverage) Authoritative() bool { return c == CoverageComplete }

// Coordinate is WGS84 latitude/longitude in degrees.
type Coordinate struct{ LatitudeDeg, LongitudeDeg float64 }

func (c Coordinate) Validate() error {
	if c.LatitudeDeg < -90 || c.LatitudeDeg > 90 || c.LongitudeDeg < -180 || c.LongitudeDeg > 180 {
		return invalid("coordinate is outside WGS84 bounds")
	}
	return nil
}

type Airport struct {
	ID         AirportID
	Name       string
	Position   Coordinate
	Provenance Provenance
}
type Runway struct {
	ID         RunwayID
	Airport    AirportID
	Threshold  Threshold
	LengthNM   float64
	Provenance Provenance
}
type Fix struct {
	ID         FixID
	Position   Coordinate
	Provenance Provenance
}
type Airway struct {
	ID         AirwayID
	Fixes      []FixID
	Provenance Provenance
}
type Threshold struct {
	Position      Coordinate
	ElevationFt   *int
	CourseTrueDeg *float64
}
type FinalApproach struct {
	Runway        RunwayID
	Threshold     Threshold
	CourseTrueDeg float64
	Provenance    Provenance
}

type ProcedureKind string

const (
	ProcedureSID      ProcedureKind = "sid"
	ProcedureSTAR     ProcedureKind = "star"
	ProcedureApproach ProcedureKind = "approach"
)

func (k ProcedureKind) Valid() bool {
	return k == ProcedureSID || k == ProcedureSTAR || k == ProcedureApproach
}

// PathTerminator retains ARINC procedure semantics. Unsupported terminators are
// retained as PathUnsupported instead of being silently discarded.
type PathTerminator string

const (
	PathIF          PathTerminator = "IF"
	PathTF          PathTerminator = "TF"
	PathCF          PathTerminator = "CF"
	PathDF          PathTerminator = "DF"
	PathAF          PathTerminator = "AF"
	PathRF          PathTerminator = "RF"
	PathCA          PathTerminator = "CA"
	PathFA          PathTerminator = "FA"
	PathFC          PathTerminator = "FC"
	PathFD          PathTerminator = "FD"
	PathVA          PathTerminator = "VA"
	PathVM          PathTerminator = "VM"
	PathVI          PathTerminator = "VI"
	PathHA          PathTerminator = "HA"
	PathHF          PathTerminator = "HF"
	PathHM          PathTerminator = "HM"
	PathUnsupported PathTerminator = "UNSUPPORTED"
)

func (p PathTerminator) IsHolding() bool { return p == PathHA || p == PathHF || p == PathHM }
func (p PathTerminator) Supported() bool { return p != "" && p != PathUnsupported }

type TurnDirection string

const (
	TurnLeft  TurnDirection = "left"
	TurnRight TurnDirection = "right"
)

func (d TurnDirection) Valid() bool { return d == TurnLeft || d == TurnRight }

type HoldingTermination string

const (
	HoldingToAltitude HoldingTermination = "altitude"
	HoldingToFix      HoldingTermination = "fix"
	HoldingManual     HoldingTermination = "manual"
)

func (t HoldingTermination) Valid() bool {
	return t == HoldingToAltitude || t == HoldingToFix || t == HoldingManual
}

// HoldingPattern is published procedure geometry only. It never authorizes a
// predictor to add circuit delay without a separate operational policy/fact.
type HoldingPattern struct {
	ID                   HoldingID
	Fix                  FixID
	InboundCourseTrueDeg float64
	TurnDirection        TurnDirection
	LegLengthNM          *float64
	LegTimeSeconds       *int64
	MinimumAltitudeFt    *int
	MaximumAltitudeFt    *int
	MaximumSpeedKt       *int
	Termination          HoldingTermination
	Provenance           Provenance
}

func (h HoldingPattern) Validate() error {
	if !validIdentifier(string(h.ID)) || !validIdentifier(string(h.Fix)) {
		return invalid("holding identity is incomplete")
	}
	if h.InboundCourseTrueDeg < 0 || h.InboundCourseTrueDeg >= 360 {
		return invalid("holding inbound course must be true degrees in [0,360)")
	}
	if !h.TurnDirection.Valid() || !h.Termination.Valid() {
		return invalid("holding turn direction or termination is invalid")
	}
	if (h.LegLengthNM == nil) == (h.LegTimeSeconds == nil) {
		return invalid("holding requires exactly one time or distance construction")
	}
	if h.LegLengthNM != nil && *h.LegLengthNM <= 0 {
		return invalid("holding leg distance must be positive")
	}
	if h.LegTimeSeconds != nil && *h.LegTimeSeconds <= 0 {
		return invalid("holding leg time must be positive")
	}
	if h.MinimumAltitudeFt != nil && h.MaximumAltitudeFt != nil && *h.MinimumAltitudeFt > *h.MaximumAltitudeFt {
		return invalid("holding altitude constraints conflict")
	}
	if h.MaximumSpeedKt != nil && *h.MaximumSpeedKt <= 0 {
		return invalid("holding speed constraint must be positive")
	}
	return h.Provenance.Validate()
}

type ProcedureLeg struct {
	ID             string
	PathTerminator PathTerminator
	FromFix        *FixID
	ToFix          *FixID
	CourseTrueDeg  *float64
	DistanceNM     *float64
	HoldingID      *HoldingID
}

func (l ProcedureLeg) Validate() error {
	if strings.TrimSpace(l.ID) == "" || l.PathTerminator == "" {
		return invalid("procedure leg identity is incomplete")
	}
	if l.CourseTrueDeg != nil && (*l.CourseTrueDeg < 0 || *l.CourseTrueDeg >= 360) {
		return invalid("leg course must be true degrees in [0,360)")
	}
	if l.DistanceNM != nil && *l.DistanceNM < 0 {
		return invalid("leg distance cannot be negative")
	}
	if l.PathTerminator.IsHolding() && l.HoldingID == nil {
		return invalid("holding leg requires holding ID")
	}
	if !l.PathTerminator.IsHolding() && l.HoldingID != nil {
		return invalid("only holding legs may reference holding ID")
	}
	return nil
}

type Procedure struct {
	ID         ProcedureID
	Airport    AirportID
	Kind       ProcedureKind
	Runways    []RunwayID
	Legs       []ProcedureLeg
	Holdings   []HoldingPattern
	Provenance Provenance
}

func (p Procedure) Validate() error {
	if !validIdentifier(string(p.ID)) || !validIdentifier(string(p.Airport)) || !p.Kind.Valid() {
		return invalid("procedure identity is incomplete")
	}
	if err := p.Provenance.Validate(); err != nil {
		return err
	}
	holdings := make(map[HoldingID]struct{}, len(p.Holdings))
	for _, holding := range p.Holdings {
		if err := holding.Validate(); err != nil {
			return err
		}
		if _, found := holdings[holding.ID]; found {
			return invalid("procedure has duplicate holding ID")
		}
		holdings[holding.ID] = struct{}{}
	}
	for _, leg := range p.Legs {
		if err := leg.Validate(); err != nil {
			return err
		}
		if leg.HoldingID != nil {
			if _, found := holdings[*leg.HoldingID]; !found {
				return invalid("holding leg references missing holding")
			}
		}
	}
	return nil
}

type ProcedureQuery struct {
	Version     DatasetVersion
	Airport     AirportID
	Kinds       []ProcedureKind
	Runways     []RunwayID
	Identifiers []ProcedureID
}

func (q ProcedureQuery) Validate() error {
	if err := q.Version.Validate(); err != nil {
		return err
	}
	if !validIdentifier(string(q.Airport)) {
		return invalid("procedure query airport is required")
	}
	for _, kind := range q.Kinds {
		if !kind.Valid() {
			return invalid("procedure query kind is invalid")
		}
	}
	return nil
}

type ProcedureSet struct {
	Version    DatasetVersion
	Airport    AirportID
	Procedures []Procedure
	Coverage   Coverage
	Provenance Provenance
}

func (s ProcedureSet) Validate() error {
	if err := s.Version.Validate(); err != nil {
		return err
	}
	if !validIdentifier(string(s.Airport)) || !s.Coverage.Valid() {
		return invalid("procedure set identity or coverage is invalid")
	}
	if err := s.Provenance.Validate(); err != nil {
		return err
	}
	if s.Coverage == CoverageComplete && len(s.Procedures) == 0 {
		return invalid("complete procedure set cannot be empty")
	}
	for _, procedure := range s.Procedures {
		if procedure.Airport != s.Airport {
			return invalid("procedure set airport mismatch")
		}
		if err := procedure.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// HoldingDigest supplies a stable comparison value for equivalent published
// holdings across source implementations. It intentionally excludes adapter
// details and imported timestamps.
func HoldingDigest(holding HoldingPattern) (string, error) {
	if err := holding.Validate(); err != nil {
		return "", err
	}
	length, seconds, minimum, maximum, speed := "", "", "", "", ""
	if holding.LegLengthNM != nil {
		length = fmt.Sprintf("%.9f", *holding.LegLengthNM)
	}
	if holding.LegTimeSeconds != nil {
		seconds = fmt.Sprintf("%d", *holding.LegTimeSeconds)
	}
	if holding.MinimumAltitudeFt != nil {
		minimum = fmt.Sprintf("%d", *holding.MinimumAltitudeFt)
	}
	if holding.MaximumAltitudeFt != nil {
		maximum = fmt.Sprintf("%d", *holding.MaximumAltitudeFt)
	}
	if holding.MaximumSpeedKt != nil {
		speed = fmt.Sprintf("%d", *holding.MaximumSpeedKt)
	}
	values := []string{string(holding.ID), string(holding.Fix), fmt.Sprintf("%.9f", holding.InboundCourseTrueDeg), string(holding.TurnDirection), length, seconds, minimum, maximum, speed, string(holding.Termination), holding.Provenance.SourceID, holding.Provenance.SourceRevision, holding.Provenance.EffectiveFrom.UTC().Format(time.RFC3339), holding.Provenance.EffectiveUntil.UTC().Format(time.RFC3339)}
	sum := sha256.Sum256([]byte(strings.Join(values, "\x1f")))
	return hex.EncodeToString(sum[:]), nil
}

type FixQuery struct {
	Version     DatasetVersion
	Identifiers []FixID
}

func (q FixQuery) Validate() error {
	if err := q.Version.Validate(); err != nil {
		return err
	}
	if len(q.Identifiers) == 0 {
		return invalid("fix query identifiers are required")
	}
	return nil
}

type FixSet struct {
	Version    DatasetVersion
	Fixes      []Fix
	Coverage   Coverage
	Provenance Provenance
}

type RouteQuery struct {
	Version          DatasetVersion
	Origin           AirportID
	Destination      AirportID
	FiledRoute       string
	ArrivalProcedure *ProcedureID
	Runway           *RunwayID
	RunwayGroup      *aman.RunwayGroupID
}

func (q RouteQuery) Validate() error {
	if err := q.Version.Validate(); err != nil {
		return err
	}
	if !validIdentifier(string(q.Origin)) || !validIdentifier(string(q.Destination)) || normalizeRoute(q.FiledRoute) == "" {
		return invalid("route query is incomplete")
	}
	return nil
}
func (q RouteQuery) Key() (RouteKey, error) {
	if err := q.Validate(); err != nil {
		return "", err
	}
	semantic := strings.Join([]string{q.Version.Cycle, q.Version.SourceRevision, q.Version.EffectiveFrom.UTC().Format(time.RFC3339), q.Version.EffectiveUntil.UTC().Format(time.RFC3339), string(q.Origin), string(q.Destination), normalizeRoute(q.FiledRoute), optionalProcedure(q.ArrivalProcedure), optionalRunway(q.Runway), optionalGroup(q.RunwayGroup)}, "\x1f")
	sum := sha256.Sum256([]byte(semantic))
	return RouteKey(hex.EncodeToString(sum[:])), nil
}

type RouteGeometry struct {
	Version         DatasetVersion
	Legs            []ProcedureLeg
	HoldingIDs      []HoldingID
	TotalDistanceNM float64
	Coverage        Coverage
	Unresolved      []string
	Provenance      Provenance
	Digest          string
}

func (g RouteGeometry) Validate() error {
	if err := g.Version.Validate(); err != nil {
		return err
	}
	if !g.Coverage.Valid() || g.TotalDistanceNM < 0 || strings.TrimSpace(g.Digest) == "" {
		return invalid("route geometry is incomplete")
	}
	if err := g.Provenance.Validate(); err != nil {
		return err
	}
	for _, leg := range g.Legs {
		if err := leg.Validate(); err != nil {
			return err
		}
	}
	if g.Coverage.Authoritative() && len(g.Unresolved) > 0 {
		return invalid("complete geometry cannot retain unresolved elements")
	}
	return nil
}

type TerminalPath struct {
	Version     DatasetVersion
	Airport     AirportID
	Feeder      FeederID
	RunwayGroup aman.RunwayGroupID
	Legs        []ProcedureLeg
	HoldingIDs  []HoldingID
	Coverage    Coverage
	Unresolved  []string
	Provenance  Provenance
	Digest      string
}

func RouteGeometryDigest(query RouteQuery, legs []ProcedureLeg, holdings []HoldingID, coverage Coverage, unresolved []string) (string, error) {
	key, err := query.Key()
	if err != nil {
		return "", err
	}
	legTokens := make([]string, 0, len(legs))
	for _, leg := range legs {
		legTokens = append(legTokens, fmt.Sprintf("%s/%s/%s/%s", leg.ID, leg.PathTerminator, optionalFix(leg.FromFix), optionalFix(leg.ToFix)))
	}
	holdings = slices.Clone(holdings)
	slices.Sort(holdings)
	unresolved = slices.Clone(unresolved)
	slices.Sort(unresolved)
	sum := sha256.Sum256([]byte(strings.Join(append([]string{string(key), string(coverage)}, append(legTokens, append(stringifyHoldings(holdings), unresolved...)...)...), "\x1f")))
	return hex.EncodeToString(sum[:]), nil
}

func validIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value != strings.ToUpper(value) {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func utc(name string, value time.Time) error {
	if value.IsZero() || value.Location() != time.UTC {
		return invalid(name + " must be UTC")
	}
	return nil
}
func invalid(message string) error {
	return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: message}
}
func normalizeRoute(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(value), " "))
}
func optionalProcedure(value *ProcedureID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func optionalRunway(value *RunwayID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func optionalGroup(value *aman.RunwayGroupID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func optionalFix(value *FixID) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
func stringifyHoldings(values []HoldingID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}
