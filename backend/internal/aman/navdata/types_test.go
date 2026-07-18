package navdata

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestRouteKeyNormalizesFormattingButPreservesDCTSemantics(t *testing.T) {
	version := testVersion()
	one := RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX"}
	two := one
	two.FiledRoute = " dct   kemax "
	first, err := one.Key()
	require.NoError(t, err)
	second, err := two.Key()
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestHoldingAndLegValidationRejectsUnsafeDefinitions(t *testing.T) {
	provenance := testProvenance()
	timeSeconds, distance := int64(60), 2.0
	holding := HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: 360, TurnDirection: TurnRight, LegTimeSeconds: &timeSeconds, Termination: HoldingManual, Provenance: provenance}
	assertInvalid(t, holding.Validate())
	holding.InboundCourseTrueDeg = 180
	holding.LegLengthNM = &distance
	assertInvalid(t, holding.Validate())
	leg := ProcedureLeg{ID: "H", PathTerminator: PathHA}
	assertInvalid(t, leg.Validate())
	for _, terminator := range []PathTerminator{PathHA, PathHF, PathHM} {
		leg = ProcedureLeg{ID: string(terminator), PathTerminator: terminator}
		assertInvalid(t, leg.Validate())
	}
	holding = HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: 180, TurnDirection: "wrong", LegTimeSeconds: &timeSeconds, Termination: "wrong", Provenance: provenance}
	assertInvalid(t, holding.Validate())
	minimum, maximum, speed := 5000, 4000, 0
	holding = HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: 180, TurnDirection: TurnRight, LegTimeSeconds: &timeSeconds, MinimumAltitudeFt: &minimum, MaximumAltitudeFt: &maximum, MaximumSpeedKt: &speed, Termination: HoldingManual, Provenance: provenance}
	assertInvalid(t, holding.Validate())
}

func TestCanonicalModelsContainNoHTTPOrVendorFields(t *testing.T) {
	for _, model := range []any{DatasetVersion{}, Provenance{}, Airport{}, Runway{}, Fix{}, Airway{}, Procedure{}, RouteGeometry{}, TerminalPath{}} {
		modelType := reflect.TypeOf(model)
		for index := range modelType.NumField() {
			field := strings.ToLower(modelType.Field(index).Name)
			if strings.Contains(field, "http") || strings.Contains(field, "etag") || strings.Contains(field, "url") || strings.Contains(field, "cursor") || strings.Contains(field, "retry") {
				t.Errorf("%s.%s leaks provider transport state", modelType.Name(), modelType.Field(index).Name)
			}
		}
	}
}

func TestProcedureRequiresExactlyOneReferencedHolding(t *testing.T) {
	provenance := testProvenance()
	timeSeconds := int64(60)
	holding := HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: 180, TurnDirection: TurnRight, LegTimeSeconds: &timeSeconds, Termination: HoldingManual, Provenance: provenance}
	missing := HoldingID("MISSING")
	procedure := Procedure{ID: "SOK1P", Airport: "EKCH", Kind: ProcedureSTAR, Provenance: provenance, Holdings: []HoldingPattern{holding}, Legs: []ProcedureLeg{{ID: "H", PathTerminator: PathHF, HoldingID: &missing}}}
	assertInvalid(t, procedure.Validate())
}

func TestProcedureHoldingTerminatorAndFixMustMatchDefinition(t *testing.T) {
	provenance := testProvenance()
	seconds := int64(60)
	holdingID, holdingFix := HoldingID("HOLD"), FixID("KEMAX")
	holding := HoldingPattern{ID: holdingID, Fix: holdingFix, InboundCourseTrueDeg: 180, TurnDirection: TurnRight, LegTimeSeconds: &seconds, Termination: HoldingToFix, Provenance: provenance}
	procedure := Procedure{ID: "SOK1P", Airport: "EKCH", Kind: ProcedureSTAR, Provenance: provenance, Holdings: []HoldingPattern{holding}, Legs: []ProcedureLeg{{ID: "H", PathTerminator: PathHA, HoldingID: &holdingID}}}
	assertInvalid(t, procedure.Validate())
	procedure.Legs[0].PathTerminator = PathHF
	wrongFix := FixID("ROSBI")
	procedure.Legs[0].ToFix = &wrongFix
	assertInvalid(t, procedure.Validate())
}

