package sat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testICAOAircraftJSON = `[
{"ICAO":"A20N","Description":"L2J","WTC":"M","IATA":"32N"},
{"ICAO":"C172","Description":"L1P","WTC":"L"}
]`

func testAircraftRegistry(t *testing.T) *AircraftRegistry {
	t.Helper()
	registry, err := LoadAircraftReference(strings.NewReader("A20N\t35.8\t37.57\t11.76\t79000\tA\t32N\nC172\t11.2\t8.3\t2.7\t1100\tP\n"))
	require.NoError(t, err)
	return registry
}

func TestLoadAircraftEngineReferenceReadsICAOJSONAndAliases(t *testing.T) {
	aircraft := testAircraftRegistry(t)
	registry, err := LoadAircraftEngineReference(strings.NewReader(testICAOAircraftJSON), aircraft)
	require.NoError(t, err)

	engine, ok := registry.Lookup("32n")
	require.True(t, ok)
	assert.Equal(t, EngineJet, engine)
	wtc, ok := registry.LookupWTC("C172")
	require.True(t, ok)
	assert.Equal(t, "L", wtc)
}

func TestLoadAircraftEngineReferenceValidatesICAOJSON(t *testing.T) {
	aircraft := testAircraftRegistry(t)
	for _, test := range []struct {
		name string
		data string
		want string
	}{
		{name: "invalid engine", data: `[{"ICAO":"A20N","Description":"L2Q","WTC":"M"}]`, want: "invalid engine code"},
		{name: "invalid WTC", data: `[{"ICAO":"A20N","Description":"L2J","WTC":"X"}]`, want: "invalid WTC"},
		{name: "malformed JSON", data: `[{"ICAO":"A20N"`, want: "decode ICAO aircraft JSON"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadAircraftEngineReference(strings.NewReader(test.data), aircraft)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestLoadAircraftEngineReferenceFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "ICAO_Aircraft.json")
	require.NoError(t, os.WriteFile(file, []byte(testICAOAircraftJSON), 0o600))
	registry, err := LoadAircraftEngineReferenceFile(file, testAircraftRegistry(t))
	require.NoError(t, err)
	engine, ok := registry.Lookup("32N")
	assert.True(t, ok)
	assert.Equal(t, EngineJet, engine)
}

func TestResolveFlightCompatibilityFactsPrefersLiveEngineAndUsesCorrectBorderEndpoint(t *testing.T) {
	aircraft := testAircraftRegistry(t)
	engines, err := LoadAircraftEngineReference(strings.NewReader(testICAOAircraftJSON), aircraft)
	require.NoError(t, err)
	borders := NewAirportCountryRegistry()

	arrival := ResolveFlightCompatibilityFacts(FlightCompatibilityInput{
		Direction:      Arrival,
		Origin:         "KJFK",
		Destination:    "EKCH",
		AircraftType:   "32N",
		LiveEngineType: "T",
		WTC:            "m",
	}, aircraft, engines, borders)
	assert.True(t, arrival.Complete())
	assert.Equal(t, EngineTurboprop, arrival.EngineType)
	assert.Equal(t, BorderStatusNonSchengen, arrival.BorderStatus)
	assert.Equal(t, "KJFK", arrival.BorderEndpoint)
	assert.Equal(t, "M", arrival.WTC)

	departure := ResolveFlightCompatibilityFacts(FlightCompatibilityInput{
		Direction:    Departure,
		Origin:       "KJFK",
		Destination:  "EKCH",
		AircraftType: "C172",
	}, aircraft, engines, borders)
	assert.True(t, departure.Complete())
	assert.Equal(t, BorderStatusSchengen, departure.BorderStatus)
	assert.Equal(t, "EKCH", departure.BorderEndpoint)
}

func TestResolveFlightCompatibilityFactsReportsUnknownFacts(t *testing.T) {
	aircraft := testAircraftRegistry(t)
	engines, err := LoadAircraftEngineReference(strings.NewReader(testICAOAircraftJSON), aircraft)
	require.NoError(t, err)
	borders := NewAirportCountryRegistry()

	facts := ResolveFlightCompatibilityFacts(FlightCompatibilityInput{
		Direction:      Arrival,
		Origin:         "XXXX",
		Destination:    "EKCH",
		AircraftType:   "UNKNOWN",
		LiveEngineType: "invalid",
		WTC:            "X",
	}, aircraft, engines, borders)

	assert.False(t, facts.Complete())
	assert.Equal(t, EngineUnknown, facts.EngineType)
	assert.Equal(t, "UNKNOWN", facts.WTC)
	assert.Equal(t, BorderStatusUnknown, facts.BorderStatus)
	assert.ElementsMatch(t, []FlightFact{FlightFactAircraftType, FlightFactEngineType, FlightFactWTC, FlightFactBorder}, facts.UnknownFactKinds())
}

func TestFlightCompatibilityFactsUnknownFactKindsReturnsCopy(t *testing.T) {
	aircraft := testAircraftRegistry(t)
	engines, err := LoadAircraftEngineReference(strings.NewReader(testICAOAircraftJSON), aircraft)
	require.NoError(t, err)

	facts := ResolveFlightCompatibilityFacts(FlightCompatibilityInput{AircraftType: "UNKNOWN"}, aircraft, engines, NewAirportCountryRegistry())
	unknown := facts.UnknownFactKinds()
	unknown[0] = "MUTATED"
	assert.Equal(t, FlightFactAircraftType, facts.UnknownFactKinds()[0])
}
