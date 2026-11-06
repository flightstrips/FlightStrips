package euroscope

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type coverageSnapshotSource struct {
	snapshot vatsim.Snapshot
	reads    int
	onRead   func()
}

func (s *coverageSnapshotSource) Snapshot() vatsim.Snapshot {
	s.reads++
	if s.onRead != nil {
		s.onRead()
	}
	return s.snapshot
}

type coverageRadioSnapshot struct {
	snapshot vatsim.TransceiverSnapshot
	reads    int
}

func (s *coverageRadioSnapshot) Snapshot() vatsim.TransceiverSnapshot { s.reads++; return s.snapshot }

type coverageFixtureHandler struct{}

func (coverageFixtureHandler) Observe(airportworkflow.Context, airportworkflow.Observation) (airportworkflow.Decision, error) {
	return airportworkflow.Decision{}, nil
}
func (coverageFixtureHandler) Lifecycle(airportworkflow.Context, airportworkflow.Lifecycle) (airportworkflow.LifecycleDecision, error) {
	return airportworkflow.LifecycleDecision{}, nil
}
func (coverageFixtureHandler) Coordinate(airportworkflow.Context, airportworkflow.Coordination) (airportworkflow.CoordinationDecision, error) {
	return airportworkflow.CoordinationDecision{}, nil
}

type coverageControllerRepo struct {
	testutil.MockControllerRepository
}

func (*coverageControllerRepo) UpsertMembershipController(context.Context, *models.Controller, bool) error {
	return nil
}
func (*coverageControllerRepo) DetachMembership(context.Context, string, int32, string) (int64, error) {
	return 1, nil
}

func coverageJoinHub(t *testing.T) *Hub {
	t.Helper()
	hub := membershipTestHub()
	hub.airports = coverageFixtureRegistry(t, 0)
	workflows, err := airportworkflow.NewRegistry(hub.airports, airportworkflow.Descriptor{ID: "example-paper/v1", Bind: func(*config.AirportDefinition) (airportworkflow.Handler, []airportworkflow.ActionDescriptor, error) {
		return coverageFixtureHandler{}, nil, nil
	}})
	require.NoError(t, err)
	hub.workflows = workflows
	repo := &coverageControllerRepo{}
	repo.ListFn = func(context.Context, int32) ([]*models.Controller, error) { return nil, nil }
	hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &testutil.MockFrontendHub{}, StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, nil }}, SessionRepoVal: &testutil.MockSessionRepository{GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
		return &models.Session{ID: id, Airport: "AAAA", Name: "LIVE"}, nil
	}}, GetOrCreateSessionFn: func(airport, environment string) (shared.Session, error) {
		if airport == "AAAA" {
			return shared.Session{Id: 1}, nil
		}
		return shared.Session{Id: 2}, nil
	}}
	return hub
}

func TestCoverageWorkerAutomaticallyRejoinsFromZeroAndRequiresNewSync(t *testing.T) {
	var appOnline atomic.Bool
	appOnline.Store(true)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			fmt.Fprintf(w, `{"data":{"v3":["http://%s/data"]}}`, r.Host)
			return
		}
		controller := ""
		if appOnline.Load() {
			controller = `{"cid":3,"callsign":"ZZZZ_APP","frequency":"119.000","facility":5}`
		}
		fmt.Fprintf(w, `{"general":{"update_timestamp":"%s"},"controllers":[%s]}`, time.Now().UTC().Format(time.RFC3339Nano), controller)
	}))
	defer provider.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	network := vatsim.NewCache(provider.URL+"/status", 25*time.Millisecond, provider.Client())
	go network.Start(ctx)
	require.Eventually(t, func() bool { return len(network.Snapshot().Controllers()) == 1 }, time.Second, time.Millisecond)
	hub := coverageJoinHub(t)
	root := membershipTestRoot(hub, "1", "retained-area")
	defer root.Close()
	defer hub.removeTransport(root)
	control := &es.ControlEvent{Environment: "LIVE", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000", TunedFrequenciesKnown: true, EligibleAirports: []string{"AAAA"}}
	payload, err := proto.Marshal(control)
	require.NoError(t, err)
	require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	radios := &coverageRadioSnapshot{snapshot: vatsim.TransceiverSnapshot{Timestamp: time.Now().Add(-time.Second)}}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Network: network, Radios: radios})
	require.NotNil(t, coordinator)
	_, err = coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Empty(t, root.transport.snapshot())
	require.True(t, hub.HasLiveSourceDemand())
	done := make(chan struct{})
	go func() { coordinator.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	// No control/callsign update or explicit join occurs when APP leaves.
	appOnline.Store(false)
	require.Eventually(t, func() bool { return len(root.transport.snapshot()) == 1 }, 3*time.Second, time.Millisecond)
	member := root.transport.snapshot()[0]
	old := member.wireContext()
	require.False(t, hub.MembershipReady(member), "new attachment starts read-only")
	_, err = hub.CaptureMembershipBinding(1, "1", root.transport.instance, old.MembershipId)
	require.Error(t, err)
	require.Eventually(t, func() bool { return member.announced.Load() && member.roleEligible.Load() }, 3*time.Second, time.Millisecond)
	_, err = hub.CaptureMembershipBinding(1, "1", root.transport.instance, old.MembershipId)
	require.Error(t, err, "startup role alone must not bypass current-epoch full sync")
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.Sync, handleSync)
	require.NoError(t, handlers.Handle(t.Context(), member, authorityMessage(t, es.Sync, &es.SyncEvent{}, member.wireContext())))
	require.True(t, hub.MembershipReady(member))
	_, err = hub.CaptureMembershipBinding(1, "1", root.transport.instance, old.MembershipId)
	require.NoError(t, err)
	appOnline.Store(true)
	require.Eventually(t, func() bool { return member.isClosed() && len(root.transport.snapshot()) == 0 }, 3*time.Second, time.Millisecond)
	require.False(t, root.isClosed())
	require.True(t, hub.HasLiveSourceDemand())
	require.False(t, hub.IsSessionSynced(1))
	appOnline.Store(false)
	require.Eventually(t, func() bool { return len(root.transport.snapshot()) == 1 }, 3*time.Second, time.Millisecond)
	next := root.transport.snapshot()[0]
	require.NotEqual(t, old.MembershipId, next.membership.MembershipId)
	require.False(t, hub.MembershipReady(next), "automatic rejoin needs another full sync")
	_, err = hub.ResolveMessage(root, old, "sync")
	require.Error(t, err, "retired scope must not become valid again after rejoin")
}

