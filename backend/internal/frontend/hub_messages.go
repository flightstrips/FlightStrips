package frontend

import (
	"FlightStrips/internal/config"
	"FlightStrips/pkg/events/frontend"
	"sync/atomic"
)

func (hub *Hub) NextMessageID() int64 {
	return atomic.AddInt64(&hub.msgCounter, 1)
}

func (hub *Hub) storeMessage(sessionID int32, msg frontend.MessageReceivedEvent) {
	hub.msgMu.Lock()
	defer hub.msgMu.Unlock()
	msgs := hub.messages[sessionID]
	msgs = append([]frontend.MessageReceivedEvent{msg}, msgs...)
	if len(msgs) > 100 {
		msgs = msgs[:100]
	}
	hub.messages[sessionID] = msgs
}

func (hub *Hub) dispatchMessage(session int32, msg frontend.MessageReceivedEvent, senderCID string) {
	if msg.IsBroadcast {
		hub.Broadcast(session, msg)
		return
	}

	// Resolve area names → positions, find first active position per area
	areaMap := config.GetMessageAreas()
	recipientPositions := make(map[string]bool)
	for _, area := range msg.Recipients {
		positions, ok := areaMap[area]
		if !ok {
			continue
		}
		for _, pos := range positions {
			found := false
			for client := range hub.clients {
				if client.session == session && client.position == pos {
					recipientPositions[pos] = true
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	for client := range hub.clients {
		if client.session != session {
			continue
		}
		if client.user.GetCid() == senderCID || recipientPositions[client.position] {
			client.send <- msg
		}
	}
}
