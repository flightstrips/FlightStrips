package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"log"
	"os"
	"os/signal"
	"time"
)

type Controller struct {
	Cid      string
	Airport  string
	Position string
}

type Event struct {
	Type      string
	Airport   string
	Source    string
	TimeStamp time.Time
	Payload   interface{}
}

func main() {
	// Connect to the WebSocket server.
	serverAddr := "ws://localhost:2994/frontEndEvents"
	c, _, err := websocket.DefaultDialer.Dial(serverAddr, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	controller := Controller{
		Cid:      "1",
		Airport:  "EKCH",
		Position: "EKCH_W_APP",
	}
	marshal, err := json.Marshal(controller)
	if err != nil {
		return
	}

	initialEvent := Event{
		Type:      "initial_connection",
		Airport:   "EKCH",
		TimeStamp: time.Now(),
		Source:    "Client",
		Payload:   string(marshal),
	}

	initialEventJson, err := json.Marshal(initialEvent)

	// Handle interrupt signal for cleanup.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	err = c.WriteMessage(websocket.TextMessage, initialEventJson)
	if err != nil {
		return
	}

	// Goroutine to listen for messages from the server.
	go func() {
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read error:", err)
				return
			}
			fmt.Printf("Server: %s\n", message)
		}
	}()

	// Main loop for reading input from the user and sending it to the server.
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Enter messages to send to the server. Type 'exit' to quit.")
	for scanner.Scan() {
		text := scanner.Text()
		if text == "exit" {
			break
		}
		err := c.WriteMessage(websocket.TextMessage, []byte(text))
		if err != nil {
			log.Println("write error:", err)
			break
		}
	}

	// Handle graceful shutdown.
	fmt.Println("Closing connection...")
	err = c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	if err != nil {
		log.Println("close error:", err)
	}
}
