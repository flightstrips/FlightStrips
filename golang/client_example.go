package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	// Connect to the WebSocket server.
	serverAddr := "ws://localhost:2994/frontEndEvents"
	c, _, err := websocket.DefaultDialer.Dial(serverAddr, nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// Handle interrupt signal for cleanup.
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

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

	// Goroutine to send ping messages periodically.
	go func() {
		for {
			time.Sleep(10 * time.Second)
			err := c.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				log.Println("ping error:", err)
				return
			}
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
