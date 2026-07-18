package fixture

import (
	"context"
	"testing"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/contracttest"
	"github.com/stretchr/testify/require"
)

func TestEKCHFixturePassesSharedSourceResolverContract(t *testing.T) {
	data := EKCH()
	source := New(data)
	contracttest.Run(t, source, contracttest.Suite{Version: data.Version, Airport: "EKCH", SIDQuery: navdata.ProcedureQuery{Version: data.Version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSID}}, STARQuery: navdata.ProcedureQuery{Version: data.Version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}}, ApproachQuery: navdata.ProcedureQuery{Version: data.Version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureApproach}}, ProcedureCoverage: map[navdata.ProcedureKind]navdata.Coverage{navdata.ProcedureApproach: navdata.CoveragePartial}, FixQuery: navdata.FixQuery{Version: data.Version, Identifiers: []navdata.FixID{"KEMAX", "SOK"}}, RouteQuery: EKCHRouteQuery(data.Version)})
}

func TestProcedureFiltersAndPublishedHoldsAreIndependent(t *testing.T) {
	data := EKCH()
	source := New(data)
	set, err := source.Procedures(context.Background(), navdata.ProcedureQuery{Version: data.Version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSTAR}, Runways: []navdata.RunwayID{"22L"}, Identifiers: []navdata.ProcedureID{"SOK1P"}})
	require.NoError(t, err)
	require.Len(t, set.Procedures, 1)
	require.Equal(t, navdata.PathHF, set.Procedures[0].Legs[0].PathTerminator)
	require.Equal(t, navdata.HoldingToFix, set.Procedures[0].Holdings[0].Termination)

	missing, err := source.Procedures(context.Background(), navdata.ProcedureQuery{Version: data.Version, Airport: "EKCH", Kinds: []navdata.ProcedureKind{navdata.ProcedureSID}, Identifiers: []navdata.ProcedureID{"UNKNOWN"}})
	require.NoError(t, err)
	require.Equal(t, navdata.CoveragePartial, missing.Coverage)
	require.Empty(t, missing.Procedures)

	for _, procedure := range data.Procedures {
		for _, holding := range procedure.Holdings {
			digest, err := navdata.HoldingDigest(holding)
			require.NoError(t, err)
			require.Equal(t, data.HoldingDigests[holding.ID], digest)
		}
	}
}

func TestRuntimeCacheMakesNoSourceCalls(t *testing.T) {
	data := EKCH()
	source := New(data)
	cache := source.Cache()
	before := source.Calls()
	query := EKCHRouteQuery(data.Version)
	key, err := query.Key()
	require.NoError(t, err)
	_, err = cache.ActiveVersion(context.Background(), "EKCH")
	require.NoError(t, err)
	_, err = cache.Route(context.Background(), key)
	require.NoError(t, err)
	_, err = cache.TerminalPath(context.Background(), "EKCH", "SOK", aman.RunwayGroupID("south"))
	require.NoError(t, err)
	require.Equal(t, before, source.Calls())
}

func EKCHRouteQuery(version navdata.DatasetVersion) navdata.RouteQuery {
	arrival := navdata.ProcedureID("SOK1P")
	runway := navdata.RunwayID("22L")
	group := aman.RunwayGroupID("south")
	return navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: "EKCH", FiledRoute: " dct   kemax ", ArrivalProcedure: &arrival, Runway: &runway, RunwayGroup: &group}
}
