package frontend

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type delayedBoardSource struct {
	boardRecoverySource
	synced bool
}

type warmingBoardSource struct {
	boardRecoverySource
	ready bool
}

func (s *warmingBoardSource) FrontendBoardReady(string, events.BoardContext) bool {
	return s.available && s.ready
}
func (s *warmingBoardSource) AuthorizeFrontendBoard(cid string, board events.BoardContext) (*models.Controller, frontendEvents.BoardDefinition, bool, error) {
	controller, definition, readOnly, err := s.boardRecoverySource.AuthorizeFrontendBoard(cid, board)
	return controller, definition, readOnly || !s.ready, err
}

func TestSelectedBoardRechecksReadinessAfterInitialSnapshotWithoutNotification(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	source := &warmingBoardSource{boardRecoverySource: boardRecoverySource{available: true}}
	hub.server.(*testutil.MockServer).EuroscopeHubVal = source
	board := events.BoardContext{SessionID: 42, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "tab"}
	client := startQueuedTestClient(&Client{hub: hub, session: 42, sessionName: "LIVE", airport: "EKCH", callsign: "EKCH_A_TWR", position: "118.105", user: validFrontendUser("1234567"), selection: membershipSelection{board: &board, session: 42, instance: "own", membership: "member"}})
	hub.clients = map[*Client]bool{client: true}
	hub.OnRegister(client)
	initial := (<-client.send).(frontendEvents.BoundMessage).Message.(frontendEvents.InitialEvent)
	require.True(t, initial.ReadOnly)
	require.False(t, initial.OwnPluginReady)
	source.ready = true
	hub.retryPendingInitializations(t.Context())
	require.Len(t, client.send, 1, "a not-ready initial snapshot must not consume the only recovery attempt")
	bound := (<-client.send).(frontendEvents.BoundMessage)
	require.Equal(t, board, bound.Board, "recovery must retain the exact authorized subscription")
	replacement := bound.Message.(frontendEvents.InitialEvent)
	require.True(t, replacement.OwnPluginReady)
	require.False(t, replacement.ReadOnly)
	hub.retryPendingInitializations(t.Context())
	require.Empty(t, client.send)
	// A lost membership cannot regain authority through another plugin's readiness.
	source.available = false
	hub.retryPendingInitializations(t.Context())
	require.Len(t, client.send, 1)
	lost := (<-client.send).(frontendEvents.BoundMessage).Message.(frontendEvents.InitialEvent)
	require.False(t, lost.OwnPluginReady)
	require.True(t, lost.ReadOnly)
	hub.retryPendingInitializations(t.Context())
	require.Empty(t, client.send)
}

func (s *delayedBoardSource) IsSessionSynced(int32) bool                          { return s.synced }
func (s *delayedBoardSource) FrontendBoardReady(string, events.BoardContext) bool { return s.synced }

func TestSelectedBoardRecoversWhenSyncCompletesWithoutNotification(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	source := &delayedBoardSource{boardRecoverySource: boardRecoverySource{available: true}}
	source.HasActiveClientForAirportFn = func(string) bool { return true }
	hub.server.(*testutil.MockServer).EuroscopeHubVal = source
	board := events.BoardContext{SessionID: 42, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "tab"}
	client := startQueuedTestClient(&Client{hub: hub, session: WaitingForEuroscopeConnectionSessionId, user: validFrontendUser("1234567"), selection: membershipSelection{board: &board, session: 42, instance: "own", membership: "member"}})
	hub.clients = map[*Client]bool{client: true}
	hub.OnRegister(client)
	require.IsType(t, frontendEvents.DisconnectEvent{}, (<-client.send).(frontendEvents.BoundMessage).Message)
	hub.retryPendingInitializations(t.Context())
	require.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)
	require.Empty(t, client.send)
	source.synced = true
	hub.retryPendingInitializations(t.Context())
	bound := (<-client.send).(frontendEvents.BoundMessage)
	require.Equal(t, board, bound.Board)
	initial := bound.Message.(frontendEvents.InitialEvent)
	require.Equal(t, int32(42), initial.SessionID)
	require.True(t, initial.OwnPluginReady)
	require.False(t, initial.ReadOnly)
	hub.retryPendingInitializations(t.Context())
	require.Empty(t, client.send, "a successful initialization must not repeat on each retry tick")
}

