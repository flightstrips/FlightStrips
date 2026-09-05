package server

import (
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRouteInputFingerprintIgnoresOwnerAndCoverageOrdering(t *testing.T) {
	session := &models.Session{ActiveRunways: pkgModels.ActiveRunways{
		DepartureRunways: []string{"22R"},
		ArrivalRunways:   []string{"22L"},
	}}

	left, err := buildRouteInputFingerprint(session, []*models.SectorOwner{
		{Position: "121.900", Identifier: "AP", Sector: []string{"AP", "AN"}},
		{Position: "118.100", Identifier: "TW", Sector: []string{"TW"}},
	}, routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.900": {"121.830": {}, "121.900": {}},
			"118.100": {},
		},
		roleByPrimary: map[string]string{
			"121.900": "EKCH_APRON",
			"118.100": "EKCH_TOWER",
		},
	})
	require.NoError(t, err)

	right, err := buildRouteInputFingerprint(session, []*models.SectorOwner{
		{Position: "118.100", Identifier: "TW", Sector: []string{"TW"}},
		{Position: "121.900", Identifier: "AP", Sector: []string{"AN", "AP"}},
	}, routeRadioState{
		coverage: map[string]map[string]struct{}{
			"118.100": {},
			"121.900": {"121.900": {}, "121.830": {}},
		},
		roleByPrimary: map[string]string{
			"118.100": "EKCH_TOWER",
			"121.900": "EKCH_APRON",
		},
	})
	require.NoError(t, err)

	assert.Equal(t, left, right)
}

func TestBuildRouteInputFingerprintChangesWithEffectiveRadioCoverage(t *testing.T) {
	session := &models.Session{ActiveRunways: pkgModels.ActiveRunways{
		DepartureRunways: []string{"22R"},
		ArrivalRunways:   []string{"22L"},
	}}
	owners := []*models.SectorOwner{
		{Position: "121.900", Identifier: "AP", Sector: []string{"AP"}},
	}

	before, err := buildRouteInputFingerprint(session, owners, routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.900": {"121.900": {}},
		},
		roleByPrimary: map[string]string{"121.900": "EKCH_APRON"},
	})
	require.NoError(t, err)

	after, err := buildRouteInputFingerprint(session, owners, routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.900": {"121.900": {}, "121.830": {}},
		},
		roleByPrimary: map[string]string{"121.900": "EKCH_APRON"},
	})
	require.NoError(t, err)

	assert.NotEqual(t, before, after)
}
