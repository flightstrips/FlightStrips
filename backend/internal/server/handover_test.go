package server

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveClearedRouteTargetUsesCompleteDeparturePath(t *testing.T) {
	session := handoverSession()

	tests := []struct {
		name      string
		stand     string
		runway    string
		coverage  map[string]map[string]struct{}
		wantOwner string
		wantLabel string
		wantFreq  string
		wantRoute bool
		ownership routeOwnership
	}{
		{
			name:      "east route starts at GE",
			stand:     "G110",
			runway:    "22R",
			coverage:  handoverCoverage("121.830", "119.355"),
			wantOwner: "121.830",
			wantLabel: "GE",
			wantFreq:  "121.830",
			wantRoute: true,
			ownership: handoverOwnership(
				map[string]string{"GE": "121.830", "TW": "119.355"},
				map[string]string{"121.830": "GE", "119.355": "TW"},
			),
		},
		{
			name:      "east logical GE uses its configured GW owner when not carried",
			stand:     "G137",
			runway:    "22R",
			coverage:  handoverCoverage("118.580", "119.355"),
			wantOwner: "118.580",
			wantLabel: "GW",
			wantFreq:  "118.580",
			wantRoute: true,
			ownership: handoverOwnership(
				map[string]string{"GE": "118.580", "TW": "119.355"},
				map[string]string{"118.580": "GW", "119.355": "TW"},
			),
		},
		{
			name:   "west route resolves logical GW on carrying TW primary",
			stand:  "W1",
			runway: "22R",
			coverage: map[string]map[string]struct{}{
				"119.355": {"118.580": {}},
			},
			wantOwner: "119.355",
			wantLabel: "GW",
			wantFreq:  "118.580",
			wantRoute: true,
			ownership: handoverOwnership(
				map[string]string{"GWD": "119.355", "TW": "119.355"},
				map[string]string{"119.355": "TW"},
			),
		},
		{
			name:      "north route starts at separately primed sequence",
			stand:     "A12",
			runway:    "22R",
			coverage:  handoverCoverage("121.905", "121.730"),
			wantOwner: "121.905",
			wantLabel: "SEQ PLN",
			wantFreq:  "121.905",
			wantRoute: true,
			ownership: handoverOwnership(
				map[string]string{"SQ": "121.905", "AD": "121.730"},
				map[string]string{"121.905": "SQ", "121.730": "AD"},
			),
		},
		{
			name:   "unknown stand is inert",
			stand:  "Z999",
			runway: "22R",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			session.ActiveRunways.DepartureRunways = []string{test.runway}
			route, ok := config.ComputeDepartureRoute(
				session.ActiveRunways.GetAllActiveRunways(),
				test.stand,
				test.runway,
			)
			assert.Equal(t, test.wantRoute, ok)
			if !ok {
				return
			}

			strip := &models.Strip{
				Stand:  handoverStringPtr(test.stand),
				Runway: handoverStringPtr(test.runway),
				Origin: "EKCH",
			}
			got := resolveClearedRouteTarget(route.Path, strip, session, test.ownership, handoverRadio(test.coverage))
			require.NotNil(t, got)
			assert.Equal(t, test.wantOwner, got.Owner)
			require.NotNil(t, got.Display)
			assert.Equal(t, test.wantLabel, got.Display.Label)
			assert.Equal(t, test.wantFreq, got.Display.Frequency)
		})
	}
}

func TestEkchCompleteDeparturePaths(t *testing.T) {
	tests := []struct {
		stand  string
		runway string
		path   []string
	}{
		{"A12", "22R", []string{"SQ", "AD", "GW", "TW"}},
		{"H107", "22R", []string{"SQ", "AD", "GW", "TW"}},
		{"A12", "04L", []string{"SQ", "AD", "GW", "TW"}},
		{"A12", "04R", []string{"SQ", "AD", "GW", "TE"}},
		{"A12", "30", []string{"SQ", "AD", "GW", "TE"}},
		{"A12", "22L", []string{"SQ", "AD", "TE"}},
		{"A12", "12", []string{"SQ", "AD", "TW"}},
		{"G120", "22R", []string{"GE", "TW"}},
		{"G120", "04L", []string{"GE", "TW"}},
		{"G120", "04R", []string{"GE", "TE"}},
		{"G120", "30", []string{"GE", "TE"}},
		{"G120", "22L", []string{"GE", "TE"}},
		{"G120", "12", []string{"GE", "TW"}},
		{"W1", "22R", []string{"GW", "TW"}},
		{"W1", "04L", []string{"GW", "TW"}},
		{"W1", "04R", []string{"GW", "TE"}},
		{"W1", "30", []string{"GW", "TE"}},
		{"W1", "22L", []string{"GW", "TE"}},
		{"W1", "12", []string{"GW", "TW"}},
	}

	for _, test := range tests {
		t.Run(test.stand+" "+test.runway, func(t *testing.T) {
			route, ok := config.ComputeDepartureRoute([]string{test.runway}, test.stand, test.runway)
			require.True(t, ok)
			assert.Equal(t, test.path, route.Path)
		})
	}
}

