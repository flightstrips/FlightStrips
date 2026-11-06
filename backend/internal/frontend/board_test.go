package frontend

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
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
