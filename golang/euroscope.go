package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

type EuroscopeClient struct {
	conn  *websocket.Conn
	send  chan []byte
	token string
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

	// Read incoming messages.
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

		switch event.Type {
		default:
			s.log("Unknown event type")
		}

		/*
			err = conn.WriteMessage(websocket.TextMessage, []byte("{\"type\": \"unknown\"}"))
			if err != nil {
				return
			}
		*/
	}
}

func (s *Server) euroscopeEventsHandler(client EuroscopeClient, event Event) {

}
