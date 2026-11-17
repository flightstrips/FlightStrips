package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

func TestActionReconciliationReturnsCurrentSubscriptionState(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	owner, route := "121.730", "DCT LANGO"
	now := time.Now()
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Strip, error) {
			require.Equal(t, int32(42), session)
			return []*models.Strip{{ID: 1, Session: session, Callsign: "EWG4TX", Owner: &owner, Route: &route, EuroscopeSeenAt: &now}}, nil
		},
	}
	board := events.BoardContext{SessionID: 42, Airport: "EKCH", SubscriptionID: "existing"}
	client := startQueuedTestClient(&Client{hub: hub, session: 42, airport: "EKCH", position: "121.630", callsign: "EKCH_A_GND",
		selection: membershipSelection{board: &board}, user: validFrontendUser("1662909")})
	t.Cleanup(func() { client.Close() })
	require.NoError(t, handleActionReconciliation(t.Context(), client, Message{Message: []byte(`{"request_id":"refresh-1"}`)}))
	outgoing := (<-client.send).(fe.BoundMessage)
	event := outgoing.Message.(fe.ActionReconciliationEvent)
	require.Equal(t, int32(42), event.SessionID)
	require.Equal(t, "refresh-1", event.RequestID)
	require.Len(t, event.Strips, 1)
	require.Equal(t, owner, event.Strips[0].Owner)
	require.Equal(t, route, event.Strips[0].Route)
	wire, err := outgoing.Marshal()
	require.NoError(t, err)
	var payload struct {
		Type  string              `json:"type"`
		Board events.BoardContext `json:"board"`
	}
	require.NoError(t, json.Unmarshal(wire, &payload))
	require.Equal(t, string(fe.ActionReconciliation), payload.Type)
	require.Equal(t, board, payload.Board)
}

func TestActionReconciliationDoesNotPublishFailedSnapshot(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) { return nil, errors.New("database unavailable") },
	}
	client := startQueuedTestClient(&Client{hub: hub, session: 42})
	t.Cleanup(func() { client.Close() })
	require.ErrorContains(t, handleActionReconciliation(t.Context(), client, Message{Message: []byte(`{"request_id":"refresh-1"}`)}), "database unavailable")
	require.Empty(t, client.send)
}

func TestActionReconciliationRejectsIncompleteCollections(t *testing.T) {
	for _, collection := range []string{"tactical strips", "stand assignments", "stand blocks"} {
		t.Run(collection, func(t *testing.T) {
			hub := newAMANInitialTestHub(t, nil)
			builder := hub.getSnapshotBuilder()
			readErr := errors.New("collection unavailable")
			var tacticalErr error
			stands := &standAssignmentSnapshotRepository{}
			switch collection {
			case "tactical strips":
				tacticalErr = readErr
			case "stand assignments":
				stands.assignmentsErr = readErr
			case "stand blocks":
				stands.blocksErr = readErr
			}
			builder.tacticalStripRepo = &testutil.MockTacticalStripRepository{
				ListBySessionFn: func(context.Context, int32) ([]*models.TacticalStrip, error) { return nil, tacticalErr },
			}
			builder.standAssignmentEnabled = true
			builder.standAssignmentRepo = stands
			// Initial connection keeps its existing best-effort contract.
			_, _, err := builder.Build(t.Context(), InitialSnapshotRequest{SessionID: 42, Airport: "EKCH"})
			require.NoError(t, err)
			client := startQueuedTestClient(&Client{hub: hub, session: 42, airport: "EKCH"})
			t.Cleanup(func() { client.Close() })
			request := Message{Message: []byte(`{"request_id":"refresh-1"}`)}
			require.ErrorIs(t, handleActionReconciliation(t.Context(), client, request), readErr)
			require.Empty(t, client.send, "an incomplete refresh must not erase existing client collections")
			// Clearing the read failure allows a normal refresh on the same client.
			tacticalErr, stands.assignmentsErr, stands.blocksErr = nil, nil, nil
			require.NoError(t, handleActionReconciliation(t.Context(), client, request))
			require.IsType(t, fe.ActionReconciliationEvent{}, waitForOutgoingMessage(t, client.send))
		})
	}
}