func TestCoverageCoordinatorSharesOneGenerationAcrossMembershipsAndDeniesDeclaration(t *testing.T) {
	hub, root, a, b := coverageTestInstance(t)
	now := time.Now()
	root.transport.evidenceAt = now
	root.transport.evidence.TunedFrequenciesKnown = true
	root.transport.evidence.EligibleAirports = []string{"AAAA", "BBBB", "NOT-ENABLED"}
	network := &coverageSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now, ControllersKnown: true}}
	radios := &coverageRadioSnapshot{snapshot: vatsim.TransceiverSnapshot{Timestamp: now}}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Network: network, Radios: radios, Now: func() time.Time { return now }, Authorize: func(_ shared.AuthenticatedUser, airport, environment string) bool {
		return airport == "BBBB" && environment == "LIVE"
	}})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, network.reads)
	require.Equal(t, 1, radios.reads)
	require.True(t, a.isClosed())
	require.False(t, b.isClosed())
}

func TestCoverageSnapshotCannotOvertakeControlDeclarationChange(t *testing.T) {
	hub := coverageJoinHub(t)
	root := membershipTestRoot(hub, "1", "area")
	defer root.Close()
	control := &es.ControlEvent{Environment: "LIVE", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000", TunedFrequenciesKnown: true, EligibleAirports: []string{"AAAA"}}
	update := func() {
		payload, err := proto.Marshal(control)
		require.NoError(t, err)
		require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	}
	update()
	now := time.Now()
	network := &coverageSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now, ControllersKnown: true}, onRead: func() { control.EligibleAirports = nil; update() }}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Network: network, Now: func() time.Time { return now }})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Empty(t, root.transport.snapshot(), "old declaration cannot create membership after revocation")
}

func TestCoverageUsesOwnInstanceRadioEvidenceForDuplicateControllerIdentity(t *testing.T) {
	hub := coverageJoinHub(t)
	first := membershipTestRoot(hub, "1", "first")
	second := membershipTestRoot(hub, "1", "second")
	defer first.Close()
	defer second.Close()
	defer hub.removeTransport(first)
	defer hub.removeTransport(second)
	now := time.Now()
	for _, root := range []*Client{first, second} {
		root.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "UNCONFIGURED", PrimaryFrequency: "121.000", TunedFrequenciesKnown: true, EligibleAirports: []string{"AAAA"}}
		root.transport.evidenceAt = now
	}
	first.transport.evidence.TunedFrequencies = []string{"118.000"}
	network := &coverageSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now, ControllersKnown: true}}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Network: network, Now: func() time.Time { return now }})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Len(t, first.transport.snapshot(), 1, "the first instance carries tower despite the second instance's empty radios")
	require.Empty(t, second.transport.snapshot(), "a sibling instance cannot inherit the first instance's tuned eligibility")
}

func TestCoverageSimulationRequiresItsOwnFreshInventory(t *testing.T) {
	hub := coverageJoinHub(t)
	root := membershipTestRoot(hub, "1", "training")
	defer root.Close()
	defer hub.removeTransport(root)
	now := time.Now()
	root.transport.evidence = es.ControlEvent{Environment: "SWEATBOX_FIXTURE", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000", TunedFrequenciesKnown: true, EligibleAirports: []string{"AAAA"}}
	root.transport.evidenceAt = now
	source := CoverageStaffingEvidence{Environment: "LIVE", Timestamp: now, Inventory: CoverageFresh}
	coordinator := hub.ConfigureCoverage(CoverageOptions{Network: &coverageSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now, ControllersKnown: true}}, Now: func() time.Time { return now }, Simulated: func(context.Context) ([]CoverageStaffingEvidence, error) {
		return []CoverageStaffingEvidence{source}, nil
	}})
	_, err := coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Empty(t, root.transport.snapshot(), "LIVE evidence cannot supply simulated inventory")
	source.Environment = "SWEATBOX_FIXTURE"
	source.Timestamp = now.Add(-time.Hour)
	_, err = coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Empty(t, root.transport.snapshot(), "expired simulation evidence cannot confirm absence")
	source.Timestamp = now
	_, err = coordinator.Reconcile(t.Context())
	require.NoError(t, err)
	require.Len(t, root.transport.snapshot(), 1)
	require.Equal(t, "SWEATBOX_FIXTURE", root.transport.snapshot()[0].sessionName)
	require.False(t, hub.HasLiveSourceDemand())
}

func TestCoverageLegacyPolicyDoesNotInstallAdditionalWorker(t *testing.T) {
	backendWorkflowFixture()
	hub := membershipTestHub()
	hub.airports = backendFixtureDefinitions
	require.Nil(t, hub.ConfigureCoverage(CoverageOptions{}))
}
