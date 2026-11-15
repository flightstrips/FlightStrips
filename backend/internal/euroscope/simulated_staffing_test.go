package euroscope

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
)

func simulatedRoster(now time.Time) simulatedStaffingDocument {
	return simulatedStaffingDocument{Version: "flightstrips-simulated-staffing/v1", Environments: []simulatedStaffingEnvironment{{
		Environment: "SWEATBOX", ObservedAt: now, ControllersComplete: true,
		Controllers: []simulatedStaffingController{{CID: "1", Callsign: "EKDK_CTR", PrimaryFrequency: "136.555", TunedFrequenciesKnown: true, TunedFrequencies: []string{}}},
	}}}
}

func writeSimulatedRoster(t *testing.T, path string, document simulatedStaffingDocument) {
	t.Helper()
	data, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0600))
}

func TestSimulatedStaffingFileValidatesExplicitSourceFacts(t *testing.T) {
	now := time.Now()
	for _, test := range []struct {
		name string
		edit func(*simulatedStaffingDocument)
	}{
		{"wrong version", func(d *simulatedStaffingDocument) { d.Version = "unknown" }},
		{"missing environments", func(d *simulatedStaffingDocument) { d.Environments = nil }},
		{"LIVE", func(d *simulatedStaffingDocument) { d.Environments[0].Environment = "LIVE" }},
		{"invalid environment", func(d *simulatedStaffingDocument) { d.Environments[0].Environment = "OTHER" }},
		{"duplicate environment", func(d *simulatedStaffingDocument) { d.Environments = append(d.Environments, d.Environments[0]) }},
		{"missing timestamp", func(d *simulatedStaffingDocument) { d.Environments[0].ObservedAt = time.Time{} }},
		{"missing inventory", func(d *simulatedStaffingDocument) { d.Environments[0].Controllers = nil }},
		{"duplicate controller", func(d *simulatedStaffingDocument) {
			d.Environments[0].Controllers = append(d.Environments[0].Controllers, d.Environments[0].Controllers[0])
		}},
		{"missing cid", func(d *simulatedStaffingDocument) { d.Environments[0].Controllers[0].CID = "" }},
		{"missing callsign", func(d *simulatedStaffingDocument) { d.Environments[0].Controllers[0].Callsign = "" }},
		{"invalid primary", func(d *simulatedStaffingDocument) { d.Environments[0].Controllers[0].PrimaryFrequency = "bad" }},
		{"invalid tuned", func(d *simulatedStaffingDocument) {
			d.Environments[0].Environment = "PLAYBACK_EXERCISE1"
			d.Environments[0].Controllers[0].TunedFrequencies = []string{"119.x"}
		}},
		{"unspecified known radios", func(d *simulatedStaffingDocument) {
			d.Environments[0].Environment = "PLAYBACK_EXERCISE1"
			d.Environments[0].Controllers[0].TunedFrequencies = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "staffing.json")
			document := simulatedRoster(now)
			test.edit(&document)
			writeSimulatedRoster(t, path, document)
			frames, err := SimulatedStaffingFile(path)(t.Context())
			require.Error(t, err)
			require.Empty(t, frames)
		})
	}
	for _, input := range []string{`{`, `{"version":"flightstrips-simulated-staffing/v1","environments":[],"typo":true}`, `{"version":"flightstrips-simulated-staffing/v1","environments":[]} {}`} {
		_, err := decodeSimulatedStaffing(strings.NewReader(input))
		require.Error(t, err)
	}
	path := filepath.Join(t.TempDir(), "staffing.json")
	source := SimulatedStaffingFile(path)
	_, err := source(t.Context())
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = source(ctx)
	require.ErrorIs(t, err, context.Canceled)
	document := simulatedRoster(now)
	document.Environments[0].Environment = "PLAYBACK_EXERCISE1"
	document.Environments[0].Controllers[0].TunedFrequencies = []string{"119005000", "119.005"}
	writeSimulatedRoster(t, path, document)
	frames, err := source(t.Context())
	require.NoError(t, err)
	require.Equal(t, now.UnixNano(), frames[0].Timestamp.UnixNano(), "reading must not renew the observation")
	require.Equal(t, []string{"119.005"}, frames[0].Controllers[0].TunedFrequencies)
	document.Environments[0].ControllersComplete = false
	document.Environments[0].Controllers[0].TunedFrequenciesKnown = false
	writeSimulatedRoster(t, path, document)
	frames, err = source(t.Context())
	require.NoError(t, err)
	require.Equal(t, CoverageUnknown, frames[0].Inventory)
	require.Equal(t, CoverageUnknown, frames[0].Controllers[0].Radios)
	require.Empty(t, frames[0].Controllers[0].TunedFrequencies)
	document.Environments[0].Environment = "SWEATBOX"
	document.Environments[0].Controllers[0].TunedFrequenciesKnown = true
	document.Environments[0].Controllers[0].TunedFrequencies = nil
	writeSimulatedRoster(t, path, document)
	frames, err = source(t.Context())
	require.NoError(t, err, "SWEATBOX does not require even a supposedly known tuned list")
	require.Equal(t, CoverageUnknown, frames[0].Controllers[0].Radios)
}