func TestResolveClearedRouteTargetCombinedNorthPrimaryUsesADOnSequenceFrequency(t *testing.T) {
	session := handoverSession()
	strip := &models.Strip{
		Stand:  handoverStringPtr("B7"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	coverage := map[string]map[string]struct{}{
		"121.905": {
			"121.730": {},
			"121.905": {},
		},
	}

	route, ok := config.ComputeDepartureRoute(
		session.ActiveRunways.GetAllActiveRunways(),
		"B7",
		"22R",
	)
	require.True(t, ok)

	ownership := handoverOwnership(
		map[string]string{"SQ": "121.905", "AD": "121.905"},
		map[string]string{"121.905": "AD"},
	)
	got := resolveClearedRouteTarget(route.Path, strip, session, ownership, handoverRadio(coverage))

	require.NotNil(t, got)
	assert.Equal(t, "121.905", got.Owner)
	require.NotNil(t, got.Display)
	assert.Equal(t, "AD", got.Display.Label)
	assert.Equal(t, "121.905", got.Display.Frequency)
}

func TestResolveOwnedHandoverTargetUsesConfiguredOwnerWhenLogicalFrequencyIsNotCarried(t *testing.T) {
	session := handoverSession()
	strip := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	ownership := routeOwnership{
		sectorToOwner:   map[string]string{"GWD": "121.830", "GWA": "118.105"},
		ownerIdentifier: map[string]string{"121.830": "GE"},
	}

	got := resolveOwnedHandoverTarget(
		"GW",
		strip,
		session,
		ownership,
		handoverRadio(handoverCoverage("121.830")),
	)

	require.NotNil(t, got)
	assert.Equal(t, "121.830", got.Owner)
	require.NotNil(t, got.Display)
	assert.Equal(t, "GE", got.Display.Label)
	assert.Equal(t, "121.830", got.Display.Frequency)
}

func TestResolveOwnedHandoverTargetSelectsDirectionalGWOwner(t *testing.T) {
	session := handoverSession()
	ownership := handoverOwnership(
		map[string]string{"GWD": "119.355", "GWA": "118.105"},
		map[string]string{"119.355": "TW", "118.105": "TE"},
	)
	radio := handoverRadio(handoverCoverage("119.355", "118.105"))

	departure := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	departureTarget := resolveOwnedHandoverTarget("GW", departure, session, ownership, radio)
	require.NotNil(t, departureTarget)
	assert.Equal(t, "119.355", departureTarget.Owner)

	arrival := &models.Strip{
		Stand:       handoverStringPtr("A12"),
		Runway:      handoverStringPtr("22L"),
		Destination: "EKCH",
	}
	arrivalTarget := resolveOwnedHandoverTarget("GW", arrival, session, ownership, radio)
	require.NotNil(t, arrivalTarget)
	assert.Equal(t, "118.105", arrivalTarget.Owner)
}

func TestResolveOwnedHandoverTargetDoesNotBypassConfiguredOwnerForAnotherCarrier(t *testing.T) {
	session := handoverSession()
	strip := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	ownership := handoverOwnership(
		map[string]string{"GWD": "121.830"},
		map[string]string{"121.830": "GE"},
	)
	radio := routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.830": {"121.830": {}},
			"119.355": {"118.580": {}},
		},
		roleByPrimary: map[string]string{},
	}

	got := resolveOwnedHandoverTarget("GW", strip, session, ownership, radio)

	require.NotNil(t, got)
	assert.Equal(t, "121.830", got.Owner)
	require.NotNil(t, got.Display)
	assert.Equal(t, "GE", got.Display.Label)
	assert.Equal(t, "121.830", got.Display.Frequency)
}