type liveControllerHub struct {
	*testutil.MockEuroscopeHub
	controller *models.Controller
	unsynced   bool
}

func (h *liveControllerHub) IsSessionSynced(int32) bool { return !h.unsynced }

func (h *liveControllerHub) GetFrontendController(cid string) *models.Controller {
	if h.controller != nil && h.controller.Cid != nil && *h.controller.Cid == cid {
		return h.controller
	}
	return nil
}

func TestWaitingFrontendRecoversUsingLiveControllerWithoutOnlineNotification(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	server := hub.server.(*testutil.MockServer)
	cid := "1234567"
	live := &liveControllerHub{
		MockEuroscopeHub: &testutil.MockEuroscopeHub{HasActiveClientForAirportFn: func(string) bool { return true }},
	}
	server.EuroscopeHubVal = live
	server.ControllerRepoVal.(*testutil.MockControllerRepository).GetByCidFn = func(context.Context, string) (*models.Controller, error) {
		t.Fatal("must not use historical CID records when live connections are available")
		return nil, nil
	}
	client := startQueuedTestClient(&Client{hub: hub, session: WaitingForEuroscopeConnectionSessionId, user: validFrontendUser(cid)})
	hub.clients = map[*Client]bool{client: true}

	// A persisted CID alone must not assign a frontend while ES is offline.
	_, err := hub.frontendController(cid)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	hub.retryPendingInitializations(context.Background())
	require.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)

	live.controller = &models.Controller{Session: 42, Cid: &cid, Callsign: "EKCH_A_TWR", Position: "118.105"}
	live.unsynced = true
	hub.retryPendingInitializations(context.Background())
	require.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)
	live.unsynced = false
	hub.retryPendingInitializations(context.Background())
	require.Equal(t, int32(42), client.session)
	event, ok := waitForOutgoingMessage(t, client.send).(frontendEvents.InitialEvent)
	require.True(t, ok)
	require.Equal(t, "EKCH_A_TWR", event.Callsign)
	require.False(t, client.initialPending)

	hub.retryPendingInitializations(context.Background())
	select {
	case <-client.send:
		t.Fatal("successful initialization should not repeat")
	default:
	}
}

func TestInitialSnapshotRetriesAfterTransientRepositoryFailure(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	server := hub.server.(*testutil.MockServer)
	attempts := 0
	server.StripRepoVal.(*testutil.MockStripRepository).ListFn = func(context.Context, int32) ([]*models.Strip, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("temporary database failure")
		}
		return nil, nil
	}
	client := startQueuedTestClient(&Client{hub: hub, session: 42, airport: "EKCH", callsign: "EKCH_A_TWR", user: validFrontendUser("1234567")})
	hub.clients = map[*Client]bool{client: true}
	hub.OnRegister(client)
	require.True(t, client.initialPending)
	hub.retryPendingInitializations(context.Background())
	_, ok := waitForOutgoingMessage(t, client.send).(frontendEvents.InitialEvent)
	require.True(t, ok)
	require.False(t, client.initialPending)
	hub.retryPendingInitializations(context.Background())
	require.Equal(t, 2, attempts)
}

func TestSessionAssociationRejectsStaleSessionNotification(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	cid := "1234567"
	hub.server.(*testutil.MockServer).EuroscopeHubVal = &liveControllerHub{
		MockEuroscopeHub: &testutil.MockEuroscopeHub{},
		controller:       &models.Controller{Session: 42, Cid: &cid, Callsign: "EKCH_A_TWR"},
	}
	client := &Client{hub: hub, session: WaitingForEuroscopeConnectionSessionId, user: validFrontendUser(cid)}
	hub.clients = map[*Client]bool{client: true}
	require.Empty(t, hub.associateCidOnlineClients(cidOnlineMessage{session: 99, cid: cid}))
	require.False(t, hub.associateClientWithSession(client, 99, &models.Controller{Session: 99, Cid: &cid}, &models.Session{ID: 99}, false))
	require.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)
}
