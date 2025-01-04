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
		s.frontEndEventHandler(event)

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
		err := s.frontendeventhandlerInitiateconnection(event)
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

func (s *Server) frontendeventhandlerInitiateconnection(event Event) error {
	var controller data.Controller
	payload := event.Payload.(string)
	err := json.Unmarshal([]byte(payload), &controller)
	if err != nil {
		log.Println("Error unmarshalling controller")
		return err
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

	select {
	case _ = <-resultsChan:
		return nil
	case err := <-errChan:
		return err
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout")
	}
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
