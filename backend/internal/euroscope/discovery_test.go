package euroscope

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"sync/atomic"
)

type membershipHeartbeatRepo struct {
	testutil.MockControllerRepository
	seen     map[string]int
	detached []string
}

func (r *membershipHeartbeatRepo) SetEuroscopeMembershipSeen(_ context.Context, _ string, _ int32, callsign string, _ *time.Time) (int64, error) {
	r.seen[callsign]++
	return 1, nil
}
func (r *membershipHeartbeatRepo) DetachMembership(_ context.Context, _ string, _ int32, callsign string) (int64, error) {
	r.detached = append(r.detached, callsign)
	return 1, nil
}

func TestFinalMembershipRemovalRetainsControlWithoutOperationalHeartbeat(t *testing.T) {
	hub := membershipTestHub()
	repo := &membershipHeartbeatRepo{seen: map[string]int{}}
	hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &testutil.MockFrontendHub{}}
	root := membershipTestRoot(hub, "123", "instance")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	a.callsign = "FIRST"
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	b.callsign = "SECOND"
	require.NoError(t, root.HandlePong())
	require.Equal(t, map[string]int{"FIRST": 1, "SECOND": 1}, repo.seen)
	hub.RemoveMembership(a, "coverage removed")
	require.NoError(t, root.HandlePong())
	require.Equal(t, 1, repo.seen["FIRST"])
	require.Equal(t, 2, repo.seen["SECOND"])
	hub.RemoveMembership(b, "coverage removed")
	require.NoError(t, root.HandlePong())
	require.False(t, root.isClosed())
	require.Empty(t, root.transport.snapshot())
	for _, frequency := range []string{"", "199.998", "121.500"} {
		payload, err := proto.Marshal(&es.ControlEvent{Environment: "LIVE", Callsign: "EKDK_CTR", PrimaryFrequency: frequency, TunedFrequencies: []string{"121.500"}})
		require.NoError(t, err)
		require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	}
	require.Equal(t, map[string]int{"FIRST": 1, "SECOND": 2}, repo.seen, "control must not keep operational rows alive")
	require.Equal(t, []string{"FIRST", "SECOND"}, repo.detached)
	found, err := hub.DiscoverMemberships(root.userSnapshot())
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Empty(t, found[0].Memberships)
	require.Equal(t, "121.500", found[0].Evidence.PrimaryFrequency)
	require.False(t, found[0].EvidenceAt.IsZero())
	found[0].Evidence.TunedFrequencies[0] = "mutated"
	require.Equal(t, "121.500", hub.ControlPresence()[0].Evidence.TunedFrequencies[0], "discovery must not expose mutable transport evidence")
}

type discoveryAuth func(string) (shared.AuthenticatedUser, error)

