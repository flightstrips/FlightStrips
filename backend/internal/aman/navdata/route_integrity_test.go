package navdata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRouteIntegrityPreservesLegitimateOceanicAndZeroCoordinates(t *testing.T) {
	for _, positions := range [][]Coordinate{
		{{LatitudeDeg: 51, LongitudeDeg: -1}, {LatitudeDeg: 60, LongitudeDeg: -40}, {LatitudeDeg: 41, LongitudeDeg: -74}},
		{{LatitudeDeg: 40, LongitudeDeg: -74}, {LatitudeDeg: 21, LongitudeDeg: -158}, {LatitudeDeg: 34, LongitudeDeg: -118}},
		{{LatitudeDeg: 1, LongitudeDeg: -1}, {}, {LatitudeDeg: 1, LongitudeDeg: 1}},
	} {
		id := FixID("MID")
		legs := []ProcedureLeg{{ToFix: &id, FromPosition: &positions[0], ToPosition: &positions[1]}, {FromFix: &id, FromPosition: &positions[1], ToPosition: &positions[2]}}
		require.Empty(t, SuspectRouteDetours(legs))
		for _, position := range positions {
			require.NoError(t, position.Validate())
		}
	}
}
