package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBoardDispatchRejectsEveryMutationSurfaceBeforeBusinessHandling(t *testing.T) {
	board := events.BoardContext{SessionID: 1, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "subscription"}
	c := startQueuedTestClient(&Client{selection: membershipSelection{board: &board, session: 1}, session: 1})
	handlers := shared.NewMessageHandlers[fe.EventType, *Client]()
	// These are the controller mutation families, including non-strip writes and
	// deferred/service work. The shared transport fence also protects new actions.
	actions := []fe.EventType{fe.Move, fe.UpdateOrder, fe.UpdateStripData, fe.Marked, fe.StartReq, fe.CdmReady, fe.CdmDeicePlatformUpdate, fe.IssuePdcClearance, fe.RevertToVoice, fe.SendMessage, fe.SendPrivateMessage, fe.SendFrequencyMessage, fe.UpdateRunwayStatus, fe.GenerateSquawk, fe.CoordinationTransferRequestType, fe.CoordinationAssumeRequestType, fe.CoordinationForceAssumeRequestType, fe.AcknowledgeValidationStatus, fe.ClxOverrideValidation, fe.ClxUpdateTobt, fe.ActionStandOccupy, fe.ActionStandVacate, fe.ActionStandManualRequest, fe.ActionCreateManualFPL, fe.ActionCreateVFRFPL, fe.ActionCreateTacticalStrip, fe.ActionDeleteTacticalStrip, fe.AMANSetHoldingEATWritebackType}
	calls := 0
	for _, action := range actions {
		handlers.Add(action, func(context.Context, *Client, Message) error { calls++; return nil })
	}
	for _, action := range actions {
		for _, change := range []func(*events.BoardContext){func(b *events.BoardContext) { b.SessionID++ }, func(b *events.BoardContext) { b.Environment = "SWEATBOX_test" }, func(b *events.BoardContext) { b.SubscriptionID = "old" }, func(b *events.BoardContext) { b.Generation++ }} {
			wrong := board
			change(&wrong)
			err := handlers.Handle(t.Context(), c, Message{Type: action, Board: &wrong})
			require.ErrorContains(t, err, "board subscription", string(action))
		}
		require.Error(t, handlers.Handle(t.Context(), c, Message{Type: action}))
	}
	require.Zero(t, calls)
	c.Close()
	require.Error(t, handlers.Handle(t.Context(), c, Message{Type: fe.Move, Board: &board}))
}
func TestBoundEventsCarrySubscriptionIncludingSnapshotAndActionResults(t *testing.T) {
	board := events.BoardContext{SessionID: 1, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "tab"}
	c := startQueuedTestClient(&Client{selection: membershipSelection{board: &board}})
	for _, event := range []events.OutgoingMessage{fe.InitialEvent{SessionID: 1}, fe.DisconnectEvent{ReadOnly: true}, fe.ActionRejectedEvent{Action: "move", Reason: "stale"}, fe.FrequencyMessageResultEvent{RequestID: "request"}} {
		require.True(t, c.Enqueue(event))
		data, err := (<-c.send).Marshal()
		require.NoError(t, err)
		var decoded struct {
			Board events.BoardContext `json:"board"`
		}
		require.NoError(t, json.Unmarshal(data, &decoded))
		require.Equal(t, board, decoded.Board)
	}
}

type boardRecoverySource struct {
	testutil.MockEuroscopeHub
	available bool
}

func (s *boardRecoverySource) AuthorizeFrontendBoard(cid string, b events.BoardContext) (*models.Controller, fe.BoardDefinition, bool, error) {
	if !s.available {
		return nil, fe.BoardDefinition{}, true, errors.New("own plugin unavailable")
	}
	return &models.Controller{Session: 42, Callsign: "EKCH_A_TWR", Position: "118.105"}, fe.BoardDefinition{DefinitionRevision: b.DefinitionRevision, View: "ekch", Actions: []string{"strip.move"}}, false, nil
}
func (s *boardRecoverySource) CaptureFrontendBoardBinding(cid string, b events.BoardContext) (shared.CommandBinding, error) {
	if !s.available {
		return nil, errors.New("own plugin unavailable")
	}
	return &identityTestBinding{session: b.SessionID, cid: cid}, nil
}
func TestSelectedBoardRetainsReadOnlySubscriptionAndResnapshotsAfterOwnLoss(t *testing.T) {
	hub := newAMANInitialTestHub(t, nil)
	source := &boardRecoverySource{available: true}
	source.HasActiveClientForAirportFn = func(string) bool { return true } // another master stays online
	hub.server.(*testutil.MockServer).EuroscopeHubVal = source
	board := events.BoardContext{SessionID: 42, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "member", TransportGeneration: 1, Generation: 2, DefinitionRevision: "revision", SubscriptionID: "tab"}
	c := startQueuedTestClient(&Client{hub: hub, selection: membershipSelection{board: &board, session: 42, instance: "own", membership: "member"}, session: 42, sessionName: "LIVE", airport: "EKCH", callsign: "EKCH_A_TWR", position: "118.105", user: validFrontendUser("1234567")})
	hub.clients = map[*Client]bool{c: true}
	hub.OnRegister(c)
	initial := (<-c.send).(fe.BoundMessage).Message.(fe.InitialEvent)
	require.False(t, initial.ReadOnly)
	source.available = false
	hub.handlePluginMembershipDisconnected(cidDisconnectMessage{cid: "1234567", session: 42, instance: "own", membership: "member"})
	require.Equal(t, int32(42), c.GetSession())
	require.True(t, c.identitySnapshot().readOnly)
	require.IsType(t, fe.DisconnectEvent{}, (<-c.send).(fe.BoundMessage).Message)
	require.True(t, c.initialPending, "existing retry must reconcile denied optimistic data")
	hub.retryPendingInitializations(t.Context())
	replacement := (<-c.send).(fe.BoundMessage).Message.(fe.InitialEvent)
	require.True(t, replacement.ReadOnly)
	require.False(t, replacement.OwnPluginReady)
	require.Equal(t, initial.Definition, replacement.Definition)
	require.Equal(t, int32(42), replacement.SessionID)
	require.False(t, c.initialPending)
	_, err := c.PrepareMessageContext(t.Context(), "marked")
	require.Error(t, err)
	_, err = hub.HandleNewConnection(nil, validFrontendUser("1234567"), events.AuthenticationEvent{Board: &board})
	require.Error(t, err, "a new socket cannot claim a removed membership even while an existing authorized board is retained")
}
