package frontend

import (
	"FlightStrips/internal/config"
	"FlightStrips/pkg/events/frontend"
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
)

var messageAreaSectors = map[string]string{
	"CLR-DEL":   "DEL",
	"SEQ PLN":   "SQ",
	"APRON ARR": "AA",
	"APRON DEP": "AD",
	"GND WEST":  "GW",
	"GND EAST":  "GE",
	"TWR-ARR":   "TW",
	"TWR-DEP":   "TE",
}

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

	ownedSectors := make(map[string][]string)
	if hub.server != nil && hub.server.GetSectorOwnerRepository() != nil {
		sectors, err := hub.server.GetSectorOwnerRepository().ListBySession(context.Background(), session)
		if err != nil {
			slog.Error("Failed to load sector ownership for message recipients", slog.Int("session", int(session)), slog.Any("error", err))
		} else {
			for _, owner := range sectors {
				if owner != nil {
					ownedSectors[owner.Position] = owner.Sector
				}
			}
		}
	}

	senderPositions := make(map[string]bool)
	for client := range hub.clients {
		if client.session == session && client.user.GetCid() == senderCID {
			senderPositions[client.position] = true
		}
	}
	isCandidate := func(client *Client) bool {
		return client.session == session && !senderPositions[client.position] && !client.readOnly && !strings.HasSuffix(strings.ToUpper(client.callsign), "_OBS")
	}

	// Match composer availability: direct frequency first, then current sector coverage.
	areaMap := config.GetMessageAreas()
	recipientPositions := make(map[string]bool)
	for _, area := range msg.Recipients {
		positions, ok := areaMap[area]
		if !ok || len(positions) == 0 {
			slog.Warn("Unknown message recipient area", slog.String("area", area))
			continue
		}
		pos := positions[0]
		if configuredPosition, err := config.GetPositionByName(pos); err == nil {
			pos = configuredPosition.Frequency
		} else if _, err := config.GetPositionBasedOnFrequency(pos); err != nil {
			slog.Warn("Unknown message recipient position", slog.String("area", area), slog.String("position", pos))
			continue
		}
		found := false
		for client := range hub.clients {
			if isCandidate(client) && client.position == pos {
				recipientPositions[pos] = true
				found = true
				break
			}
		}
		if found {
			continue
		}
		sector, ok := messageAreaSectors[area]
		if !ok {
			slog.Warn("Unknown message recipient sector", slog.String("area", area))
			continue
		}
		for client := range hub.clients {
			if !isCandidate(client) {
				continue
			}
			for _, ownedSector := range ownedSectors[client.position] {
				if ownedSector == "GWA" || ownedSector == "GWD" {
					ownedSector = "GW"
				}
				if ownedSector == sector {
					recipientPositions[client.position] = true
					break
				}
			}
		}
	}

	for client := range hub.clients {
		if client.session != session {
			continue
		}
		if client.user.GetCid() == senderCID || (recipientPositions[client.position] && !client.readOnly && !strings.HasSuffix(strings.ToUpper(client.callsign), "_OBS")) {
			client.Enqueue(msg)
		}
	}
}
