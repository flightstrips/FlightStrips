package navdata

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"strconv"
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
	if !finite(c.LatitudeDeg) || !finite(c.LongitudeDeg) {
		return invalid("coordinate must be finite")
	}
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

func (a Airport) Validate() error {
	if !validIdentifier(string(a.ID)) || strings.TrimSpace(a.Name) == "" {
		return invalid("airport identity is incomplete")
	}
	if err := a.Position.Validate(); err != nil {
		return err
	}
	return a.Provenance.Validate()
}

type Runway struct {
	ID         RunwayID
	Airport    AirportID
	Threshold  Threshold
	LengthNM   float64
	Provenance Provenance
}

func (r Runway) Validate() error {
	if !validIdentifier(string(r.ID)) || !validIdentifier(string(r.Airport)) || !finite(r.LengthNM) || r.LengthNM <= 0 {
		return invalid("runway identity or length is invalid")
	}
	if err := r.Threshold.Validate(); err != nil {
		return err
	}
	return r.Provenance.Validate()
}

type Fix struct {
	ID         FixID
	Position   Coordinate
	Provenance Provenance
}

func (f Fix) Validate() error {
	if !validIdentifier(string(f.ID)) {
		return invalid("fix ID is invalid")
	}
	if err := f.Position.Validate(); err != nil {
		return err
	}
	return f.Provenance.Validate()
}

type Airway struct {
	ID         AirwayID
	Fixes      []FixID
	Provenance Provenance
}

func (a Airway) Validate() error {
	if !validIdentifier(string(a.ID)) || len(a.Fixes) < 2 {
		return invalid("airway identity or fixes are invalid")
	}
	seen := map[FixID]struct{}{}
	for _, fix := range a.Fixes {
		if !validIdentifier(string(fix)) {
			return invalid("airway fix is invalid")
		}
		if _, found := seen[fix]; found {
			return invalid("airway contains duplicate fix")
		}
		seen[fix] = struct{}{}
	}
	return a.Provenance.Validate()
}

type Threshold struct {
	Position      Coordinate
	ElevationFt   *int
	CourseTrueDeg *float64
}

func (t Threshold) Validate() error {
	if err := t.Position.Validate(); err != nil {
		return err
	}
	if t.CourseTrueDeg != nil && (!finite(*t.CourseTrueDeg) || *t.CourseTrueDeg < 0 || *t.CourseTrueDeg >= 360) {
		return invalid("threshold course must be true degrees in [0,360)")
	}
	return nil
}

type FinalApproach struct {
	Runway        RunwayID
	Threshold     Threshold
	CourseTrueDeg float64
	Provenance    Provenance
}