func TestCompleteCoverageRejectsUnsupportedAndUnresolvedGeometry(t *testing.T) {
	version, provenance := testVersion(), testProvenance()
	unsupported := Procedure{ID: "ILS22L", Airport: "EKCH", Kind: ProcedureApproach, Provenance: provenance, Legs: []ProcedureLeg{{ID: "V", PathTerminator: PathUnsupported}}}
	assertInvalid(t, ProcedureSet{Version: version, Airport: "EKCH", Procedures: []Procedure{unsupported}, Coverage: CoverageComplete, Provenance: provenance}.Validate())
	geometry := RouteGeometry{Version: version, Legs: []ProcedureLeg{{ID: "V", PathTerminator: PathUnsupported}}, Coverage: CoverageComplete, Provenance: provenance, Digest: "digest"}
	assertInvalid(t, geometry.Validate())
	path := TerminalPath{Version: version, Airport: "EKCH", Feeder: "SOK", RunwayGroup: "SOUTH", Legs: []ProcedureLeg{{ID: "V", PathTerminator: PathUnsupported}}, Coverage: CoverageComplete, Provenance: provenance, Digest: "digest"}
	assertInvalid(t, path.Validate())
}

func TestQueriesRejectNonCanonicalOptionalIdentifiers(t *testing.T) {
	version := testVersion()
	procedure := ProcedureID("SOK1P")
	runway := RunwayID("22L")
	group := aman.RunwayGroupID("south")
	assertInvalid(t, ProcedureQuery{Version: version, Airport: " EKCH", Kinds: []ProcedureKind{ProcedureSID}}.Validate())
	assertInvalid(t, FixQuery{Version: version, Identifiers: []FixID{"KEMAX", "KEMAX"}}.Validate())
	assertInvalid(t, RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT", ArrivalProcedure: &procedure, Runway: &runway, RunwayGroup: &group}.Validate())
}

func TestCanonicalFloatFieldsRejectNaNAndInfinity(t *testing.T) {
	assertInvalid(t, Coordinate{LatitudeDeg: math.NaN(), LongitudeDeg: 12}.Validate())
	provenance, seconds := testProvenance(), int64(60)
	assertInvalid(t, HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: math.Inf(1), TurnDirection: TurnRight, LegTimeSeconds: &seconds, Termination: HoldingManual, Provenance: provenance}.Validate())
	distance := math.Inf(1)
	assertInvalid(t, ProcedureLeg{ID: "LEG", PathTerminator: PathDF, DistanceNM: &distance}.Validate())
	assertInvalid(t, ProcedureLeg{ID: "leg/with delimiter", PathTerminator: PathDF}.Validate())
	assertInvalid(t, RouteGeometry{Version: testVersion(), TotalDistanceNM: math.NaN(), Coverage: CoveragePartial, Provenance: provenance, Digest: "digest"}.Validate())
}