func TestActionReconciliationOrdersConcurrentOwnershipUpdates(t *testing.T) {
	for _, failSnapshot := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful snapshot", true: "failed snapshot"}[failSnapshot], func(t *testing.T) {
			hub := newAMANInitialTestHub(t, nil)
			var ownerMu sync.Mutex
			persistedOwner := "121.630"
			readStarted, resumeRead := make(chan struct{}), make(chan struct{})
			releaseRead := sync.OnceFunc(func() { close(resumeRead) })
			t.Cleanup(releaseRead)
			now := time.Now()
			hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
				ListFn: func(context.Context, int32) ([]*models.Strip, error) {
					ownerMu.Lock()
					ownerAtRead := persistedOwner
					ownerMu.Unlock()
					close(readStarted)
					<-resumeRead
					if failSnapshot {
						return nil, errors.New("snapshot unavailable")
					}
					return []*models.Strip{{ID: 1, Session: 42, Callsign: "EWG4TX", Owner: &ownerAtRead, EuroscopeSeenAt: &now}}, nil
				},
			}
			client := startQueuedTestClient(&Client{hub: hub, session: 42,
				send: make(chan events.OutgoingMessage, 4), user: validFrontendUser("1662909")})
			witness := startQueuedTestClient(&Client{session: 42,
				send: make(chan events.OutgoingMessage, 4), user: validFrontendUser("832365")})
			hub.send = make(chan internalMessage, 4)
			hub.clients = map[*Client]bool{client: true, witness: true}
			ctx, cancel := context.WithCancel(t.Context())
			hubDone := make(chan struct{})
			go func() { defer close(hubDone); hub.Run(ctx) }()
			t.Cleanup(func() { cancel(); <-hubDone; client.Close(); witness.Close() })
			result := make(chan error, 1)
			go func() {
				result <- handleActionReconciliation(ctx, client, Message{Message: []byte(`{"request_id":"refresh-1"}`)})
			}()
			select {
			case <-readStarted:
			case <-time.After(time.Second):
				t.Fatal("snapshot read did not start")
			}

			// Commit a new owner after the snapshot has read the old one. The
			// hub must keep serving other clients while this read is suspended.
			ownerMu.Lock()
			persistedOwner = "121.730"
			ownerMu.Unlock()
			update := fe.OwnersUpdateEvent{Callsign: "EWG4TX", Owner: "121.730"}
			hub.Broadcast(42, update)
			hub.Send(42, witness.GetCid(), fe.OwnersUpdateEvent{Callsign: "barrier"})
			require.Equal(t, update, waitForOutgoingMessage(t, witness.send))
			require.Equal(t, "barrier", waitForOutgoingMessage(t, witness.send).(fe.OwnersUpdateEvent).Callsign)
			// The direct barrier is dispatched after the broadcast's entire fanout.
			releaseRead()
			select {
			case err := <-result:
				if failSnapshot {
					require.ErrorContains(t, err, "snapshot unavailable")
				} else {
					require.NoError(t, err)
				}
			case <-time.After(time.Second):
				t.Fatal("reconciliation did not complete")
			}
			if !failSnapshot {
				message := waitForOutgoingMessage(t, client.send)
				require.IsType(t, fe.ActionReconciliationEvent{}, message, "snapshot must precede the concurrent ownership update")
				snapshot := message.(fe.ActionReconciliationEvent)
				require.Equal(t, "121.630", snapshot.Strips[0].Owner)
			}
			require.Equal(t, update, waitForOutgoingMessage(t, client.send), "the concurrent owner must follow any older snapshot")
			// Both success and failure must release the delivery fence.
			require.True(t, client.Enqueue(fe.OwnersUpdateEvent{Callsign: "later"}))
			require.Equal(t, "later", waitForOutgoingMessage(t, client.send).(fe.OwnersUpdateEvent).Callsign)
		})
	}
}

