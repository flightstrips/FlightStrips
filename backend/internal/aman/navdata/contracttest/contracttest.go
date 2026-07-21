// Package contracttest contains reusable source/resolver conformance checks.
// The fixture source runs them now; #310 runs these same checks for AIRAC.NET.
package contracttest

import (
	"context"
	"testing"

	"FlightStrips/internal/aman/navdata"
)

type Provider interface {
	navdata.CycleSource
	navdata.AirportSource
	navdata.ProcedureSource
	navdata.FixSource
	navdata.RouteResolver
}

type Suite struct {
	Version           navdata.DatasetVersion
	Airport           navdata.AirportID
	SIDQuery          navdata.ProcedureQuery
	STARQuery         navdata.ProcedureQuery
	ApproachQuery     navdata.ProcedureQuery
	ProcedureCoverage map[navdata.ProcedureKind]navdata.Coverage
	FixQuery          navdata.FixQuery
	RouteQuery        navdata.RouteQuery
	RouteDigest       string
	HoldingDigests    map[navdata.HoldingID]string
}

func Run(t testing.TB, provider Provider, suite Suite) {
	t.Helper()
	ctx := context.Background()
	version, err := provider.LatestVersion(ctx)
	if err != nil {
		t.Fatalf("latest dataset version: %v", err)
	}
	if !version.Equal(suite.Version) {
		t.Fatalf("dataset version = %#v, want %#v", version, suite.Version)
	}
	airport, err := provider.Airport(ctx, suite.Version, suite.Airport)
	if err != nil || airport.ID != suite.Airport {
		t.Fatalf("airport = %#v, %v", airport, err)
	}
	for _, query := range []navdata.ProcedureQuery{suite.SIDQuery, suite.STARQuery, suite.ApproachQuery} {
		procedures, err := provider.Procedures(ctx, query)
		if err != nil {
			t.Fatalf("procedures for %v: %v", query.Kinds, err)
		}
		expectedCoverage := navdata.CoverageComplete
		if len(query.Kinds) == 1 && suite.ProcedureCoverage != nil {
			if expected, ok := suite.ProcedureCoverage[query.Kinds[0]]; ok {
				expectedCoverage = expected
			}
		}
		if procedures.Coverage != expectedCoverage || len(procedures.Procedures) == 0 {
			t.Fatalf("procedures for %v have coverage %q and %d values", query.Kinds, procedures.Coverage, len(procedures.Procedures))
		}
		for _, procedure := range procedures.Procedures {
			if err := procedure.Validate(); err != nil {
				t.Fatalf("procedure %q invalid: %v", procedure.ID, err)
			}
			for _, holding := range procedure.Holdings {
				digest, err := navdata.HoldingDigest(holding)
				if err != nil {
					t.Fatalf("holding %q digest: %v", holding.ID, err)
				}
				if expected, ok := suite.HoldingDigests[holding.ID]; !ok || digest != expected {
					t.Fatalf("holding %q digest = %q, want %q", holding.ID, digest, expected)
				}
			}
		}
	}
	fixes, err := provider.Fixes(ctx, suite.FixQuery)
	if err != nil || !fixes.Coverage.Authoritative() {
		t.Fatalf("fixes = %#v, %v", fixes, err)
	}
	route, err := provider.Resolve(ctx, suite.RouteQuery)
	if err != nil {
		t.Fatalf("resolve DCT route: %v", err)
	}
	if route.Coverage != navdata.CoveragePartial || len(route.Unresolved) == 0 {
		t.Fatalf("route must retain unsupported geometry explicitly: %#v", route)
	}
	digest, err := navdata.RouteGeometryDigest(suite.RouteQuery, route)
	if err != nil {
		t.Fatalf("route digest: %v", err)
	}
	if route.Digest != suite.RouteDigest || digest != suite.RouteDigest {
		t.Fatalf("route digest = stored %q / computed %q, want %q", route.Digest, digest, suite.RouteDigest)
	}
}
