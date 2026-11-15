package euroscope

import (
	"testing"
	"time"

	"FlightStrips/internal/vatsim"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func visibleControl(environment string, at time.Time) *es.ControlEvent { // mirrors an SDK control observation
	return &es.ControlEvent{Environment: environment, Callsign: "EKDK_CTR", PrimaryFrequency: "136.555", EligibleAirports: []string{"EKCH", "EKBI"},
		VisibleStaffing: &es.VisibleStaffing{Complete: true, Revision: uint64(at.UnixMilli()), Controllers: []*es.VisibleController{{Callsign: "EKDK_CTR", PrimaryFrequency: "136.555"}}}}
}

func TestVisibleCoverageUsesActualPeersAndObservationFreshness(t *testing.T) {
	now := time.Now()
	presence := PluginPresence{CID: "1", EvidenceAt: now, Evidence: visibleControl("SWEATBOX", now)}
	presence.Evidence.VisibleStaffing.Controllers = append(presence.Evidence.VisibleStaffing.Controllers,
		&es.VisibleController{Callsign: "EKBI_APP", PrimaryFrequency: "127580000"},
		&es.VisibleController{Callsign: "EKBI_ATIS", PrimaryFrequency: "118.000"},
		&es.VisibleController{Callsign: "TRAINING_OBS", Observer: true})
	frame := visibleCoverageEvidence(presence, now, 45*time.Second)
	require.Equal(t, CoverageFresh, frame.Inventory)
	require.Len(t, frame.Controllers, 2)
	require.Equal(t, "/EKDK_CTR", frame.Controllers[0].Key)
	require.Equal(t, "/EKBI_APP", frame.Controllers[1].Key, "nonplugin SDK peers do not need a backend CID")
	require.Equal(t, "127.580", frame.Controllers[1].PrimaryFrequency)
	require.Equal(t, CoverageUnknown, frame.Controllers[1].Radios)
	for _, test := range []struct {
		name string
		edit func(*PluginPresence)
	}{
		{"missing report", func(p *PluginPresence) { p.Evidence.VisibleStaffing = nil }},
		{"partial report", func(p *PluginPresence) { p.Evidence.VisibleStaffing.Complete = false }},
		{"empty roster", func(p *PluginPresence) { p.Evidence.VisibleStaffing.Controllers = nil }},
		{"missing self", func(p *PluginPresence) {
			p.Evidence.VisibleStaffing.Controllers = p.Evidence.VisibleStaffing.Controllers[1:]
		}},
		{"changed primary", func(p *PluginPresence) { p.Evidence.PrimaryFrequency = "124.555" }},
		{"duplicate identity", func(p *PluginPresence) {
			p.Evidence.VisibleStaffing.Controllers = append(p.Evidence.VisibleStaffing.Controllers, p.Evidence.VisibleStaffing.Controllers[0])
		}},
		{"invalid primary", func(p *PluginPresence) { p.Evidence.VisibleStaffing.Controllers[1].PrimaryFrequency = "bad" }},
		{"invalid observation age", func(p *PluginPresence) {
			p.Evidence.VisibleStaffing.ObservationAgeMs = -1
		}},
		{"expired observation", func(p *PluginPresence) {
			p.Evidence.VisibleStaffing.ObservationAgeMs = 60_000
		}},
		{"stale transport", func(p *PluginPresence) { p.EvidenceAt = now.Add(-time.Minute) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := presence
			p.Evidence = proto.Clone(presence.Evidence).(*es.ControlEvent)
			test.edit(&p)
			require.NotEqual(t, CoverageFresh, visibleCoverageEvidence(p, now, 45*time.Second).Inventory)
		})
	}
}

func TestVisibleCoverageNormalCoordinatorUsesOnlyOwnViewWithoutPublicFeed(t *testing.T) {
	hub := coverageJoinHub(t)
	root := membershipTestRoot(hub, "1", "exercise")
	defer root.Close()
	defer hub.removeTransport(root)
	now := time.Now()
	control := visibleControl("LIVE", now)
	control.Callsign = "ZZXX_CTR"
	control.PrimaryFrequency = "120.000"
	control.EligibleAirports = []string{"AAAA"}
	control.VisibleStaffing.Controllers = []*es.VisibleController{{Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000"}}
	root.transport.evidence = *control
	root.transport.evidenceAt = now
	other := membershipTestRoot(hub, "2", "other")
	defer other.Close()
	defer hub.removeTransport(other)
	other.transport.evidence = *proto.Clone(control).(*es.ControlEvent)
	other.transport.evidence.Environment = "SWEATBOX_OTHER"
	other.transport.evidence.EligibleAirports = nil
	other.transport.evidence.VisibleStaffing.Controllers = append(other.transport.evidence.VisibleStaffing.Controllers, &es.VisibleController{Callsign: "ZZZZ_APP", PrimaryFrequency: "119.000"})
	other.transport.evidenceAt = now
	coordinator := hub.ConfigureCoverage(CoverageOptions{Now: func() time.Time { return now }})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Len(t, root.transport.snapshot(), 1, "normal runtime bootstraps from the wire report without injected source")
	require.Equal(t, "LIVE", root.transport.snapshot()[0].sessionName)
	frames := BuildCoverageEvidence(vatsim.Snapshot{}, vatsim.TransceiverSnapshot{}, hub.ControlPresence(), now, time.Minute)
	require.Equal(t, CoverageUnknown, frames["LIVE"].Inventory)
}

func TestVisibleObservationCannotRegressAndResetsAcrossEnvironments(t *testing.T) {
	hub := coverageJoinHub(t)
	root := membershipTestRoot(hub, "1", "exercise")
	defer root.Close()
	defer hub.removeTransport(root)
	now := time.Now().Add(-time.Second)
	control := visibleControl("SWEATBOX", now)
	update := func() {
		payload, err := proto.Marshal(control)
		require.NoError(t, err)
		require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	}
	update()
	control.VisibleStaffing.Revision--
	update()
	require.False(t, root.transport.evidence.VisibleStaffing.Complete, "older observations cannot roll back a roster within its freshness window")
	update()
	require.False(t, root.transport.evidence.VisibleStaffing.Complete, "high-water mark survives rejected observations")
	control.Environment = "PLAYBACK_OTHER"
	update()
	require.True(t, root.transport.evidence.VisibleStaffing.Complete, "a new environment establishes its own observation sequence")
	control.Environment = "LIVE"
	update()
	require.True(t, root.transport.evidence.VisibleStaffing.Complete)
}

func TestVisibleRosterTieBreakIsConsistentAcrossInstances(t *testing.T) {
	hub := coverageJoinHub(t)
	now := time.Now()
	first := membershipTestRoot(hub, "1", "first")
	second := membershipTestRoot(hub, "2", "second")
	defer first.Close()
	defer second.Close()
	defer hub.removeTransport(first)
	defer hub.removeTransport(second)
	controllers := []*es.VisibleController{{Callsign: "GENERIC_A_CTR", PrimaryFrequency: "120.000"}, {Callsign: "GENERIC_B_CTR", PrimaryFrequency: "120.000"}}
	for i, root := range []*Client{first, second} {
		root.transport.evidenceAt = now
		root.transport.evidence = es.ControlEvent{Environment: "SWEATBOX", Callsign: controllers[i].Callsign, PrimaryFrequency: "120.000", EligibleAirports: []string{"AAAA"}, VisibleStaffing: &es.VisibleStaffing{Complete: true, Revision: 1, Controllers: controllers}}
	}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Now: func() time.Time { return now }})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Len(t, first.transport.snapshot(), 1)
	require.Empty(t, second.transport.snapshot(), "both instances must agree which same-frequency controller wins")
}
