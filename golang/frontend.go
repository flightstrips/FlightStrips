package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"log"
	"net/http"
	"time"
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
	cid, airport, err := s.handleInitialConnectionEvent(msg)
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
	client := &FrontEndClient{conn: conn, send: make(chan []byte), cid: cid}
	frontEndClients[client] = true

	// Goroutine for outgoing messages.
	// TODO: This needs to be a function to also determine whether a message needs to be sent to euroscope?
	go handleOutgoingMessages(client)

	// Read incoming messages.
	for {
		// TODO: Once the position is online is it worth adding the CID or positon to the client list information so we can take it offline if the connection fails?
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
		eventOutput, err := s.frontEndEventHandler(event)
		if err != nil {
			log.Printf("Error handling event: %s \n", err)
			return
		}
		fmt.Printf("Event Output: %v", eventOutput)

		// Broadcast the received message to all clients.
		// TODO: Decide whether this is the best case
		resp, ok := eventOutput.([]byte)
		if !ok {
			log.Fatal("Error casting eventOutput to byte")
		}

		frontEndBroadcast <- resp
	}

	// Cleanup when connection is closed.
	// TODO: Add removal of online controllers if the controller is also not seen in Euroscope?
	delete(frontEndClients, client)
	close(client.send)
}

func (s *Server) frontEndEventHandler(event Event) (interface{}, error) {
	// TODO: SwitchCase for different types of messages
	// TODO: Decide whether the responses are handled here or whether they are handled in the frontEndEvents function
	// TODO: In order for there to be non broadcasted messages it is just done here?
	switch event.Type {
	case CloseConnection:
		log.Println("Close Connection Event")
		err := s.frontendeventhandlerCloseconnection(event)
		if err != nil {
			return nil, err
		}
		response := []byte("Connection Closed")
		return response, nil

	case GoAround:
		log.Println("Go Around Event")
		// This event is sent to all FrontEndClients - No need for any backend parseing
		_, err := s.frontendeventhandlerGoARound(event)
		if err != nil {
			return nil, err
		}

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

func (s *Server) handleInitialConnectionEvent(msg []byte) (cid, airport string, err error) {
	// Unmarshall into event
	var initialConnectionEvent Event
	var initialConnectionEventPayload InitialConnectionEventPayload
	err = json.Unmarshal(msg, &initialConnectionEvent)
	if err != nil {
		log.Fatalf("Error unmarshalling initial connection event: %v", err)
		return "", "", err
	}
	// Check event type
	if initialConnectionEvent.Type != InitialConnection {
		log.Fatalf("Error: Initial Connection Event not received, instead recieved event of type: %v", initialConnectionEvent.Type)
		return "", "", err
	}

	// handle event payload
	eventPayload := initialConnectionEvent.Payload.(string)
	err = json.Unmarshal([]byte(eventPayload), &initialConnectionEventPayload)
	if err != nil {
		log.Fatalf("Error unmarshalling initial connection event payload: %v", err)
		return "", "", err
	}

	// Check the authentication of the event
	// TODO: Auth

	// Define insertable controller params
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
		return "", "", err
	}
	// Redundant Check
	if insertedController.Cid != initialConnectionEventPayload.CID {
		log.Fatalf("Error inserting controller into database: Cid mismatch")
		return "", "", err
	}

	// Broadcast the position coming online
	if err := s.publishControllerOnlineEvent(initialConnectionEventPayload.Airport, initialConnectionEventPayload.Position); err != nil {
		log.Fatalf("Error publishing controller online event: %v", err)
		return "", "", err
	}

	return initialConnectionEventPayload.CID, initialConnectionEventPayload.Airport, nil
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
	controllers, err := db.ListControllersByAirport(context.Background(), pgtype.Text{
		String: airport,
		Valid:  true,
	})
	if err != nil {
		return err
	}
	if controllers == nil {
		log.Println("No controllers found for airport: %s", airport)
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
func (s *Server) frontendeventhandlerCloseconnection(event Event) error {
	var controller Controller
	payload := event.Payload.(string)
	err := json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return err
	}

	removeControllerParams := controller.Cid

	db := data.New(s.DBPool)
	err = db.RemoveController(context.Background(), removeControllerParams)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) frontendeventhandlerGoARound(event Event) (resp interface{}, err error) {
	var goaround GoAroundEventPayload
	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &goaround)
	if err != nil {
		log.Println("Error unmarshalling goaround event")
		return nil, err
	}

	bEvent, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	//Go Around is an event to send to all FrontEndClients
	frontEndBroadcast <- bEvent

	return nil, nil
}

func (s *Server) publishControllerOnlineEvent(airport, position string) error {
	// Build PositionOnline Event
	positionOnlineEvent := Event{
		Type:      PositionOnline,
		Airport:   airport,
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload: PositionOnlinePayload{
			Airport:  airport,
			Position: position,
		},
	}

	positionOnlineEventBytes, err := json.Marshal(positionOnlineEvent)
	if err != nil {
		log.Fatalf("Error marshalling position online event: %v", err)
		return err
	}

	// Broadcast the position coming online
	frontEndBroadcast <- positionOnlineEventBytes

	return nil
}

func (s *Server) publishControllerOfflineEvent(airport, position string) error {
	return errors.New("not implemented")
}
