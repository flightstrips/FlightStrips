package euroscope

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FlightStrips/internal/vatsim"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
)

func TestSharedCoverageEvidenceIncludesNonPluginControllersAndIsolatesSimulation(t *testing.T) {
	networkServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			fmt.Fprintf(w, `{"data":{"v3":["http://%s/data"]}}`, r.Host)
			return
		}
		fmt.Fprintf(w, `{"general":{"update_timestamp":"%s"},"controllers":[{"cid":1,"callsign":"ZZZZ_APP","frequency":"119.000","facility":5},{"cid":2,"callsign":"ZZZZ_OBS","frequency":"118.000","facility":0}],"atis":[{"cid":3,"callsign":"ZZZZ_ATIS","frequency":"118.000","facility":4}]}`, time.Now().UTC().Format(time.RFC3339Nano))
	}))
	defer networkServer.Close()
	radioServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"callsign":"ZZZZ_APP","transceivers":[{"frequency":118000000}]}]`)
	}))
	defer radioServer.Close()
	network := vatsim.NewCache(networkServer.URL+"/status", time.Hour, networkServer.Client())
	radios := vatsim.NewTransceiverCache(radioServer.URL, time.Hour, radioServer.Client(), nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go network.Start(ctx)
	go radios.Start(ctx)
	require.Eventually(t, func() bool { return !network.Snapshot().Timestamp.IsZero() && !radios.Snapshot().Timestamp.IsZero() }, time.Second, time.Millisecond)
	sim := PluginPresence{CID: "1", EvidenceAt: time.Now(), Evidence: &es.ControlEvent{Environment: "SWEATBOX_fixture", Callsign: "ZZZZ_APP", PrimaryFrequency: "120.000", TunedFrequenciesKnown: true}}
	result := BuildCoverageEvidence(network.Snapshot(), radios.Snapshot(), []PluginPresence{sim}, time.Now(), time.Minute)
	require.Len(t, result["LIVE"].Controllers, 1)
	require.Equal(t, "119.000", result["LIVE"].Controllers[0].PrimaryFrequency)
	require.Equal(t, []string{"118.000"}, result["LIVE"].Controllers[0].TunedFrequencies)
	require.Equal(t, CoverageFresh, result["LIVE"].Inventory)
	require.Equal(t, CoverageUnknown, result["SWEATBOX_fixture"].Inventory)
	require.Empty(t, result["SWEATBOX_fixture"].Controllers[0].TunedFrequencies)
	require.Equal(t, "120.000", result["SWEATBOX_fixture"].Controllers[0].PrimaryFrequency)
	sim.Evidence.Environment = "LIVE"
	sim.Evidence.TunedFrequenciesKnown = false
	result = BuildCoverageEvidence(network.Snapshot(), radios.Snapshot(), []PluginPresence{sim}, time.Now(), time.Minute)
	require.Equal(t, []string{"118.000"}, result["LIVE"].Controllers[0].TunedFrequencies, "unknown plugin radios must not erase provider carriage")
	sim.Evidence.TunedFrequenciesKnown = true
	result = BuildCoverageEvidence(network.Snapshot(), radios.Snapshot(), []PluginPresence{sim}, time.Now(), time.Minute)
	require.Empty(t, result["LIVE"].Controllers[0].TunedFrequencies, "known empty plugin radios override the public generation")
	require.Equal(t, []string{"118.000"}, radios.GetFrequencies("ZZZZ_APP"), "projection must not mutate shared sources")
}

func TestMissingCoverageInventoryIsUnknown(t *testing.T) {
	result := BuildCoverageEvidence(vatsim.Snapshot{}, vatsim.TransceiverSnapshot{}, nil, time.Now(), time.Minute)
	require.Equal(t, CoverageUnknown, result["LIVE"].Inventory)
	require.Empty(t, result["LIVE"].Controllers)
}

func TestCoverageFreshnessBoundariesAndLastGoodFailure(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		at   time.Time
		err  error
		want CoverageEvidenceState
	}{
		{"missing", time.Time{}, nil, CoverageUnknown},
		{"fresh", now.Add(-time.Minute + time.Nanosecond), nil, CoverageFresh},
		{"expired", now.Add(-time.Minute), nil, CoverageStale},
		{"future", now.Add(time.Nanosecond), nil, CoverageStale},
		{"failed-refresh", now, errors.New("unavailable"), CoverageStale},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, coverageSourceState(now, test.at, time.Minute, test.err))
		})
	}
	network := vatsim.Snapshot{Timestamp: now, ControllersKnown: true, LastRefreshError: errors.New("refresh failed")}
	plugin := PluginPresence{CID: "1", EvidenceAt: now, Evidence: &es.ControlEvent{Environment: "LIVE", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000", TunedFrequenciesKnown: true}}
	frame := BuildCoverageEvidence(network, vatsim.TransceiverSnapshot{}, []PluginPresence{plugin}, now, time.Minute)["LIVE"]
	require.Equal(t, CoverageStale, frame.Inventory, "fresh own control cannot prove network absence")
	require.Equal(t, CoverageFresh, frame.Controllers[0].Identity)
	require.Equal(t, CoverageFresh, frame.Controllers[0].Radios)
}