func TestActionReconciliationKeepsDeliveryBufferBounded(t *testing.T) {
	client := startQueuedTestClient(&Client{send: make(chan events.OutgoingMessage, 2)})
	t.Cleanup(func() { client.Close() })
	err := client.reconcile(func() (events.OutgoingMessage, error) {
		require.True(t, client.Enqueue(fe.OwnersUpdateEvent{Callsign: "first"}))
		require.True(t, client.Enqueue(fe.OwnersUpdateEvent{Callsign: "second"}))
		require.False(t, client.Enqueue(fe.OwnersUpdateEvent{Callsign: "overflow"}))
		return fe.ActionReconciliationEvent{}, nil
	})
	require.Error(t, err)
	select {
	case <-client.closed:
	default:
		t.Fatal("an overflowing reconciliation buffer must disconnect the slow consumer")
	}
}

func TestActionReconciliationKeepsAuthorityAndBoardFences(t *testing.T) {
	board := events.BoardContext{SessionID: 42, SubscriptionID: "existing"}
	client := startQueuedTestClient(&Client{selection: membershipSelection{board: &board}, session: 42})
	t.Cleanup(func() { client.Close() })
	handlers := shared.NewMessageHandlers[fe.EventType, *Client]()
	handlers.Add(fe.ActionReconciliation, handleActionReconciliation)
	wrong := board
	wrong.SubscriptionID = "old"
	require.ErrorContains(t, handlers.Handle(t.Context(), client, Message{Type: fe.ActionReconciliation, Board: &wrong}), "board subscription")
	ctx, err := shared.WithHumanCommand(t.Context(), &identityTestBinding{session: 99, cid: "1662909"})
	require.NoError(t, err)
	require.ErrorContains(t, handleActionReconciliation(ctx, client, Message{Message: []byte(`{"request_id":"refresh-1"}`)}), "another session")
	require.Empty(t, client.send)
}

func TestActionReconciliationDoesNotPublishAfterAuthorityIsLostDuringRead(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	binding := &reconciliationBinding{identityTestBinding: identityTestBinding{session: 42, cid: "1662909"}}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
		ListFn: func(context.Context, int32) ([]*models.Strip, error) {
			binding.revoked = true
			return nil, nil
		},
	}
	client := startQueuedTestClient(&Client{hub: hub, session: 42})
	t.Cleanup(func() { client.Close() })
	require.ErrorContains(t, handleActionReconciliation(ctx, client, Message{Message: []byte(`{"request_id":"refresh-1"}`)}), "revoked")
	require.Empty(t, client.send)
}

type reconciliationBinding struct {
	identityTestBinding
	revoked bool
}

func (b *reconciliationBinding) Validate() error {
	if b.revoked {
		return errors.New("binding revoked")
	}
	return nil
}

func TestRouteEditStillRejectsConcurrentOwnershipChange(t *testing.T) {
	owner := "121.730"
	route := "DCT LANGO"
	repo := &stripUpdateStoreFake{getByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
		return &models.Strip{Callsign: "EWG4TX", Owner: &owner}, nil
	}}
	sender := &rejectedRouteSender{}
	service := NewFrontendStripUpdateService(repo, nil, sender, nil, nil, nil, nil, nil)
	err := service.UpdateStrip(t.Context(), FrontendStripUpdateRequest{Session: 42, Cid: "1662909", Position: "121.630",
		Event: fe.UpdateStripDataEvent{Callsign: "EWG4TX", Route: &route}})
	require.ErrorContains(t, err, "non-owner")
	require.False(t, sender.sent)
}

type rejectedRouteSender struct {
	recordingStripUpdateEuroscopeSender
	sent bool
}

func (s *rejectedRouteSender) SendRoute(int32, string, string, string) { s.sent = true }
