package main

import (
	"encoding/json"
	"log"
	"time"
)

func (s *Server) _publishEvent(airport string, event Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Fatalf("Error marshalling event: %v", err)
		return err
	}
	// Broadcast the position coming online
	frontEndBroadcast <- eventBytes

	return nil
}

func (s *Server) _publishEventSpecificFrontEndClients(airport, position string, event Event) error {
	eventBytes, err := json.Marshal(event)
	if err != nil {
		log.Fatalf("Error marshalling event: %v", err)
		return err
	}

	for client := range frontEndClients {
		if client.airport == airport && client.position == position {
			client.send <- eventBytes
		}
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
