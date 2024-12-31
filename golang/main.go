package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "localhost:2994", "http service address")

var upgrader = websocket.Upgrader{} // use default options

// FrontEndClient structure.
type FrontEndClient struct {
	conn *websocket.Conn
	send chan []byte // Channel for outgoing messages.
}

// Global variables for managing clients.
var frontEndClients = make(map[*FrontEndClient]bool) // Map to track connected FrontEnd clients.
var frontEndBroadcast = make(chan []byte)            // Channel for broadcasting messages for the FrontEnd.

func frontEndEvents(w http.ResponseWriter, r *http.Request) {

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

	// Goroutine for outgoing messages.
	// This needs to be a function to also determine whether a message needs to be sent to euroscope?
	go handleOutgoingMessages(client)

	// Read incoming messages.
	for {
		// TODO: Validate messages?
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read error:", err)
			break
		}
		log.Printf("recv: %s", msg)

		// Broadcast the received message to all clients.
		frontEndBroadcast <- msg
	}

	// Cleanup when connection is closed.
	delete(frontEndClients, client)
	close(client.send)
}

// Goroutine to handle outgoing messages for each client.
func handleOutgoingMessages(client *FrontEndClient) {
	//TODO: Store Message somewhere?
	//How do we determine when a message is out of lifespan?
	for msg := range client.send {
		err := client.conn.WriteMessage(websocket.TextMessage, msg)
		if err != nil {
			log.Println("write error:", err)
			client.conn.Close()
			break
		}
	}
}

// Periodic server-side message example.
func periodicMessages() {
	for {
		time.Sleep(5 * time.Second)
		serealisedHeartbeatEvent := NewHeartBeatEvent("Server heartbeat").json()
		frontEndBroadcast <- []byte(serealisedHeartbeatEvent)
	}
}

// Broadcast messages to all clients.
func handleFrontEndBroadcast() {
	for {
		msg := <-frontEndBroadcast
		for client := range frontEndClients {
			client.send <- msg
		}
	}
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	//http.HandleFunc("/euroscopeEvents", euroscopeEvents)
	http.HandleFunc("/frontEndEvents", frontEndEvents)

	// Start background tasks.
	go handleFrontEndBroadcast()
	go periodicMessages()

	log.Fatal(http.ListenAndServe(*addr, nil))
}