// Use the real EKBI provider order and the reported EKDK_CTR/136.555 identity.
// Inventory is supplied through the same acquisition function as cmd/server.
func TestSimulatedStaffingAcquisitionAndEKBIRejoin(t *testing.T) {
	hub := coverageJoinHub(t)
	registry, err := config.LoadAirportFiles(config.ProductionAirportCatalog(), []string{"backend/config/airports/ekbi.airport.yaml"}, config.AirportLoadOptions{RepositoryRoot: "../../..", LookupEnv: func(string) (string, bool) { return "backend/config/test/ICAO_Aircraft.json", true }})
	require.NoError(t, err)
	hub.airports = registry
	hub.workflows, err = airportworkflow.NewRegistry(registry, airportworkflow.RADISDescriptor())
	require.NoError(t, err)
	root := membershipTestRoot(hub, "1", "exercise")
	defer root.Close()
	defer hub.removeTransport(root)
	now := time.Now()
	root.transport.evidenceAt = now
	// Like the real plugin, own tuned frequencies are unknown. SWEATBOX must use
	// configured primary frequency/callsign without inventing additional radios.
	root.transport.evidence = es.ControlEvent{Environment: "SWEATBOX", Callsign: "EKDK_CTR", PrimaryFrequency: "136.555", EligibleAirports: []string{"EKCH", "EKBI", "EKYT"}}
	path := filepath.Join(t.TempDir(), "staffing.json")
	coordinator := hub.ConfigureCoverage(CoverageOptions{Now: func() time.Time { return now }, Simulated: SimulatedStaffingFile(path)})
	apply := func() { t.Helper(); _, err := coordinator.Reconcile(t.Context()); require.NoError(t, err) }
	for _, test := range []struct {
		name string
		edit func(*simulatedStaffingDocument)
	}{
		{"incomplete roster", func(d *simulatedStaffingDocument) { d.Environments[0].ControllersComplete = false }},
		{"empty roster", func(d *simulatedStaffingDocument) { d.Environments[0].Controllers = []simulatedStaffingController{} }},
		{"wrong exercise", func(d *simulatedStaffingDocument) { d.Environments[0].Environment = "SWEATBOX_OTHER" }},
		{"playback", func(d *simulatedStaffingDocument) { d.Environments[0].Environment = "PLAYBACK_OTHER" }},
		{"expired roster", func(d *simulatedStaffingDocument) { d.Environments[0].ObservedAt = now.Add(-time.Minute) }},
		{"future roster", func(d *simulatedStaffingDocument) { d.Environments[0].ObservedAt = now.Add(time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			document := simulatedRoster(now)
			test.edit(&document)
			writeSimulatedRoster(t, path, document)
			apply()
			require.Empty(t, root.transport.snapshot())
		})
	}
	document := simulatedRoster(now)
	document.Environments[0].Controllers[0].TunedFrequenciesKnown = false
	writeSimulatedRoster(t, path, document)
	apply()
	require.Len(t, root.transport.snapshot(), 1)
	member := root.transport.snapshot()[0]
	require.Equal(t, "EKBI", member.airport)
	require.Equal(t, "SWEATBOX", member.sessionName)
	require.False(t, hub.MembershipReady(member), "ordinary startup and sync remain required")
	old := member.wireContext()

	// A nonplugin APP takes over the duties using only its configured primary
	// frequency; neither controller can report additional tuned radios in SWEATBOX.
	document.Environments[0].Controllers = append(document.Environments[0].Controllers, simulatedStaffingController{CID: "2", Callsign: "EKBI_APP", PrimaryFrequency: "127.580"})
	writeSimulatedRoster(t, path, document)
	apply()
	require.Empty(t, root.transport.snapshot(), "EKBI's configured zero grace applies to nonplugin takeover")
	require.False(t, root.isClosed())

	document.Environments[0].Controllers = document.Environments[0].Controllers[:1]
	writeSimulatedRoster(t, path, document)
	apply()
	require.Len(t, root.transport.snapshot(), 1)
	rejoined := root.transport.snapshot()[0]
	require.NotEqual(t, old.MembershipId, rejoined.wireContext().MembershipId)
	require.False(t, hub.MembershipReady(rejoined))
	_, err = hub.ResolveMessage(root, old, "sync")
	require.Error(t, err)
	require.NoError(t, os.WriteFile(path, []byte(`{`), 0600))
	_, err = coordinator.Reconcile(t.Context())
	require.Error(t, err)
	require.False(t, rejoined.isClosed(), "source failure must preserve established membership")

	// The same source cannot supply LIVE inventory even for the same controller.
	root.transport.evidence.Environment = "LIVE"
	frames := BuildCoverageEvidence(vatsim.Snapshot{}, vatsim.TransceiverSnapshot{}, hub.ControlPresence(), now, time.Minute, CoverageStaffingEvidence{Environment: "SWEATBOX", Timestamp: now, Inventory: CoverageFresh})
	require.Equal(t, CoverageUnknown, frames["LIVE"].Inventory)
}
