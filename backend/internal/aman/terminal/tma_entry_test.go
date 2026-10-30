package terminal

import (
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestEntryFractionIncludesCrossingLegsAndAltitudeBoundary(t *testing.T) {
	volume := TMAVolume{polygons: []tmaPolygon{{{{longitude: 0, latitude: 0}, {longitude: 2, latitude: 0}, {longitude: 2, latitude: 2}, {longitude: 0, latitude: 2}, {longitude: 0, latitude: 0}}}}}
	fraction, ok := volume.EntryFraction(1, -1, 10000, 1, 3, 10000)
	require.True(t, ok)
	require.InDelta(t, .25, fraction, 1e-12, "outside-to-outside legs still enter the volume")
	fraction, ok = volume.EntryFraction(1, 1, 20000, 1, 1, 18000)
	require.True(t, ok)
	require.Greater(t, fraction, .25, "FL195 itself is outside")
	_, ok = volume.EntryFraction(1, 1, 19500, 1, 1, 19500)
	require.False(t, ok)
	_, ok = volume.EntryFraction(math.NaN(), 1, 10000, 1, 1, 10000)
	require.False(t, ok)
}
