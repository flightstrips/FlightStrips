package main

import (
	"encoding/json"
	"errors"
	"log"
)

func (s *Server) frontendeventhandlerGoARound(event Event) (err error) {
	var goAround GoAroundEventPayload
	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &goAround)
	if err != nil {
		log.Println("Error unmarshalling goAround event")
		return err
	}

	bEvent, err := json.Marshal(event)
	if err != nil {
		return err
	}

	//Go Around is an event to send to all FrontEndClients
	s.FrontendHub.broadcast <- bEvent

	return nil
}

func (s *Server) frontendeventhandlerStripUpdate(event Event) (err error) {
	// TODO
	return errors.New("not implemented")
}

