package main

import (
	"FlightStrips/data"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
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

	initialConnectionEventPayload, err := s.frontEndEventHandler(initialConnectionEvent)
	if err != nil {
		log.Printf("Error handling event: %s \n", err)
		return
	}
	fmt.Printf("Initial Event Output: %v", initialConnectionEventPayload)

	// Send the initial message back to the client.

	initialConnectionEvent = Event{
		Type:      InitialConnection,
		Airport:   "EKCH",
		Source:    "Server",
		TimeStamp: time.Now(),
		Payload:   initialConnectionEventPayload,
	}

	eventOutputBytes, err := json.Marshal(initialConnectionEvent)

	err = conn.WriteMessage(websocket.TextMessage, eventOutputBytes)
	if err != nil {
		return
	}

	// Goroutine for outgoing messages.
	// This needs to be a function to also determine whether a message needs to be sent to euroscope?
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
		eventOutput, err := s.frontEndEventHandler(event)
		if err != nil {
			log.Printf("Error handling event: %s \n", err)
			return
		}
		fmt.Printf("Event Output: %v", eventOutput)

		// Broadcast the received message to all clients.
		// TODO: Work on this
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
		response, err := s.frontendeventhandlerGoARound(event)
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
		Airport: sql.NullString{
			String: controller.Airport,
		},
		Position: sql.NullString{
			String: controller.Position,
		},
	}

	resultsChan := make(chan interface{})
	errChan := make(chan error)

	s.Jobs <- DBJob{
		Action: func(ctx context.Context, q *data.Queries) (interface{}, error) {
			err := q.InsertController(ctx, insertControllerParams)
			if err != nil {
				return nil, err
			}
			// Because InsertController does not return anything
			return nil, nil
		},
		Result: resultsChan,
		Err:    errChan,
	}

	// Insert a controller into the database
	select {
	case _ = <-resultsChan:
		break
	case err := <-errChan:
		return resp, err
	case <-time.After(5 * time.Second):
		return resp, fmt.Errorf("timeout")
	}
	//  Fetch all the initial connection event response data
	// Strips, Controllers & Runway configurations

	// Fetch all the Controllers
	controllersResultsChan := make(chan interface{})
	controllersErrChan := make(chan error)
	s.Jobs <- DBJob{
		Action: func(ctx context.Context, q *data.Queries) (interface{}, error) {
			controllers, err := q.ListControllersByAirport(ctx, sql.NullString{String: controller.Airport})
			if err != nil {
				return nil, err
			}
			return controllers, nil
		},
		Result: controllersResultsChan,
		Err:    controllersErrChan,
	}

	select {
	case resultControllers := <-controllersResultsChan:
		controllers, ok := resultControllers.([]data.Controller)
		if !ok {
			return resp, fmt.Errorf(dbFetchingControllersCastingErr)
		}
		resp.Controllers = controllers
	case err := <-controllersErrChan:
		return resp, err
	case <-time.After(5 * time.Second):
		return resp, fmt.Errorf(dbFetchingControllersTimeoutErr)
	}

	// Fetch all the Strips
	stripsResultsChan := make(chan interface{})
	stripsErrChan := make(chan error)
	s.Jobs <- DBJob{
		Action: func(ctx context.Context, q *data.Queries) (interface{}, error) {
			strips, err := q.ListStripsByOrigin(ctx, sql.NullString{String: controller.Airport})
			if err != nil {
				return nil, err
			}
			return strips, nil
		},
		Result: stripsResultsChan,
		Err:    stripsErrChan,
	}
	select {
	case resultsStrips := <-stripsResultsChan:
		strips, ok := resultsStrips.([]data.Strip)
		if !ok {
			return resp, fmt.Errorf(dbFetchingStripsCastingErr)
		}
		resp.Strips = strips

		break
	case err := <-stripsErrChan:
		return resp, err
	case <-time.After(5 * time.Second):
		return resp, fmt.Errorf(dbFetchingStripsTimeoutErr)
	}

	// TODO: Still to do is Airport Configurations
	// TODO: Send a PositionOnline message to all other FrontEndClients
	return resp, nil
}

func (s *Server) frontendeventhandlerCloseconnection(event Event) error {
	var controller data.Controller
	payload := event.Payload.(string)
	err := json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return err
	}

	removeControllerParams := controller.Cid

	resultsChan := make(chan interface{})
	errChan := make(chan error)

	s.Jobs <- DBJob{
		Action: func(ctx context.Context, q *data.Queries) (interface{}, error) {
			err := q.RemoveController(ctx, removeControllerParams)
			if err != nil {
				return nil, err
			}
			// Because RemoveController does not return anything
			return nil, nil
		},
		Result: resultsChan,
		Err:    errChan,
	}

	select {
	case _ = <-resultsChan:
		return nil
	case err := <-errChan:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout")
	}
}
