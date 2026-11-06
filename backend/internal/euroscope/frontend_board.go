package euroscope

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
)

func boardMembership(b events.BoardContext) *es.MembershipContext {
	return &es.MembershipContext{SessionId: b.SessionID, Airport: b.Airport, Environment: b.Environment, InstanceId: b.InstanceID, MembershipId: b.MembershipID, TransportGeneration: b.TransportGeneration, Generation: b.Generation}
}
func (hub *Hub) frontendBoardMember(cid string, board events.BoardContext) *Client {
	if !board.Complete() {
		return nil
	}
	for _, c := range hub.clientsSnapshot() {
		if c.GetCid() == cid && c.IsAuthenticated() && c.matches(boardMembership(board)) && c.definition != nil && c.definition.Airport() == board.Airport && c.definition.Revision() == board.DefinitionRevision {
			return c
		}
	}
	return nil
}

// AuthorizeFrontendBoard uses the MA-03 live registry. No historical controller
// lookup or another plugin's master role can authorize a browser subscription.
func (hub *Hub) AuthorizeFrontendBoard(cid string, board events.BoardContext) (*models.Controller, fe.BoardDefinition, bool, error) {
	c := hub.frontendBoardMember(cid, board)
	if c == nil {
		return nil, fe.BoardDefinition{}, true, shared.RejectAction("selected own membership is unavailable or stale")
	}
	identity := c.identitySnapshot()
	return &models.Controller{Session: c.session, Callsign: identity.callsign, Position: identity.position, Observer: identity.observer, Cid: &cid}, c.boardDefinition(), identity.observer || !hub.MembershipReady(c), nil
}
func (hub *Hub) CaptureFrontendBoardBinding(cid string, board events.BoardContext) (shared.CommandBinding, error) {
	member := hub.frontendBoardMember(cid, board)
	if member == nil {
		return nil, shared.RejectAction("selected own membership is unavailable or stale")
	}
	binding := &commandBinding{hub: hub, client: member, session: board.SessionID, cid: cid, requireReady: true}
	return binding, binding.Validate()
}

func (hub *Hub) FrontendBoardRuntimeState(cid string, board events.BoardContext) (bool, bool, string) {
	member := hub.frontendBoardMember(cid, board)
	if member == nil {
		return false, false, ""
	}
	hub.runwayStateMu.RLock()
	state := hub.runwayStates[runwayStateKey(board.SessionID, board.MembershipID)]
	departure, arrival := false, false
	if state != nil && state.Client == member {
		departure, arrival = state.DepartureMismatch, state.ArrivalMismatch
	}
	hub.runwayStateMu.RUnlock()
	return departure, arrival, member.identitySnapshot().localIP
}

func sendFrontendRunwayConfiguration(target shared.FrontendHub, member *Client, session int32, cid string, event fe.RunwayConfigurationEvent) {
	if member != nil && member.membership != nil {
		if scoped, ok := target.(interface {
			SendToPluginMembership(string, events.BoardContext, fe.OutgoingMessage)
		}); ok {
			scope := member.wireContext()
			scoped.SendToPluginMembership(cid, events.BoardContext{SessionID: session, InstanceID: scope.InstanceId, MembershipID: scope.MembershipId, Generation: scope.Generation, TransportGeneration: scope.TransportGeneration}, event)
			return
		}
	}
	target.Send(session, cid, event)
}
