package terminal

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadVendoredEKCHTMAVolume(t *testing.T) {
	volume, err := LoadTMAVolume(filepath.Join(ekchBoundaryDirectory(t), "EKCH.json"))
	require.NoError(t, err)

	tests := []struct {
		name                      string
		latitude, longitude, feet float64
		want                      bool
	}{
		{name: "Copenhagen airport", latitude: 55.6181, longitude: 12.6561, feet: 2_000, want: true},
		{name: "outside horizontally", latitude: 57, longitude: 12.6561, feet: 2_000},
		{name: "first vertex", latitude: 55.497222, longitude: 13.058611, feet: 2_000, want: true},
		{name: "edge midpoint", latitude: (55.497222 + 55.366944) / 2, longitude: (13.058611 + 13.026944) / 2, feet: 2_000, want: true},
		{name: "surface", latitude: 55.6181, longitude: 12.6561, feet: 0, want: true},
		{name: "just below FL195", latitude: 55.6181, longitude: 12.6561, feet: 19_499.999, want: true},
		{name: "at FL195", latitude: 55.6181, longitude: 12.6561, feet: 19_500},
		{name: "above FL195", latitude: 55.6181, longitude: 12.6561, feet: 20_000},
		{name: "below surface", latitude: 55.6181, longitude: 12.6561, feet: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, volume.Contains(test.latitude, test.longitude, test.feet))
		})
	}
}

func TestTMAVolumeSupportsHolesAndMultiplePolygons(t *testing.T) {
	volume := loadTestTMAVolume(t, `{
  "type":"Feature",
  "geometry":{"type":"MultiPolygon","coordinates":[
    [[[0,0],[10,0],[10,10],[0,10],[0,0]],[[3,3],[7,3],[7,7],[3,7],[3,3]]],
    [[[20,20],[22,20],[22,22],[20,22],[20,20]]]
  ]}}
`)

	tests := []struct {
		name                string
		latitude, longitude float64
		want                bool
	}{
		{name: "exterior interior", latitude: 2, longitude: 2, want: true},
		{name: "exterior edge", latitude: 5, longitude: 0, want: true},
		{name: "exterior vertex", latitude: 10, longitude: 10, want: true},
		{name: "hole interior", latitude: 5, longitude: 5},
		{name: "hole edge is polygon boundary", latitude: 3, longitude: 5, want: true},
		{name: "hole vertex is polygon boundary", latitude: 3, longitude: 3, want: true},
		{name: "second polygon", latitude: 21, longitude: 21, want: true},
		{name: "outside", latitude: 15, longitude: 15},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, volume.Contains(test.latitude, test.longitude, 1_000))
		})
	}
}

func TestLoadTMAVolumeRejectsInvalidGeoJSON(t *testing.T) {
	tests := []struct {
		name, document, wantError string
	}{
		{name: "invalid JSON", document: `{`, wantError: "decode TMA boundary"},
		{name: "not a feature", document: `{"type":"Polygon"}`, wantError: "type must be Feature"},
		{name: "not a MultiPolygon", document: `{"type":"Feature","geometry":{"type":"Polygon"}}`, wantError: "type must be MultiPolygon"},
		{name: "empty", document: `{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[]}}`, wantError: "must contain a polygon"},
		{name: "missing exterior", document: `{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":[[]]}}`, wantError: "must contain an exterior ring"},
		{name: "short ring", document: featureWithCoordinates(`[[[[0,0],[1,0],[0,0]]]]`), wantError: "at least four positions"},
		{name: "missing latitude", document: featureWithCoordinates(`[[[[0,0],[1,0],[1,1],[0]]]]`), wantError: "must contain longitude and latitude"},
		{name: "longitude out of range", document: featureWithCoordinates(`[[[[181,0],[1,0],[1,1],[181,0]]]]`), wantError: "longitude is invalid"},
		{name: "latitude out of range", document: featureWithCoordinates(`[[[[0,0],[1,0],[1,91],[0,0]]]]`), wantError: "latitude is invalid"},
		{name: "open ring", document: featureWithCoordinates(`[[[[0,0],[1,0],[1,1],[0,1]]]]`), wantError: "must be closed"},
		{name: "degenerate ring", document: featureWithCoordinates(`[[[[0,0],[1,0],[2,0],[0,0]]]]`), wantError: "must enclose an area"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "boundary.json")
			require.NoError(t, os.WriteFile(path, []byte(test.document), 0o600))
			_, err := LoadTMAVolume(path)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestTMAVolumeRejectsNonFiniteObservations(t *testing.T) {
	volume := loadTestTMAVolume(t, featureWithCoordinates(`[[[[0,0],[1,0],[1,1],[0,1],[0,0]]]]`))
	require.False(t, volume.Contains(0.5, 0.5, math.NaN()))
	require.False(t, volume.Contains(math.NaN(), 0.5, 1_000))
	require.False(t, volume.Contains(0.5, math.NaN(), 1_000))
	require.False(t, volume.Contains(math.Inf(1), 0.5, 1_000))
	require.False(t, volume.Contains(0.5, math.Inf(1), 1_000))
	require.False(t, volume.Contains(0.5, 0.5, math.Inf(1)))
}

func loadTestTMAVolume(t *testing.T, document string) TMAVolume {
	t.Helper()
	path := filepath.Join(t.TempDir(), "boundary.json")
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
	volume, err := LoadTMAVolume(path)
	require.NoError(t, err)
	return volume
}

func featureWithCoordinates(coordinates string) string {
	return `{"type":"Feature","geometry":{"type":"MultiPolygon","coordinates":` + coordinates + `}}`
}
