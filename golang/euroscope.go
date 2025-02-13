package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gorilla/websocket"
)

type EuroscopeClient struct {
	conn      *websocket.Conn
	authToken *jwt.Token
	send      chan []byte
	token     string
}

// Global variables for managing clients.
var euroscopeClients = make(map[*EuroscopeClient]bool) // Map to track connected FrontEnd clients.
var euroscopeBroadcast = make(chan []byte)             // Channel for broadcasting messages for the FrontEnd.

func (s *Server) euroscopeEvents(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	defer conn.Close()

	// TODO: Handle this on which one is the master etc
	err = conn.WriteMessage(websocket.TextMessage, []byte("{\"type\": \"session_info\", \"role\": \"master\"}"))
	if err != nil {
		return
	}

	// go handleOutgoingMessages(client)
	client, success, err := s.euroscopeInitialEventsHandler(conn)
	if err != nil {
		log.Printf("Error handling initial events: %s \n", err)
		success = false
	}

	euroscopeClients[client] = true

	// Read incoming messages.
	if success {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				log.Println("read error (connection closed by remote?):", err)
				break
			}
			log.Printf("recv: %s", msg)

			var event EuroscopeEvent
			err = json.Unmarshal(msg, &event)
			if err != nil {
				log.Printf("Error unmarshalling event: %s \n", err)
				continue
			}

			eventOutput, err := s.euroscopeEventsHandler(client, event, msg)
			fmt.Printf("Event Output: %v", eventOutput)
			if event.Type == CloseConnection || event.Type == PositionOffline {
				log.Println("Closing connection")
				// break
			}
		}
	}

	/*
		err = s.euroscopeeventhandlerConnectionClosed(client)
		if err != nil {
			log.Printf("Error handling connection closed event: %s \n", err)
			return
		}
	*/
	delete(euroscopeClients, client)
	close(client.send)
}

func (s *Server) euroscopeInitialEventsHandler(conn *websocket.Conn) (client *EuroscopeClient, success bool, err error) {
	// Auth handling
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, false, err
	}

	token, err := s.euroscopeeventhandlerAuthentication(msg, s.AuthServerURL)
	if err != nil {
		return nil, false, err
	}
	// Login Handling

	_, msg, err = conn.ReadMessage()
	if err != nil {
		return nil, false, err
	}

	success, err = s.euroscopeeventhandlerLogin(msg)
	if err != nil {
		return nil, false, err
	}
	if !success {
		return nil, false, errors.New("login failed")
	}

	// Controller Online

	client = &EuroscopeClient{conn: conn, authToken: token, send: make(chan []byte)}
	return client, true, nil
}

func (s *Server) euroscopeEventsHandler(client *EuroscopeClient, event EuroscopeEvent, msg []byte) (output string, err error) {

	switch event.Type {
	case PositionOnline:
		return "", errors.New("not implemented")
	default:
		return "", errors.New("unknown event type")
	}
}
