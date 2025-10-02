package main

import (
	"FlightStrips/data"
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	DBPool          *pgxpool.Pool
	AuthServerURL   string
	AuthSigningAlgo string
	EuroscopeHub    *EuroscopeHub
	FrontendHub     *FrontendHub

	FrontendEventHandlers FrontendEventHandlers
	EuroscopeEventHandlers EuroscopeEventHandlers
}

func handleWebsocketConnection[TClient WebsocketClient, THub Hub[TClient]](s *Server, w http.ResponseWriter, r *http.Request, initializer func(*Server, *websocket.Conn) (TClient, error), hub THub) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // TODO: Implement proper origin checking
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade connection:", err)
		return
	}

	client, err := initializer(s, conn)
	if err != nil {
		log.Println("Failed to initialize client:", err)
		conn.Close()
		return
	}

	hub.Register(client)

	go WritePump(hub, client)
	go ReadPump(hub, client)
}

func (s *Server) monitorSessions() {
	for {
		expired := time.Now().Add(-time.Minute * 5).UTC()
		db := data.New(s.DBPool)

		sessions, err := db.GetExpiredSessions(context.Background(), pgtype.Timestamp{Time: expired, Valid: true})

		if err != nil {
			log.Println("Failed to get expired sessions:", err)
		}

		for _, session := range sessions {
			log.Println("Removing expired session:", session)
			count, err := db.DeleteSession(context.Background(), session)
			if err != nil {
				log.Println("Failed to remove expired session:", session, err)
			}

			if count != 1 {
				log.Println("Failed to remove expired session (no changes):", session, err)
			}
		}

		time.Sleep(time.Minute)
	}
}
