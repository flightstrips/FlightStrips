package euroscope

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"FlightStrips/internal/config"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"
)

type coverageFixtureSettings struct {
	InitialBay      string            `yaml:"initial_bay"`
	RunwayBay       string            `yaml:"runway_bay"`
	BayGroundStates map[string]string `yaml:"bay_ground_states"`
	GroundStateBays map[string]string `yaml:"ground_state_bays"`
}

func coverageFixtureRegistry(t *testing.T, grace float64) *config.AirportRegistry {
	t.Helper()
	data, err := os.ReadFile("../../../.github/specs/multi-airport-support/examples/zzzz.airport.yaml")
	require.NoError(t, err)
	catalog, err := config.NewAirportCatalog(config.HandlerDescriptor{ID: "example-paper/v1", View: "example-paper", Layouts: []string{"example-paper"}, DecodeSettings: config.TypedAirportSettings[coverageFixtureSettings], Validate: func(*config.AirportDocument) error { return nil }})
	require.NoError(t, err)
	doc, err := config.NewAirportRegistry(catalog, []config.AirportInput{{Source: "fictional", Reader: bytes.NewReader(data)}})
	require.NoError(t, err)
	fictional, _ := doc.Lookup("ZZZZ")
	var inputs []config.AirportInput
	for _, airport := range []string{"AAAA", "BBBB"} {
		d := fictional.Document()
		d.Airport = airport
		d.Positions = []config.Position{{Name: "ZZZZ_TWR", Frequency: "118.000", Section: "TWR"}, {Name: "ZZZZ_APP", Frequency: "119.000", Section: "APP"}, {Name: "ZZXX_CTR", Frequency: "120.000", Section: "CTR"}}
		d.Coverage.Responsibilities = []config.AirportResponsibility{{ID: "tower", Name: "TWR", Providers: []string{"ZZZZ_TWR", "ZZZZ_APP", "ZZXX_CTR"}}, {ID: "approach", Name: "APP", Providers: []string{"ZZZZ_APP", "ZZXX_CTR"}}}
		if airport == "BBBB" {
			for i := range d.Coverage.Responsibilities {
				d.Coverage.Responsibilities[i].Providers = []string{"ZZXX_CTR"}
			}
		}
		d.Coverage.Membership.RelinquishGraceSeconds = &grace
		d.Master.Priorities = []config.MasterPriority{{Position: "ZZZZ_APP", Match: "primary-frequency-or-callsign"}, {Position: "ZZXX_CTR", Match: "primary-frequency-or-callsign"}}
		encoded, err := yaml.Marshal(d)
		require.NoError(t, err)
		inputs = append(inputs, config.AirportInput{Source: fmt.Sprintf("synthetic-%s", airport), Reader: bytes.NewReader(encoded)})
	}
	registry, err := config.NewAirportRegistry(catalog, inputs)
	require.NoError(t, err)
	return registry
}

func coverageEntry(cid, callsign, frequency string) CoverageControllerEvidence {
	return CoverageControllerEvidence{CoverageParticipant: config.CoverageParticipant{Key: coverageControllerKey(cid, callsign), Callsign: callsign, PrimaryFrequency: frequency}, Identity: CoverageFresh, Radios: CoverageFresh}
}

func coverageFrame(entries ...CoverageControllerEvidence) CoverageStaffingEvidence {
	return CoverageStaffingEvidence{Environment: "LIVE", Inventory: CoverageFresh, Controllers: entries}
}

func coverageTestInstance(t *testing.T) (*Hub, *Client, *Client, *Client) {
	t.Helper()
	hub := membershipTestHub()
	hub.airports = coverageFixtureRegistry(t, 60)
	root := membershipTestRoot(hub, "1", "area")
	root.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000"}
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	for _, member := range []*Client{a, b} {
		member.callsign = "ZZXX_CTR"
		member.definition, _ = hub.airports.Lookup(member.airport)
		hub.master[member.session] = member
	}
	return hub, root, a, b
}

