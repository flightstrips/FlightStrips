package navdata

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerminalFragmentLegacyPayloadRetainsCanonicalDigest(t *testing.T) {
	version, provenance := testVersion(), testProvenance()
	validated := provenance.ImportedAt.Add(time.Minute)
	legacyPath := TerminalPath{Version: version, Airport: "EKCH", Feeder: "TESPI", RunwayGroup: "SOUTH", Coverage: CoverageComplete, Provenance: provenance, Digest: "legacy-path"}
	legacyPayload := struct {
		Airport       AirportID
		ConfigVersion string
		Paths         []TerminalPath
		Holdings      []HoldingPattern
	}{"EKCH", "legacy-v1", []TerminalPath{legacyPath}, nil}
	digest, err := CanonicalFragmentDigest(CanonicalSchemaVersion, version, provenance, legacyPayload)
	require.NoError(t, err)
	fragment := CandidateTerminalFragment{SchemaVersion: CanonicalSchemaVersion, Version: version, Airport: "EKCH", ConfigVersion: "legacy-v1", Paths: []TerminalPath{legacyPath}, Provenance: provenance, ImportedAt: provenance.ImportedAt, ValidatedAt: &validated, State: ValidationValidated, Digest: digest}

	encoded, err := MarshalTerminalFragmentPayload(fragment)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "STARFamily")
	require.NotContains(t, string(encoded), "FeederFix")
	require.NoError(t, UnmarshalTerminalFragmentPayload(encoded, &fragment))
	require.NoError(t, fragment.Validate())
}

func TestTerminalFragmentExplicitFieldsAffectDigestAndRoundTrip(t *testing.T) {
	version, provenance := testVersion(), testProvenance()
	duration := 3*time.Minute + 15*time.Second
	path := TerminalPath{Version: version, Airport: "EKCH", Feeder: "TESPI", STARFamily: "TESPI", FeederFix: "TNO", HoldingToFeederDuration: &duration, RunwayGroup: "SOUTH", Coverage: CoverageComplete, Provenance: provenance, Digest: "explicit-path"}
	fragment := CandidateTerminalFragment{Airport: "EKCH", ConfigVersion: "explicit-v1", Paths: []TerminalPath{path}}

	encoded, err := MarshalTerminalFragmentPayload(fragment)
	require.NoError(t, err)
	var decoded CandidateTerminalFragment
	require.NoError(t, UnmarshalTerminalFragmentPayload(encoded, &decoded))
	require.Equal(t, fragment.Airport, decoded.Airport)
	require.Equal(t, fragment.ConfigVersion, decoded.ConfigVersion)
	require.Equal(t, path, decoded.Paths[0])

	legacy := fragment
	legacy.Paths = append([]TerminalPath(nil), fragment.Paths...)
	legacy.Paths[0].STARFamily = ""
	legacy.Paths[0].FeederFix = ""
	legacy.Paths[0].HoldingToFeederDuration = nil
	explicitDigest, err := CanonicalPayloadDigest(fragment.payload())
	require.NoError(t, err)
	legacyDigest, err := CanonicalPayloadDigest(legacy.payload())
	require.NoError(t, err)
	require.NotEqual(t, legacyDigest, explicitDigest)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(encoded, &raw))
	paths := raw["Paths"].([]any)
	storedPath := paths[0].(map[string]any)
	require.Equal(t, "TESPI", storedPath["STARFamily"])
	require.Equal(t, "TNO", storedPath["FeederFix"])
	require.EqualValues(t, duration, storedPath["HoldingToFeederDuration"])
}
