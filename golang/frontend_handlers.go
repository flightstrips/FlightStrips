package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgtype"
	"log"
)

func (s *Server) frontendeventhandlerControllerOffline(client *FrontEndClient) error {
	// TODO: Var verification?

	// Obtain a list of the controllers at the airport from the database
	controllersAtAirport, err := data.New(s.DBPool).ListControllersByAirport(context.Background(), pgtype.Text{String: client.airport})
	if err != nil {
		log.Fatalf("Error getting controllers by airport: %v", err)
	}

	// Check to see if there is another controller online at that position
	otherControllerAtPosition := false
	for _, controller := range controllersAtAirport {
		if controller.Position.String == client.position {
			otherControllerAtPosition = true
		}
	}

	// If another controller is not online at that position then publish a PositionOffline event
	if !otherControllerAtPosition {
		err = s.publishPositionOfflineEvent(client.airport, client.position)
		if err != nil {
			log.Fatalf("Error publishing controller offline event: %v", err)
		}
	}

	// Remove the controller from the database
	db := data.New(s.DBPool)
	err = db.RemoveController(context.Background(), client.cid)
	if err != nil {
		log.Fatalf("Error removing controller from database: %v", err)
	}

	return nil
}

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
	frontEndBroadcast <- bEvent

	return nil
}

func (s *Server) frontendeventhandlerStripUpdate(event Event) (err error) {
	// TODO
	return errors.New("not implemented")
}

func (s *Server) frontendeventhandlerMessage(event Event) (err error) {
	var message MessageEventPayload
	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &message)
	if err != nil {
		log.Println("Error unmarshalling message")
	}

	switch message.TargetPosition {
	case "all":
		err = s._publishEvent(event.Airport, event)
	case "":
		log.Println("No target position specified")
	default:
		err = s._publishEventSpecificFrontEndClients(event.Airport, message.TargetPosition, event)
	}

	return errors.New("not implemented")
}
