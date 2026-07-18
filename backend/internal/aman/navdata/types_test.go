package navdata

import (
	"errors"
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

func TestRouteDigestIsDeterministicForHoldingAndUnresolvedOrdering(t *testing.T) {
	query := RouteQuery{Version: testVersion(), Origin: "ENGM", Destination: "EKCH", FiledRoute: "DCT KEMAX"}
	legs := []ProcedureLeg{{ID: "DCT", PathTerminator: PathDF}}
	first, err := RouteGeometryDigest(query, legs, []HoldingID{"B", "A"}, CoveragePartial, []string{"vector", "missing"})
	require.NoError(t, err)
	second, err := RouteGeometryDigest(query, legs, []HoldingID{"A", "B"}, CoveragePartial, []string{"missing", "vector"})
	require.NoError(t, err)
	require.Equal(t, first, second)
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
