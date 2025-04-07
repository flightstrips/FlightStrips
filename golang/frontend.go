package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) frontEndEventHandler(client *FrontendClient, event Event) (interface{}, error) {
	// TODO: SwitchCase for different types of messages
	// TODO: Decide whether the responses are handled here or whether they are handled in the frontEndEvents function <- so far it is inside the handler.
	// TODO: In order for there to be non broadcast messages it is just done here?
	switch event.Type {
	/*
		case CloseConnection:
			log.Println("Close Connection Event")
			err := s.frontendeventhandlerCloseConnection(event)
			if err != nil {
				return nil, err
			}
			response := []byte("Connection Closed")
			return response, nil
	*/
	case GoAround:
		// This event is sent to all FrontEndClients - No need for any backend parsing
		err := s.frontendeventhandlerGoARound(event)
		return nil, err
	case PositionOffline:
		// This event is sent to all FrontEndClients - No need for any backend parsing
		err := s.frontendeventhandlerControllerOffline(client)
		return nil, err

	case Message:
		// This event is sent to all OR specific FrontEndClients - No need for any backend parsing
		err := s._publishEvent(client.GetAirport(), event)
		return nil, err

	// TODO:
	case StripUpdate:
		print("Not Implemented")

	case StripTransferRequestInit:
		print("Not Implemented")

	case StripTransferRequestReject:
		print("Not Implemented")

	case StripMoveRequest:
		print("Not Implemented")

	default:
		log.Println("Unknown Event Type")
		response := []byte("Not sure what to do here - Unknown EventType Handler")
		return response, nil
	}
	// TODO: Other Events

	// TODO: Better managing Return responses.

	// TODO: Not sure what to do here.
	return nil, nil
}

func (s *Server) handleInitialConnectionEvent(msg []byte) (cid, airport, position string, err error) {
	// Unmarshall into event
	var initialConnectionEvent Event
	var initialConnectionEventPayload InitialConnectionEventPayload
	err = json.Unmarshal(msg, &initialConnectionEvent)
	if err != nil {
		log.Fatalf("Error unmarshalling initial connection event: %v", err)
		return "", "", "", err
	}
	// Check event type
	if initialConnectionEvent.Type != InitialConnection {
		log.Fatalf("Error: Initial Connection Event not received, instead recieved event of type: %v", initialConnectionEvent.Type)
		return "", "", "", err
	}

	// handle event payload
	eventPayload := initialConnectionEvent.Payload.(string)
	err = json.Unmarshal([]byte(eventPayload), &initialConnectionEventPayload)
	if err != nil {
		log.Fatalf("Error unmarshalling initial connection event payload: %v", err)
		return "", "", "", err
	}

	// Check the authentication of the event
	// TODO: Auth

	// Define insertable controller params
	/*
		insertControllerParams := data.InsertControllerParams{
			Cid: initialConnectionEventPayload.CID,
			Airport: pgtype.Text{
				String: initialConnectionEventPayload.Airport,
				Valid:  true,
			},
			Position: pgtype.Text{
				String: initialConnectionEventPayload.Position,
				Valid:  true,
			},
		}

		// Insert into database
		db := data.New(s.DBPool)
		insertedController, err := db.InsertController(context.Background(), insertControllerParams)
		if err != nil {
			log.Fatalf("Error inserting controller into database: %v", err)
			return "", "", "", err
		}
		// Redundant Check
		if insertedController.Cid != initialConnectionEventPayload.CID {
			log.Fatalf("Error inserting controller into database: Cid mismatch")
			return "", "", "", err
		}

		// Broadcast the position coming online
		if err := s.publishPositionOnlineEvent(initialConnectionEventPayload.Airport, initialConnectionEventPayload.Position); err != nil {
			log.Fatalf("Error publishing controller online event: %v", err)
			return "", "", "", err
		}
	*/

	return initialConnectionEventPayload.CID, initialConnectionEventPayload.Airport, initialConnectionEventPayload.Position, nil
}

func (s *Server) returnInitialConnectionResponseEvent(conn *websocket.Conn, airport string) error {
	if conn == nil {
		return errors.New("returning initial connection response event parser failed on null connection")
	}
	if airport == "" {
		return errors.New("returning initial connection response event parser failed on empty airport")
	}

	db := data.New(s.DBPool)
	var resp InitialConnectionEventResponsePayload

	// Get all Controllers
	// TODO session
	controllers, err := db.ListControllers(context.Background(), 1)
	if err != nil {
		return err
	}
	if controllers == nil {
		log.Printf("no controllers found for airport: %s\n", airport)
	}
	resp.Controllers = controllers

	// Get all Strips
	// TODO: session

	// Get all Airport Configurations
	// TODO:

	// Build the event
	initialConnectionEvent := Event{
		Type:      InitialConnection,
		Source:    "FlightStrips",
		Airport:   airport,
		TimeStamp: time.Now(),
		Payload:   resp,
	}
	var initialConnectionEventBytes []byte
	initialConnectionEventBytes, err = json.Marshal(initialConnectionEvent)
	if err != nil {
		return err
	}

	// Send the event
	err = conn.WriteMessage(websocket.TextMessage, initialConnectionEventBytes)
	if err != nil {
		return err
	}

	log.Printf("Initial Connection Response Event sent to client: %s", initialConnectionEventBytes)

	return nil
}