func TestCoveragePartialTakeoverPreservesMembershipsAndIndependentMasters(t *testing.T) {
	hub, root, a, b := coverageTestInstance(t)
	oldA, oldB := a.wireContext(), b.wireContext()
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"), coverageEntry("2", "ZZZZ_TWR", "118.000"))
	require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(string) bool { return true }, frame, time.Now()))
	require.True(t, a.matches(oldA))
	require.True(t, b.matches(oldB))
	require.Same(t, a, hub.getMasterClient(1))
	require.Same(t, b, hub.getMasterClient(2))
	require.Empty(t, root.transport.takeovers)
}

func TestCoverageTakeoverDeadlineDoesNotWaitForReplacementPlugin(t *testing.T) {
	hub, root, a, b := coverageTestInstance(t)
	start := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"), coverageEntry("3", "ZZZZ_APP", "119.000"))
	apply := func(at time.Time) {
		require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(string) bool { return true }, frame, at))
	}
	apply(start)
	require.False(t, a.isClosed())
	apply(start.Add(59 * time.Second))
	require.False(t, a.isClosed())
	oldB := b.wireContext()
	apply(start.Add(60 * time.Second))
	require.True(t, a.isClosed())
	require.False(t, root.isClosed())
	require.True(t, b.matches(oldB))
	require.Same(t, b, hub.getMasterClient(2))
	require.Nil(t, hub.getMasterClient(1))
}

func TestCoverageTakeoverEndsAtAttachmentBeforeReadiness(t *testing.T) {
	hub, root, a, _ := coverageTestInstance(t)
	start := time.Now()
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"), coverageEntry("3", "ZZZZ_APP", "119.000"))
	apply := func(at time.Time) {
		require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(string) bool { return true }, frame, at))
	}
	apply(start)
	appRoot := membershipTestRoot(hub, "3", "app")
	app := membershipTestMember(hub, appRoot, 1, "AAAA", "LIVE", "app")
	app.callsign = "ZZZZ_APP"
	app.announced.Store(false)
	app.roleEligible.Store(false) // intentional plugin warmup is still active
	require.False(t, hub.MembershipReady(app))
	apply(start.Add(time.Second))
	require.True(t, a.isClosed())
	require.False(t, app.roleEligible.Load(), "coverage must not bypass startup delay")
	require.False(t, hub.MembershipReady(app), "attachment is distinct from writable synchronization")
}

func TestCoverageAuthorityRevocationIsScoped(t *testing.T) {
	hub, root, a, b := coverageTestInstance(t)
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"))
	require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(airport string) bool { return airport == "BBBB" }, frame, time.Now()))
	require.True(t, a.isClosed())
	require.False(t, b.isClosed())
	require.Same(t, b, hub.getMasterClient(2))
}

func TestCoverageZeroGraceAndEnvironmentIsolation(t *testing.T) {
	hub, root, a, b := coverageTestInstance(t)
	hub.airports = coverageFixtureRegistry(t, 0)
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"), coverageEntry("3", "ZZZZ_APP", "119.000"))
	frame.Environment = "SWEATBOX_fixture"
	apply := func() {
		require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(string) bool { return true }, frame, time.Now()))
	}
	apply()
	require.False(t, a.isClosed(), "other environments cannot confirm takeover")
	require.Empty(t, root.transport.takeovers)
	frame.Environment = "LIVE"
	apply()
	require.True(t, a.isClosed(), "zero grace must relinquish without waiting for a plugin")
	require.False(t, b.isClosed())
}

func TestCoverageConfirmedTakeoverCancelsWhenResponsibilityReturns(t *testing.T) {
	hub, root, a, _ := coverageTestInstance(t)
	start := time.Now()
	frame := coverageFrame(coverageEntry("1", "ZZXX_CTR", "120.000"), coverageEntry("3", "ZZZZ_APP", "119.000"))
	apply := func(at time.Time) {
		require.NoError(t, hub.ReconcileCoverage(root, []string{"AAAA", "BBBB"}, func(string) bool { return true }, frame, at))
	}
	apply(start)
	require.Contains(t, root.transport.takeovers, "AAAA")
	frame.Controllers = frame.Controllers[:1]
	apply(start.Add(30 * time.Second))
	require.Empty(t, root.transport.takeovers)
	apply(start.Add(60 * time.Second))
	require.False(t, a.isClosed())
}
