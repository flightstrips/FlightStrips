package main

import (
	"errors"
	"log"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

type WebsocketClient interface {
	comparable
	// Core methods
	Send(message []byte) error
	Close() error

	// Client identification
	GetCid() string
	GetAirport() string
	GetPosition() string
	GetConnection() *websocket.Conn

	IsAuthenticated() bool
	SetUser(user ClientUser)

	HandlePong() error
	HandleMessage(message []byte) error

	GetSendChannel() chan []byte
}

type ClientUser struct {
	cid    string
	rating int
	token  *jwt.Token
}

type BaseWebsocketClient struct {
	server  *Server
	send    chan []byte
	conn    *websocket.Conn
	session int32
	user    *ClientUser

	position string
	airport  string
}

func (c *BaseWebsocketClient) Send(message []byte) error {
	c.send <- message
	return nil
}

func (c *BaseWebsocketClient) Close() error {
	return c.conn.Close()
}

func (c *BaseWebsocketClient) GetCid() string {
	return c.user.cid
}

func (c *BaseWebsocketClient) GetAirport() string {
	return c.airport
}

func (c *BaseWebsocketClient) GetPosition() string {
	return c.position
}

func (c *BaseWebsocketClient) GetConnection() *websocket.Conn {
	return c.conn
}

func (c *BaseWebsocketClient) IsAuthenticated() bool {
	if c.user == nil {
		return false
	}

	// TODO fix this is not correct as it does not check expiration
	return c.user.token.Valid
}

func (c *BaseWebsocketClient) GetSendChannel() chan []byte {
	return c.send
}

func (c *BaseWebsocketClient) SetUser(user ClientUser) {
	c.user = &user
}

func (c *BaseWebsocketClient) HandlePong() error {
	switch {

	}
	return errors.New("pong: not implemented")
}

func (c *BaseWebsocketClient) HandleMessage(message []byte) error {
	return errors.New("message: not implemented")
}

// readPump pumps messages from the WebSocket connection to the hub.
func ReadPump[TClient WebsocketClient, THub Hub[TClient]](hub THub, client TClient) {
	log.Println("ReadPump")
	defer func() {
		hub.Unregister(client)
		client.GetConnection().Close()
	}()

	client.GetConnection().SetReadDeadline(time.Now().Add(pongWait))
	client.GetConnection().SetPongHandler(func(string) error {
		client.GetConnection().SetReadDeadline(time.Now().Add(pongWait))
		return client.HandlePong()
	})

	for {
		_, message, err := client.GetConnection().ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		err = client.HandleMessage(message)

		if err != nil {
			log.Println("Failed to handle message", err)
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func WritePump[TClient WebsocketClient, THub Hub[TClient]](hub THub, client TClient) {
	log.Println("WritePump")
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.GetConnection().Close()
	}()

	for {
		select {
		case message, ok := <-client.GetSendChannel():
			client.GetConnection().SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				client.GetConnection().WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := client.GetConnection().WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			client.GetConnection().SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.GetConnection().WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
