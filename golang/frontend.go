package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
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

	// Create a new client instance.
	client := &FrontEndClient{conn: conn, send: make(chan []byte)}
	frontEndClients[client] = true

	// Read initial message from client
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("read error:", err)
		delete(frontEndClients, client)
		conn.Close()
		return
	}
	// TODO: Check payload is an Initial Connection Payload
	var initialConnectionEvent Event
	err = json.Unmarshal(msg, &initialConnectionEvent)
	if err != nil {
		log.Printf("Error unmarshalling initial connection event: %s \n\n", msg)

		delete(frontEndClients, client)
		conn.Close()
		return
	}
	if initialConnectionEvent.Type != InitialConnection {
		log.Println("Error: Initial Connection Event not received")
		delete(frontEndClients, client)
		conn.Close()
		return
	}

	//TODO: Auth

	log.Printf("recv: %s", msg)

	initialConnectionEventResponsePayload, err := s.frontEndEventHandler(initialConnectionEvent)
	if err != nil {
		log.Printf("Error handling event: %s \n", err)
		return
	}
	fmt.Printf("Initial Event Output: %v", initialConnectionEventResponsePayload)

	var initialConnectionEventPayload Controller
	err = json.Unmarshal([]byte(initialConnectionEvent.Payload.(string)), &initialConnectionEventPayload)
	if err != nil {
		log.Println("Error unmarshalling initial connection event payload")
		return
	}

	initialConnectionEventReturn := Event{
		Type:      InitialConnection,
		Airport:   initialConnectionEventPayload.Airport,
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload:   initialConnectionEventPayload.Position,
	}

	eventOutputBytes, err := json.Marshal(initialConnectionEventReturn)
	err = conn.WriteMessage(websocket.TextMessage, eventOutputBytes)
	if err != nil {
		return
	}

	PositionOnlineEvent := Event{
		Type:      PositionOnline,
		Airport:   initialConnectionEvent.Airport,
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload: PositionOnlinePayload{
			Airport:  initialConnectionEventPayload.Airport,
			Position: initialConnectionEventPayload.Position,
		},
	}

	positionOnlineEventBytes, err := json.Marshal(PositionOnlineEvent)
	if err != nil {
		return
	}

	frontEndBroadcast <- positionOnlineEventBytes

	// Goroutine for outgoing messages.
	// This needs to be a function to also determine whether a message needs to be sent to euroscope?
	go handleOutgoingMessages(client)

	// Read incoming messages.
	for {
		// Once the position is online is it worth adding the CID or positon to the client list information so we can take it offline if the connection fails?
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
	// In order for there to be non broadcasted messages it is just done here?
	switch event.Type {
	// Insert Controller Event
	case InitialConnection:
		log.Println("Initial Connection Event")
		response, err := s.frontendeventhandlerInitialconnection(event)
		if err != nil {
			return nil, err
		}
		return response, nil
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
		_, err := s.frontendeventhandlerGoARound(event)
		if err != nil {
			return nil, err
		}

		frontEndBroadcast <- []byte("Go Around!")

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

func (s *Server) frontendeventhandlerInitialconnection(event Event) (resp InitialConnectionEventResponsePayload, err error) {
	var controller Controller
	resp = InitialConnectionEventResponsePayload{}

	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return resp, err
	}

	insertControllerParams := data.InsertControllerParams{
		Cid: controller.Cid,
		Airport: pgtype.Text{
			String: controller.Airport,
			Valid:  true,
		},
		Position: pgtype.Text{
			String: controller.Position,
			Valid:  true,
		},
	}

	db := data.New(s.DBPool)
	_, err = db.InsertController(context.Background(), insertControllerParams)
	if err != nil {
		return resp, nil
	}

	//  Fetch all the initial connection event response data
	// Strips, Controllers & Runway configurations

	// Fetch all the Controllers
	db = data.New(s.DBPool)
	controllers, err := db.ListControllers(context.Background())
	if err != nil {
		return resp, err
	}
	resp.Controllers = controllers

	// Fetch all the Strips
	db = data.New(s.DBPool)
	strips, err := db.ListStripsByOrigin(context.Background(), pgtype.Text{String: controller.Airport, Valid: true})
	if err != nil {
		return resp, err
	}
	resp.Strips = strips

	// TODO: Still to do is Airport Configurations
	// TODO: Send a PositionOnline message to all other FrontEndClients
	log.Printf("Initial Connection Event Response: %v", resp)
	return resp, nil
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