func (f FinalApproach) Validate() error {
	if !validIdentifier(string(f.Runway)) || !finite(f.CourseTrueDeg) || f.CourseTrueDeg < 0 || f.CourseTrueDeg >= 360 {
		return invalid("final approach identity or course is invalid")
	}
	if err := f.Threshold.Validate(); err != nil {
		return err
	}
	return f.Provenance.Validate()
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
func (p PathTerminator) Supported() bool {
	switch p {
	case PathIF, PathTF, PathCF, PathDF, PathAF, PathRF, PathCA, PathFA, PathFC, PathFD, PathVA, PathVM, PathVI, PathHA, PathHF, PathHM:
		return true
	default:
		return false
	}
}

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
	if !finite(h.InboundCourseTrueDeg) || h.InboundCourseTrueDeg < 0 || h.InboundCourseTrueDeg >= 360 {
		return invalid("holding inbound course must be true degrees in [0,360)")
	}
	if !h.TurnDirection.Valid() || !h.Termination.Valid() {
		return invalid("holding turn direction or termination is invalid")
	}
	if (h.LegLengthNM == nil) == (h.LegTimeSeconds == nil) {
		return invalid("holding requires exactly one time or distance construction")
	}
	if h.LegLengthNM != nil && (!finite(*h.LegLengthNM) || *h.LegLengthNM <= 0) {
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
	if !validIdentifier(l.ID) || l.PathTerminator == "" {
		return invalid("procedure leg identity is incomplete")
	}
	if l.CourseTrueDeg != nil && (!finite(*l.CourseTrueDeg) || *l.CourseTrueDeg < 0 || *l.CourseTrueDeg >= 360) {
		return invalid("leg course must be true degrees in [0,360)")
	}
	if l.DistanceNM != nil && (!finite(*l.DistanceNM) || *l.DistanceNM < 0) {
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
	runways := make(map[RunwayID]struct{}, len(p.Runways))
	for _, runway := range p.Runways {
		if !validIdentifier(string(runway)) {
			return invalid("procedure runway is invalid")
		}
		if _, found := runways[runway]; found {
			return invalid("procedure contains duplicate runway")
		}
		runways[runway] = struct{}{}
	}
	holdings := make(map[HoldingID]HoldingPattern, len(p.Holdings))
	for _, holding := range p.Holdings {
		if err := holding.Validate(); err != nil {
			return err
		}
		if _, found := holdings[holding.ID]; found {
			return invalid("procedure has duplicate holding ID")
		}
		holdings[holding.ID] = holding
	}
	legIDs := make(map[string]struct{}, len(p.Legs))
	for _, leg := range p.Legs {
		if err := leg.Validate(); err != nil {
			return err
		}
		if _, found := legIDs[leg.ID]; found {
			return invalid("procedure contains duplicate leg ID")
		}
		legIDs[leg.ID] = struct{}{}
		if leg.HoldingID != nil {
			holding, found := holdings[*leg.HoldingID]
			if !found {
				return invalid("holding leg references missing holding")
			}
			if (leg.PathTerminator == PathHA && holding.Termination != HoldingToAltitude) || (leg.PathTerminator == PathHF && holding.Termination != HoldingToFix) || (leg.PathTerminator == PathHM && holding.Termination != HoldingManual) {
				return invalid("holding leg terminator does not match holding termination")
			}
			if (leg.FromFix != nil && *leg.FromFix != holding.Fix) || (leg.ToFix != nil && *leg.ToFix != holding.Fix) {
				return invalid("holding leg fix does not match holding definition")
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
	kinds := map[ProcedureKind]struct{}{}
	for _, kind := range q.Kinds {
		if !kind.Valid() {
			return invalid("procedure query kind is invalid")
		}
		if _, found := kinds[kind]; found {
			return invalid("procedure query has duplicate kind")
		}
		kinds[kind] = struct{}{}
	}
	if err := uniqueRunways(q.Runways); err != nil {
		return err
	}
	if err := uniqueProcedures(q.Identifiers); err != nil {
		return err
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
	if s.Coverage == CoverageUnavailable && len(s.Procedures) > 0 {
		return invalid("unavailable procedure set cannot carry procedures")
	}
	for _, procedure := range s.Procedures {
		if procedure.Airport != s.Airport {
			return invalid("procedure set airport mismatch")
		}
		if err := procedure.Validate(); err != nil {
			return err
		}
		if s.Coverage == CoverageComplete && procedure.HasUnsupportedLeg() {
			return invalid("complete procedure set cannot retain unsupported leg")
		}
	}
	return nil
}
func (p Procedure) HasUnsupportedLeg() bool {
	for _, leg := range p.Legs {
		if !leg.PathTerminator.Supported() {
			return true
		}
	}
	return false
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
	values := []string{string(holding.ID), string(holding.Fix), fmt.Sprintf("%.9f", holding.InboundCourseTrueDeg), string(holding.TurnDirection), length, seconds, minimum, maximum, speed, string(holding.Termination)}
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
	fixes := map[FixID]struct{}{}
	for _, fix := range q.Identifiers {
		if !validIdentifier(string(fix)) {
			return invalid("fix query identifier is invalid")
		}
		if _, found := fixes[fix]; found {
			return invalid("fix query contains duplicate identifier")
		}
		fixes[fix] = struct{}{}
	}
	return nil
}

type FixSet struct {
	Version    DatasetVersion
	Fixes      []Fix
	Coverage   Coverage
	Provenance Provenance
}

func (s FixSet) Validate() error {
	if err := s.Version.Validate(); err != nil {
		return err
	}
	if !s.Coverage.Valid() {
		return invalid("fix set coverage is invalid")
	}
	if err := s.Provenance.Validate(); err != nil {
		return err
	}
	if s.Coverage == CoverageComplete && len(s.Fixes) == 0 {
		return invalid("complete fix set cannot be empty")
	}
	if s.Coverage == CoverageUnavailable && len(s.Fixes) > 0 {
		return invalid("unavailable fix set cannot carry fixes")
	}
	seen := map[FixID]struct{}{}
	for _, fix := range s.Fixes {
		if err := fix.Validate(); err != nil {
			return err
		}
		if _, found := seen[fix.ID]; found {
			return invalid("fix set contains duplicate fix")
		}
		seen[fix.ID] = struct{}{}
	}
	return nil
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
	if q.ArrivalProcedure != nil && !validIdentifier(string(*q.ArrivalProcedure)) {
		return invalid("route query arrival procedure is invalid")
	}
	if q.Runway != nil && !validIdentifier(string(*q.Runway)) {
		return invalid("route query runway is invalid")
	}
	if q.RunwayGroup != nil && !validIdentifier(string(*q.RunwayGroup)) {
		return invalid("route query runway group is invalid")
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
	if err := g.validateCanonical(); err != nil {
		return err
	}
	if strings.TrimSpace(g.Digest) == "" {
		return invalid("route geometry digest is required")
	}
	return nil
}

func (g RouteGeometry) validateCanonical() error {
	if err := g.Version.Validate(); err != nil {
		return err
	}
	if !g.Coverage.Valid() || !finite(g.TotalDistanceNM) || g.TotalDistanceNM < 0 {
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
	if g.Coverage == CoverageComplete && (len(g.Unresolved) > 0 || hasUnsupportedLeg(g.Legs)) {
		return invalid("complete geometry cannot retain unresolved or unsupported legs")
	}
	if g.Coverage == CoverageUnavailable && (len(g.Legs) > 0 || len(g.HoldingIDs) > 0 || len(g.Unresolved) > 0) {
		return invalid("unavailable geometry cannot carry data")
	}
	return uniqueHoldings(g.HoldingIDs)
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

func (p TerminalPath) Validate() error {
	if err := p.Version.Validate(); err != nil {
		return err
	}
	if !validIdentifier(string(p.Airport)) || !validIdentifier(string(p.Feeder)) || !validIdentifier(string(p.RunwayGroup)) || !p.Coverage.Valid() || strings.TrimSpace(p.Digest) == "" {
		return invalid("terminal path is incomplete")
	}
	if err := p.Provenance.Validate(); err != nil {
		return err
	}
	for _, leg := range p.Legs {
		if err := leg.Validate(); err != nil {
			return err
		}
	}
	if p.Coverage == CoverageComplete && (len(p.Unresolved) > 0 || hasUnsupportedLeg(p.Legs)) {
		return invalid("complete terminal path cannot retain unresolved or unsupported legs")
	}
	if p.Coverage == CoverageUnavailable && (len(p.Legs) > 0 || len(p.HoldingIDs) > 0 || len(p.Unresolved) > 0) {
		return invalid("unavailable terminal path cannot carry data")
	}
	return uniqueHoldings(p.HoldingIDs)
}

func RouteGeometryDigest(query RouteQuery, geometry RouteGeometry) (string, error) {
	if err := query.Validate(); err != nil {
		return "", err
	}
	if !query.Version.Equal(geometry.Version) {
		return "", &aman.DomainError{Class: ErrorDatasetMismatch, Message: "route query and geometry dataset versions differ"}
	}
	key, err := query.Key()
	if err != nil {
		return "", err
	}
	if err := geometry.validateCanonical(); err != nil {
		return "", err
	}
	legTokens := make([]string, 0, len(geometry.Legs))
	for _, leg := range geometry.Legs {
		course, distance := "", ""
		if leg.CourseTrueDeg != nil {
			course = strconv.FormatFloat(*leg.CourseTrueDeg, 'f', -1, 64)
		}
		if leg.DistanceNM != nil {
			distance = strconv.FormatFloat(*leg.DistanceNM, 'f', -1, 64)
		}
		legTokens = append(legTokens, strings.Join([]string{leg.ID, string(leg.PathTerminator), optionalFix(leg.FromFix), optionalFix(leg.ToFix), course, distance, optionalHolding(leg.HoldingID)}, "/"))
	}
	holdings := slices.Clone(geometry.HoldingIDs)
	slices.Sort(holdings)
	unresolved := slices.Clone(geometry.Unresolved)
	slices.Sort(unresolved)
	sum := sha256.Sum256([]byte(strings.Join(append([]string{string(key), strconv.FormatFloat(geometry.TotalDistanceNM, 'f', -1, 64), string(geometry.Coverage)}, append(legTokens, append(stringifyHoldings(holdings), unresolved...)...)...), "\x1f")))
	return hex.EncodeToString(sum[:]), nil
}

func validIdentifier(value string) bool {
	trimmed := strings.TrimSpace(value)
	if value != trimmed || trimmed == "" || trimmed != strings.ToUpper(trimmed) {
		return false
	}
	for _, r := range trimmed {
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
func optionalHolding(value *HoldingID) string {
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
func uniqueRunways(values []RunwayID) error {
	seen := map[RunwayID]struct{}{}
	for _, value := range values {
		if !validIdentifier(string(value)) {
			return invalid("runway identifier is invalid")
		}
		if _, found := seen[value]; found {
			return invalid("duplicate runway identifier")
		}
		seen[value] = struct{}{}
	}
	return nil
}
func uniqueProcedures(values []ProcedureID) error {
	seen := map[ProcedureID]struct{}{}
	for _, value := range values {
		if !validIdentifier(string(value)) {
			return invalid("procedure identifier is invalid")
		}
		if _, found := seen[value]; found {
			return invalid("duplicate procedure identifier")
		}
		seen[value] = struct{}{}
	}
	return nil
}
func uniqueHoldings(values []HoldingID) error {
	seen := map[HoldingID]struct{}{}
	for _, value := range values {
		if !validIdentifier(string(value)) {
			return invalid("holding identifier is invalid")
		}
		if _, found := seen[value]; found {
			return invalid("duplicate holding identifier")
		}
		seen[value] = struct{}{}
	}
	return nil
}
func hasUnsupportedLeg(legs []ProcedureLeg) bool {
	for _, leg := range legs {
		if !leg.PathTerminator.Supported() {
			return true
		}
	}
	return false
}
func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