func TestResolveOwnedHandoverTargetUsesCallsignRoleForFallbackDisplay(t *testing.T) {
	session := handoverSession()
	strip := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	ownership := handoverOwnership(
		map[string]string{"GWD": "121.830"},
		map[string]string{"121.830": "TE"},
	)
	radio := routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.830": {"121.830": {}},
		},
		roleByPrimary: map[string]string{"121.830": "EKCH_GE_TWR"},
	}

	got := resolveOwnedHandoverTarget("GW", strip, session, ownership, radio)

	require.NotNil(t, got)
	assert.Equal(t, "121.830", got.Owner)
	require.NotNil(t, got.Display)
	assert.Equal(t, "GE", got.Display.Label)
	assert.Equal(t, "121.830", got.Display.Frequency)
}

func TestResolveOwnedHandoverTargetDoesNotDisplayUnsensedFallbackFrequency(t *testing.T) {
	session := handoverSession()
	session.ActiveRunways.DepartureRunways = []string{"30"}
	strip := &models.Strip{
		Stand:  handoverStringPtr("W1"),
		Runway: handoverStringPtr("30"),
		Origin: "EKCH",
	}
	ownership := handoverOwnership(
		map[string]string{"GWD": "119.355"},
		map[string]string{"119.355": "GW"},
	)
	radio := routeRadioState{
		coverage: map[string]map[string]struct{}{
			"119.355": {},
		},
		roleByPrimary: map[string]string{"119.355": "EKCH_D_TWR"},
	}

	got := resolveOwnedHandoverTarget("GW", strip, session, ownership, radio)

	require.NotNil(t, got)
	assert.Equal(t, "119.355", got.Owner)
	assert.Nil(t, got.Display)
}

func TestComputeRouteStateUsesLogicalPathAndRadioCarrier(t *testing.T) {
	session := handoverSession()
	owner := "121.730"

	tests := []struct {
		name      string
		coverage  map[string]map[string]struct{}
		wantOwner string
		wantLabel string
		wantFreq  string
		owners    []*models.SectorOwner
	}{
		{
			name: "TW carries logical GW",
			coverage: map[string]map[string]struct{}{
				"121.730": {},
				"119.355": {"118.580": {}},
			},
			wantOwner: "119.355",
			wantLabel: "GW",
			wantFreq:  "118.580",
			owners: []*models.SectorOwner{
				{Position: "121.730", Sector: []string{"AD"}, Identifier: "AD"},
				{Position: "119.355", Sector: []string{"GWD", "TW"}, Identifier: "TW"},
			},
		},
		{
			name: "GE carries logical GW",
			coverage: map[string]map[string]struct{}{
				"121.730": {},
				"121.830": {"118.580": {}},
				"119.355": {},
			},
			wantOwner: "121.830",
			wantLabel: "GW",
			wantFreq:  "118.580",
			owners: []*models.SectorOwner{
				{Position: "121.730", Sector: []string{"AD"}, Identifier: "AD"},
				{Position: "121.830", Sector: []string{"GWD"}, Identifier: "GE"},
				{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
			},
		},
		{
			name:      "GE owns logical GW when GW is not carried",
			coverage:  handoverCoverage("121.730", "121.830", "119.355"),
			wantOwner: "121.830",
			wantLabel: "GE",
			wantFreq:  "121.830",
			owners: []*models.SectorOwner{
				{Position: "121.730", Sector: []string{"AD"}, Identifier: "AD"},
				{Position: "121.830", Sector: []string{"GWD"}, Identifier: "GE"},
				{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			strip := &models.Strip{
				Bay:    "CLEARED",
				Stand:  handoverStringPtr("A12"),
				Runway: handoverStringPtr("22R"),
				Origin: "EKCH",
				Owner:  &owner,
			}

			state, shouldUpdate, err := computeRouteStateForStrip(strip, session, test.owners, handoverRadio(test.coverage))

			require.NoError(t, err)
			require.True(t, shouldUpdate)
			require.NotEmpty(t, state.NextOwners)
			assert.Equal(t, test.wantOwner, state.NextOwners[0])
			require.NotNil(t, state.NextDisplay)
			assert.Equal(t, test.wantLabel, state.NextDisplay.Label)
			assert.Equal(t, test.wantFreq, state.NextDisplay.Frequency)
		})
	}
}

func TestComputeRouteStateUsesHistoryForRepeatedPhysicalOwner(t *testing.T) {
	session := handoverSession()
	currentOwner := "121.830"
	strip := &models.Strip{
		Stand:          handoverStringPtr("A12"),
		Runway:         handoverStringPtr("22R"),
		Origin:         "EKCH",
		Owner:          &currentOwner,
		PreviousOwners: []string{"121.830", "121.730"},
	}
	owners := []*models.SectorOwner{
		{Position: "121.830", Sector: []string{"SQ", "GWD"}, Identifier: "GE"},
		{Position: "121.730", Sector: []string{"AD"}, Identifier: "AD"},
		{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
	}
	radio := routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.830": {"121.905": {}},
			"121.730": {},
			"119.355": {},
		},
		roleByPrimary: map[string]string{},
	}

	state, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, radio)

	require.NoError(t, err)
	require.True(t, shouldUpdate)
	assert.Equal(t, []string{"119.355"}, state.NextOwners)
	require.NotNil(t, state.NextDisplay)
	assert.Equal(t, "TW", state.NextDisplay.Label)
}

