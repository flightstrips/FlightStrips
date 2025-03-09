package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) frontEndEvents(w http.ResponseWriter, r *http.Request) {

	//TODO: Authenticate
	//TODO: Initial Information and message.
	//TODO: Logging per websocket? An ID or a prefix perhaps?

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	// Read initial message from client
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("read error:", err)
		conn.Close()
		return
	}

	// Handle the initial connection event and insert the controller into the database
	cid, airport, position, err := s.handleInitialConnectionEvent(msg)
	if err != nil {
		log.Printf("Error handling initial connection event: %s \n", err)
		conn.Close()
		return
	}
	// Return the initialConnectionEventPayload to the client
	err = s.returnInitialConnectionResponseEvent(conn, airport)
	if err != nil {
		log.Printf("Error returning initial connection response event: %s \n", err)
		conn.Close()
		return
	}
	// Create a new client instance.
	client := &FrontEndClient{conn: conn, send: make(chan []byte), cid: cid, airport: airport, position: position}
	frontEndClients[client] = true

	// Goroutine for outgoing messages.
	go handleOutgoingMessages(client)

	// Read incoming messages.
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read error (connection closed by remote?):", err)
			break
		}
		log.Printf("recv: %s", msg)
		var event Event
		err = json.Unmarshal(msg, &event)
		if err != nil {
			log.Printf("Error unmarshalling event: %s \n", err)
			continue
		}

		// TODO: SwitchCase for different types of messages
		eventOutput, err := s.frontEndEventHandler(*client, event)
		if err != nil {
			log.Printf("Error handling event: %s \n", err)
			return
		}
		fmt.Printf("Event Output: %v", eventOutput)
		if event.Type == CloseConnection || event.Type == PositionOffline {
			break
		}
	}

	// Cleanup when connection is closed.
	// TODO: Add removal of online controllers if the controller is also not seen in Euroscope?
	err = s.frontendeventhandlerControllerOffline(client)
	if err != nil {
		return
	}
	delete(frontEndClients, client)
	close(client.send)
}

func (s *Server) frontEndEventHandler(client FrontEndClient, event Event) (interface{}, error) {
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
		s.log("Go Around Event")
		// This event is sent to all FrontEndClients - No need for any backend parsing
		err := s.frontendeventhandlerGoARound(event)
		return nil, err
	case PositionOffline:
		s.log("Position Offline Event")
		// This event is sent to all FrontEndClients - No need for any backend parsing
		err := s.frontendeventhandlerControllerOffline(&client)
		return nil, err

	case Message:
		s.log("Message Event")
		// This event is sent to all OR specific FrontEndClients - No need for any backend parsing
		err := s._publishEvent(client.airport, event)
		return nil, err

	// TODO:
	case StripUpdate:
		s.log("Strip Update Event")

	case StripTransferRequestInit:
		s.log("Strip Transfer Request Init Event")
		print("Not Implemented")

	case StripTransferRequestReject:
		s.log("Strip Transfer Request Reject Event")
		print("Not Implemented")

	case StripMoveRequest:
		s.log("Strip Move Request Event")
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
	controllers, err := db.ListControllersByAirport(context.Background(), airport)
	if err != nil {
		return err
	}
	if controllers == nil {
		log.Printf("no controllers found for airport: %s\n", airport)
	}
	resp.Controllers = controllers

	// Get all Strips
	strips, err := db.ListStripsByOrigin(context.Background(), pgtype.Text{
		String: airport,
		Valid:  true,
	})
	if err != nil {
		return err
	}
	resp.Strips = strips

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

// TODO
func (s *Server) frontendeventhandlerCloseConnection(event Event) error {
	var controller Controller
	payload := event.Payload.(string)
	err := json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return err
	}

	removeControllerParams := controller.Cid

	db := data.New(s.DBPool)
	_, err = db.RemoveController(context.Background(), removeControllerParams)
	if err != nil {
		return err
	}
	return nil
}
