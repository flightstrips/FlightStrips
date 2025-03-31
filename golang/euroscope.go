package main

import (
	"FlightStrips/data"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/gorilla/websocket"
)

type EuroscopeUser struct {
	cid       string
	rating    int
	authToken *jwt.Token
}

type EuroscopeClient struct {
	conn     *websocket.Conn
	send     chan []byte
	user     *EuroscopeUser
	airport  string
	session  int32
	position string
	callsign string
}

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

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

	// go handleOutgoingMessages(client)
	client, err := s.euroscopeInitialEventsHandler(conn)
	if err != nil {
		log.Printf("Error handling initial events: %s \n", err)
		return
	}
	defer func() {
		close(client.send)
		s.euroscopeeventhandlerConnectionClosed(client)
	}()
	go client.writeLoop()

	// TODO: Handle this on which one is the master etc
	client.send <- []byte("{\"type\": \"session_info\", \"role\": \"master\"}")

	euroscopeClients[client] = true

	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		db := data.New(s.DBPool)
		params := data.SetControllerEuroscopeSeenParams{Callsign: client.callsign, Session: client.session, LastSeenEuroscope: pgtype.Timestamp{Valid: true, Time: time.Now().UTC()}}
		_, err := db.SetControllerEuroscopeSeen(context.Background(), params)
		return err
	})

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

		err = s.euroscopeEventsHandler(client, event, msg)
		if err != nil {
			fmt.Printf("Failed to process event %s with error %v\n", event.Type, err)
		}
	}

	delete(euroscopeClients, client)
}

func (c *EuroscopeClient) writeLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))

			if !ok {
				// We want to close the connection
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.conn.WriteMessage(websocket.TextMessage, message)
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) euroscopeInitialEventsHandler(conn *websocket.Conn) (client *EuroscopeClient, err error) {
	// Auth handling
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	user, err := s.euroscopeeventhandlerAuthentication(msg)
	if err != nil {
		return nil, err
	}
	// Login Handling

	_, msg, err = conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	loginEvent, sessionId, err := s.euroscopeeventhandlerLogin(msg)
	if err != nil {
		return nil, err
	}

	// Controller Online

	client = &EuroscopeClient{conn: conn, send: make(chan []byte), user: user, airport: loginEvent.Airport, position: loginEvent.Position, callsign: loginEvent.Callsign, session: sessionId}
	return client, nil
}

func (s *Server) euroscopeEventsHandler(client *EuroscopeClient, event EuroscopeEvent, msg []byte) error {

	switch event.Type {
	case EuroscopeControllerOnline:
		return s.euroscopeeventhandlerControllerOnline(msg, client.session, client.airport)
	case EuroscopeControllerOffline:
		return s.euroscopeeventhandlerControllerOffline(msg, client.session, client.airport)
	case EuroscopeSync:
		return s.euroscopeeventhandlerSync(msg, client.session, client.airport)
	case EuroscopeAssignedSquawk:
		return s.euroscopeeventhandlerAssignedSquawk(msg, client.session)
	case EuroscopeSquawk:
		return s.euroscopeeventhandlerSquawk(msg, client.session)
	case EuroscopeRequestedAltitude:
		return s.euroscopeeventhandlerRequestedAltitude(msg, client.session)
	case EuroscopeClearedAltitude:
		return s.euroscopeeventhandlerClearedAltitude(msg, client.session)
	case EuroscopeCommunicationType:
		return s.euroscopeeventhandlerCommunicationType(msg, client.session)
	case EuroscopeGroundState:
		return s.euroscopeeventhandlerGroundState(msg, client.session)
	case EuroscopeClearedFlag:
		return s.euroscopeeventhandlerClearedFlag(msg, client.session)
	case EuroscopePositionUpdate:
		return s.euroscopeeventhandlerPositionUpdate(msg, client.session)
	case EuroscopeSetHeading:
		return s.euroscopeeventhandlerSetHeading(msg, client.session)
	case EuroscopeAircraftDisconnected:
		return s.euroscopeeventhandlerAircraftDisconnected(msg, client.session)
	case EuroscopeStand:
		return s.euroscopeeventhandlerStand(msg, client.session)
	case EuroscopeStripUpdate:
		return s.euroscopeeventhandlerStripUpdate(msg, client.session)
	case EuroscopeRunway:
		return errors.New("not implemented")
	default:
		return errors.New("unknown event type")
	}
}