func TestRouteDigestRejectsDatasetMismatch(t *testing.T) {
	query := RouteQuery{Version: testVersion(), Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX"}
	geometry := RouteGeometry{Version: query.Version, Coverage: CoveragePartial, Provenance: testProvenance()}
	geometry.Version.SourceRevision = "other"
	_, err := RouteGeometryDigest(query, geometry)
	var domain *aman.DomainError
	require.Error(t, err)
	require.True(t, errors.As(err, &domain))
	require.Equal(t, ErrorDatasetMismatch, domain.Class)
}

func TestRouteDigestIsDeterministicForHoldingAndUnresolvedOrdering(t *testing.T) {
	query := RouteQuery{Version: testVersion(), Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX"}
	course, distance := 180.0, 12.5
	geometry := RouteGeometry{Version: query.Version, Legs: []ProcedureLeg{{ID: "DCT", PathTerminator: PathDF, CourseTrueDeg: &course, DistanceNM: &distance}}, HoldingIDs: []HoldingID{"B", "A"}, TotalDistanceNM: distance, Coverage: CoveragePartial, Unresolved: []string{"vector", "missing"}, Provenance: testProvenance()}
	first, err := RouteGeometryDigest(query, geometry)
	require.NoError(t, err)
	geometry.HoldingIDs = []HoldingID{"A", "B"}
	geometry.Unresolved = []string{"missing", "vector"}
	second, err := RouteGeometryDigest(query, geometry)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestRouteDigestIncludesEveryMaterialGeometryField(t *testing.T) {
	query := RouteQuery{Version: testVersion(), Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX"}
	course, distance := 180.0, 12.5
	holding := HoldingID("HOLD")
	from, to := FixID("KEMAX"), FixID("KEMAX")
	base := RouteGeometry{Version: query.Version, Legs: []ProcedureLeg{{ID: "DCT", PathTerminator: PathHF, FromFix: &from, ToFix: &to, CourseTrueDeg: &course, DistanceNM: &distance, HoldingID: &holding}}, HoldingIDs: []HoldingID{holding}, TotalDistanceNM: distance, Coverage: CoveragePartial, Unresolved: []string{"vector"}, Provenance: testProvenance()}
	baseline, err := RouteGeometryDigest(query, base)
	require.NoError(t, err)
	changes := []func(*RouteGeometry){func(g *RouteGeometry) { value := 181.0; g.Legs[0].CourseTrueDeg = &value }, func(g *RouteGeometry) { value := 13.0; g.Legs[0].DistanceNM = &value }, func(g *RouteGeometry) { value := HoldingID("OTHER"); g.Legs[0].HoldingID = &value }, func(g *RouteGeometry) { value := FixID("ROSBI"); g.Legs[0].FromFix = &value }, func(g *RouteGeometry) { value := FixID("ROSBI"); g.Legs[0].ToFix = &value }, func(g *RouteGeometry) { g.Legs[0].PathTerminator = PathHA }, func(g *RouteGeometry) { g.TotalDistanceNM = 13 }, func(g *RouteGeometry) { g.HoldingIDs = []HoldingID{"OTHER"} }, func(g *RouteGeometry) { g.Coverage = CoverageUnsupported }, func(g *RouteGeometry) { g.Unresolved = []string{"other"} }, func(g *RouteGeometry) { g.Legs[0].ID = "OTHER" }}
	for _, change := range changes {
		candidate := cloneRoute(base)
		change(&candidate)
		digest, err := RouteGeometryDigest(query, candidate)
		require.NoError(t, err)
		require.NotEqual(t, baseline, digest)
	}
}

func TestHoldingDigestExcludesProvenanceButIncludesGeometry(t *testing.T) {
	seconds := int64(60)
	base := HoldingPattern{ID: "HOLD", Fix: "KEMAX", InboundCourseTrueDeg: 180, TurnDirection: TurnRight, LegTimeSeconds: &seconds, Termination: HoldingManual, Provenance: testProvenance()}
	first, err := HoldingDigest(base)
	require.NoError(t, err)
	other := base
	other.Provenance = Provenance{SourceID: "airacnet", SourceRevision: "different", ImportedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), EffectiveFrom: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), EffectiveUntil: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	second, err := HoldingDigest(other)
	require.NoError(t, err)
	require.Equal(t, first, second)
	other.InboundCourseTrueDeg = 181
	changed, err := HoldingDigest(other)
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
}

func testVersion() DatasetVersion {
	from := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	return DatasetVersion{Cycle: "2608", SourceRevision: "r1", EffectiveFrom: from, EffectiveUntil: from.AddDate(0, 0, 28)}
}
func testProvenance() Provenance {
	version := testVersion()
	return Provenance{SourceID: "fixture", SourceRevision: "r1", ImportedAt: version.EffectiveFrom, EffectiveFrom: version.EffectiveFrom, EffectiveUntil: version.EffectiveUntil}
}
func assertInvalid(t *testing.T, err error) {
	t.Helper()
	var domain *aman.DomainError
	require.Error(t, err)
	require.True(t, errors.As(err, &domain))
	require.Equal(t, aman.ErrorInvalidArgument, domain.Class)
}
