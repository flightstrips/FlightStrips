package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

func (s *Server) frontEndEvents(w http.ResponseWriter, r *http.Request) {

	//TODO: Authenticate
	//TODO: Initial Information and message.
	//TODO:

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
	// TODO: Check payload is an Initiate Connection Payload
	var initialConnectionEvent Event
	err = json.Unmarshal(msg, &initialConnectionEvent)
	if err != nil {
		log.Println("Error unmarshalling initial connection event")
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
			return
		}
		fmt.Printf("Event Output: %v", eventOutput)

		// Broadcast the received message to all clients.
		// TODO: Work on this
		frontEndBroadcast <- msg
	}

	// Cleanup when connection is closed.
	delete(frontEndClients, client)
	close(client.send)
}

func (s *Server) frontEndEventHandler(event Event) (interface{}, error) {

	// TODO: SwitchCase for different types of messages
	switch event.Type {
	// Insert Controller Event
	case InitiateConnection:
		_, err := s.frontendeventhandlerInitiateconnection(event)
		if err != nil {
			return nil, err
		}
	case CloseConnection:
		err := s.frontendeventhandlerCloseconnection(event)
		if err != nil {
			return nil, err
		}
	}

	// TODO: Not sure what to do here.
	return nil, nil
}

func (s *Server) frontendeventhandlerInitiateconnection(event Event) (resp InitialConnectionEventResponsePayload, err error) {
	var controller data.Controller
	resp = InitialConnectionEventResponsePayload{}

	payload := event.Payload.(string)
	err = json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return resp, err
	}

	insertControllerParams := data.InsertControllerParams{
		Cid:      controller.Cid,
		Airport:  controller.Airport,
		Position: controller.Position,
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
			controllers, err := q.ListControllersByAirport(ctx, controller.Airport)
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
			strips, err := q.ListStripsByOrigin(ctx, controller.Airport)
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

	//TODO: Still to do is Airport Configurations
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