func (a discoveryAuth) Validate(token string) (shared.AuthenticatedUser, error) { return a(token) }
func TestDiscoveryAuthorizesCIDAndNeverTrustsQueryIdentity(t *testing.T) {
	hub := membershipTestHub()
	own := membershipTestRoot(hub, "123", "own-instance")
	other := membershipTestRoot(hub, "456", "private-instance")
	membershipTestMember(hub, own, 1, "AAAA", "LIVE", "own")
	membershipTestMember(hub, other, 2, "BBBB", "SWEATBOX_secret", "other")
	auth := discoveryAuth(func(token string) (shared.AuthenticatedUser, error) {
		if token == "own" {
			return own.userSnapshot(), nil
		}
		return shared.AuthenticatedUser{}, errors.New("invalid")
	})
	handler := hub.MembershipDiscovery(auth)
	for _, token := range []string{"", "invalid"} {
		request := httptest.NewRequest(http.MethodGet, "/plugin-memberships", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler(response, request)
		require.Equal(t, http.StatusUnauthorized, response.Code)
	}
	request := httptest.NewRequest(http.MethodGet, "/plugin-memberships?cid=456&session=2", nil)
	request.Header.Set("Authorization", "Bearer own")
	response := httptest.NewRecorder()
	handler(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.NotContains(t, response.Body.String(), "private-instance")
	require.NotContains(t, response.Body.String(), "SWEATBOX_secret")
	var result struct {
		Transports []PluginPresence `json:"transports"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.Transports, 1)
	require.Len(t, result.Transports[0].Memberships, 1)
	require.Equal(t, "own", result.Transports[0].Memberships[0].MembershipID)
	root := membershipTestRoot(hub, "123", "empty-instance")
	require.Empty(t, root.transport.snapshot())
	found, err := hub.DiscoverMemberships(own.userSnapshot())
	require.NoError(t, err)
	require.Len(t, found, 2)
}

func TestControlFrequencyChangeRetainsIndependentMembershipIncarnations(t *testing.T) {
	hub := membershipTestHub()
	updates := []int32{}
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{SetPositionFn: func(_ context.Context, session int32, _ string, _ string) (int64, error) {
		updates = append(updates, session)
		return 1, nil
	}}, FrontendHubVal: &testutil.MockFrontendHub{}, UpdateLayoutsFn: func(int32) error { return nil }}
	root := membershipTestRoot(hub, "123", "instance")
	root.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "EKDK_CTR", PrimaryFrequency: "118.000"}
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	for _, member := range []*Client{a, b} {
		member.updateIdentity("118.000", "EKDK_CTR", false, "")
		hub.master[member.session] = member
	}
	beforeA, beforeB := a.wireContext(), b.wireContext()
	payload, err := proto.Marshal(&es.ControlEvent{Environment: "LIVE", Callsign: "EKDK_CTR", PrimaryFrequency: "119.000"})
	require.NoError(t, err)
	require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	require.ElementsMatch(t, []int32{1, 2}, updates)
	require.Len(t, root.transport.snapshot(), 2)
	require.True(t, a.matches(beforeA))
	require.True(t, b.matches(beforeB))
	require.Equal(t, "119.000", a.GetPosition())
	require.Equal(t, "119.000", b.GetPosition())
}

type blockingDetachRepo struct {
	testutil.MockControllerRepository
	entered, release chan struct{}
}

func (r *blockingDetachRepo) DetachMembership(context.Context, string, int32, string) (int64, error) {
	close(r.entered)
	<-r.release
	return 1, nil
}
func TestReconnectAdmissionWaitsForOldMembershipRowDetach(t *testing.T) {
	backendWorkflowFixture()
	workflows, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
	require.NoError(t, err)
	hub := membershipTestHub()
	hub.airports = backendFixtureDefinitions
	hub.workflows = workflows
	var reads atomic.Int32
	repo := &blockingDetachRepo{entered: make(chan struct{}), release: make(chan struct{})}
	repo.GetFn = func(context.Context, string, int32) (*models.Controller, error) {
		reads.Add(1)
		return &models.Controller{Callsign: "C", Position: "118.000"}, nil
	}
	repo.SetCidFn = func(context.Context, int32, string, *string) (int64, error) { return 1, nil }
	repo.SetObserverFn = func(context.Context, int32, string, bool) (int64, error) { return 1, nil }
	hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &testutil.MockFrontendHub{}, StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, nil }}, GetOrCreateSessionFn: func(string, string) (shared.Session, error) { return shared.Session{Id: 1}, nil }}
	oldRoot := membershipTestRoot(hub, "123", "instance")
	old := membershipTestMember(hub, oldRoot, 1, "EKCH", "LIVE", "old")
	old.callsign = "C"
	cleanup := make(chan error, 1)
	go func() { cleanup <- hub.clearClientCid(old) }()
	<-repo.entered
	newRoot := membershipTestRoot(hub, "123", "instance")
	newRoot.transport.evidence = es.ControlEvent{Environment: "LIVE", Callsign: "C", PrimaryFrequency: "118.000"}
	joined := make(chan error, 1)
	started := make(chan struct{})
	go func() { close(started); _, err := hub.JoinMembership(newRoot, "EKCH"); joined <- err }()
	<-started
	select {
	case err := <-joined:
		t.Fatalf("admission passed old detach: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	require.Zero(t, reads.Load(), "row admission must not revive before old detach finishes")
	close(repo.release)
	require.NoError(t, <-cleanup)
	require.NoError(t, <-joined)
	require.EqualValues(t, 1, reads.Load())
	require.Len(t, newRoot.transport.snapshot(), 1)
	oldRoot.Close()
	newRoot.Close()
}

func TestIdenticalControlRetriesFailedAdmissionWithoutReconnect(t *testing.T) {
	backendWorkflowFixture()
	workflows, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
	require.NoError(t, err)
	hub := membershipTestHub()
	hub.airports = backendFixtureDefinitions
	hub.workflows = workflows
	var attempts atomic.Int32
	repo := &testutil.MockControllerRepository{GetFn: func(context.Context, string, int32) (*models.Controller, error) {
		return &models.Controller{Callsign: "C", Position: "118.000"}, nil
	}, SetCidFn: func(context.Context, int32, string, *string) (int64, error) { return 1, nil }, SetObserverFn: func(context.Context, int32, string, bool) (int64, error) { return 1, nil }}
	hub.server = &testutil.MockServer{ControllerRepoVal: repo, FrontendHubVal: &testutil.MockFrontendHub{}, StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, nil }}, GetOrCreateSessionFn: func(string, string) (shared.Session, error) {
		if attempts.Add(1) == 1 {
			return shared.Session{}, errors.New("transient database failure")
		}
		return shared.Session{Id: 1}, nil
	}}
	root := membershipTestRoot(hub, "123", "instance")
	defer root.Close()
	payload, err := proto.Marshal(&es.ControlEvent{Environment: "LIVE", Callsign: "C", PrimaryFrequency: "118.000", RelevantAirport: "EKCH"})
	require.NoError(t, err)
	require.Error(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	require.Empty(t, root.transport.snapshot())
	require.False(t, root.isClosed())
	require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	require.Len(t, root.transport.snapshot(), 1)
	require.EqualValues(t, 2, attempts.Load())
	require.NoError(t, handleControlEvent(t.Context(), root, Message{Message: payload}))
	require.EqualValues(t, 2, attempts.Load())
}

func TestPhysicalConnectionReturnsOnlyAfterWelcomeIsQueued(t *testing.T) {
	hub := membershipTestHub()
	hub.register = make(chan *Client)
	owner := membershipTestRoot(hub, "123", "source")
	ready := make(chan *Client, 1)
	failed := make(chan error, 1)
	go func() {
		root, err := hub.HandleNewConnection(nil, owner.userSnapshot(), events.AuthenticationEvent{InstanceID: "new-instance"})
		if err != nil {
			failed <- err
			return
		}
		ready <- root
	}()
	root := <-hub.register
	select {
	case <-ready:
		t.Fatal("socket pumps started before transport welcome")
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(20 * time.Millisecond):
	}
	require.True(t, root.Enqueue(es.NewScopedMessage(root.wireContext(), es.ControlEvent{})))
	root.registrationComplete()
	select {
	case client := <-ready:
		require.Same(t, root, client)
	case err := <-failed:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("registration completion did not release connection")
	}
	require.Len(t, root.send, 1)
	root.Close()
}

func TestDiscoveryProjectsBoundDefinitionAndIndependentOwnReadiness(t *testing.T) {
	backendWorkflowFixture()
	workflow, err := airportworkflow.NewRegistry(backendFixtureDefinitions, airportworkflow.EKCHDescriptor())
	require.NoError(t, err)
	bound, _ := workflow.Lookup("EKCH")
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "own")
	a := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "live")
	b := membershipTestMember(hub, root, 2, "EKCH", "SWEATBOX_test", "exercise")
	for _, c := range []*Client{a, b} {
		c.definition = bound.Definition()
		c.workflow = bound
		c.updateIdentity("118.000", "OWN", false, "")
	}
	b.updateIdentity("119.000", "OBSERVER", true, "")
	b.roleEligible.Store(false)
	found, err := hub.DiscoverMemberships(root.userSnapshot())
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Len(t, found[0].Memberships, 2)
	first, second := found[0].Memberships[0], found[0].Memberships[1]
	require.Equal(t, bound.Definition().Revision(), first.DefinitionRevision)
	require.Equal(t, "ekch", first.View)
	require.Equal(t, bound.ActionCatalog(), first.Actions)
	require.Equal(t, "OWN", first.Callsign)
	require.True(t, first.Ready)
	require.False(t, first.ReadOnly)
	require.Equal(t, "SWEATBOX_test", second.Environment)
	require.Equal(t, "observer", second.Role)
	require.False(t, second.Ready)
	require.True(t, second.ReadOnly)
	found[0].Memberships[0].Actions[0] = "poison"
	require.NotContains(t, bound.ActionCatalog(), "poison")
	hub.RemoveMembership(a, "removed")
	found, err = hub.DiscoverMemberships(root.userSnapshot())
	require.NoError(t, err)
	require.Len(t, found[0].Memberships, 1)
}
