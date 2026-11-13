package euroscope

import (
	"context"

	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
)

type aircraftSelectionState struct {
	callsign string
	revision uint64
}

func (c *Client) currentAircraftSelection() *fe.AircraftSelection {
	c.aircraftSelectionMu.Lock()
	defer c.aircraftSelectionMu.Unlock()
	return c.aircraftSelection.snapshot()
}

func (s *aircraftSelectionState) snapshot() *fe.AircraftSelection {
	if s == nil {
		return nil
	}
	selection := &fe.AircraftSelection{Revision: s.revision}
	if s.callsign != "" {
		callsign := s.callsign
		selection.Callsign = &callsign
	}
	return selection
}

// CurrentAircraftSelection never falls back to another instance or membership.
func (hub *Hub) CurrentAircraftSelection(cid string, board events.BoardContext) *fe.AircraftSelection {
	member := hub.frontendBoardMember(cid, board)
	if member == nil {
		return nil
	}
	return member.currentAircraftSelection()
}

func handleAircraftSelection(_ context.Context, client *Client, message Message) error {
	var event es.AircraftSelectionEvent
	if err := message.ProtoUnmarshal(&event); err != nil {
		return err
	}
	// This report is meaningful only for a scoped physical instance. Inbound
	// dispatch has already checked authentication and the complete incarnation.
	if client.membership == nil {
		return nil
	}
	client.aircraftSelectionMu.Lock()
	if client.aircraftSelection == nil {
		client.aircraftSelection = &aircraftSelectionState{}
	}
	client.aircraftSelection.callsign = event.Callsign
	client.aircraftSelection.revision++
	selection := client.aircraftSelection.snapshot()
	client.aircraftSelectionMu.Unlock()
	if target, ok := client.hub.server.GetFrontendHub().(interface {
		SendToPluginMembership(string, events.BoardContext, fe.OutgoingMessage)
	}); ok {
		scope := client.membership
		target.SendToPluginMembership(client.GetCid(), events.BoardContext{
			SessionID: scope.SessionId, InstanceID: scope.InstanceId,
			MembershipID: scope.MembershipId, Generation: scope.Generation,
			TransportGeneration: scope.TransportGeneration,
		}, fe.AircraftSelectionEvent{AircraftSelection: *selection})
	}
	return nil
}