func TestComputeRouteStateDisplaysADOnSharedNorthPrimary(t *testing.T) {
	session := handoverSession()
	strip := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
	}
	owners := []*models.SectorOwner{
		{Position: "121.905", Sector: []string{"SQ", "AD"}, Identifier: "AD"},
		{Position: "118.580", Sector: []string{"GWD"}, Identifier: "GW"},
		{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
	}
	radio := routeRadioState{
		coverage: map[string]map[string]struct{}{
			"121.905": {"121.730": {}, "121.905": {}},
			"118.580": {},
			"119.355": {},
		},
		roleByPrimary: map[string]string{},
	}

	state, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, radio)

	require.NoError(t, err)
	require.True(t, shouldUpdate)
	require.NotNil(t, state.NextDisplay)
	assert.Equal(t, "AD", state.NextDisplay.Label)
	assert.Equal(t, "121.905", state.NextDisplay.Frequency)
}

func TestComputeRouteStateCollapsesLogicalStagesCarriedByCurrentPrimary(t *testing.T) {
	session := handoverSession()
	owner := "121.730"
	strip := &models.Strip{
		Stand:  handoverStringPtr("A12"),
		Runway: handoverStringPtr("22R"),
		Origin: "EKCH",
		Owner:  &owner,
	}
	coverage := map[string]map[string]struct{}{
		"121.730": {
			"121.730": {},
			"121.905": {},
			"118.580": {},
		},
		"119.355": {},
	}

	owners := []*models.SectorOwner{
		{Position: "121.730", Sector: []string{"SQ", "AD", "GWD"}, Identifier: "AD"},
		{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
	}
	state, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, handoverRadio(coverage))

	require.NoError(t, err)
	require.True(t, shouldUpdate)
	assert.Equal(t, []string{"119.355"}, state.NextOwners)
	require.NotNil(t, state.NextDisplay)
	assert.Equal(t, "TW", state.NextDisplay.Label)
}

func TestCompleteRouteDoesNotDependOnBay(t *testing.T) {
	session := handoverSession()
	owner := "121.730"
	coverage := handoverCoverage("121.730", "118.580", "119.355")

	var expected computedRouteState
	for index, bay := range []string{"CLEARED", "TAXI_LWR", "TWY_ARR", ""} {
		strip := &models.Strip{
			Bay:    bay,
			Stand:  handoverStringPtr("A12"),
			Runway: handoverStringPtr("22R"),
			Origin: "EKCH",
			Owner:  &owner,
		}
		owners := []*models.SectorOwner{
			{Position: "121.730", Sector: []string{"SQ", "AD"}, Identifier: "AD"},
			{Position: "118.580", Sector: []string{"GWD"}, Identifier: "GW"},
			{Position: "119.355", Sector: []string{"TW"}, Identifier: "TW"},
		}
		state, shouldUpdate, err := computeRouteStateForStrip(strip, session, owners, handoverRadio(coverage))
		require.NoError(t, err)
		require.True(t, shouldUpdate)
		if index == 0 {
			expected = state
			continue
		}
		assert.Equal(t, expected, state)
	}
}

func handoverSession() *models.Session {
	return &models.Session{
		Airport: "EKCH",
		ActiveRunways: pkgModels.ActiveRunways{
			ArrivalRunways:   []string{"22L"},
			DepartureRunways: []string{"22R"},
		},
	}
}

func handoverCoverage(frequencies ...string) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{}, len(frequencies))
	for _, frequency := range frequencies {
		result[frequency] = map[string]struct{}{frequency: {}}
	}
	return result
}

func handoverRadio(coverage map[string]map[string]struct{}) routeRadioState {
	return routeRadioState{
		coverage:      coverage,
		roleByPrimary: map[string]string{},
	}
}

func handoverOwnership(sectors map[string]string, identifiers map[string]string) routeOwnership {
	return routeOwnership{
		sectorToOwner:   sectors,
		ownerIdentifier: identifiers,
	}
}

func handoverStringPtr(value string) *string {
	return &value
}
