package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

func (s *Server) _publishEvent(airport string, event Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Fatalf("Error marshalling event: %v", err)
		return err
	}
	// Broadcast the event to all frontend clients
	s.FrontendHub.broadcast <- eventBytes

	return nil
}

func (s *Server) _publishEventSpecificFrontEndClients(airport, position string, event Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Fatalf("Error marshalling event: %v", err)
		return err
	}

	// Send to clients in the specific position group
	group := fmt.Sprintf("position:%s", position)
	err = s.FrontendHub.SendToPosition(group, eventBytes)
	if err != nil {
		log.Printf("Error sending to group %s: %v", group, err)
	}

	return nil
}

func (s *Server) publishPositionOnlineEvent(airport, position string) error {
	// Build PositionOnline Event
	positionOnlineEvent := Event{
		Type:      PositionOnline,
		Airport:   airport,
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload: PositionOnlinePayload{
			Position: position,
		},
	}

	return s._publishEvent(airport, positionOnlineEvent)
}

func (s *Server) publishPositionOfflineEvent(airport, position string) error {
	// Build PositionOnline Event
	positionOfflineEvent := Event{
		Type:      PositionOffline,
		Airport:   airport,
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload: PositionOfflinePayload{
			Position: position,
		},
	}

	return s._publishEvent(airport, positionOfflineEvent)
}
