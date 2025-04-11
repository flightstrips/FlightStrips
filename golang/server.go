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
	EuroscopeHub    *BaseHub[*EuroscopeClient]
	FrontendHub     *BaseHub[*FrontendClient]
}

func handleWebsocketConnection[T WebsocketClient](s *Server, w http.ResponseWriter, r *http.Request, initializer func(*Server, *websocket.Conn) (T, error), hub *BaseHub[T]) {
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

	hub.register <- client

	go hub.WritePump(client)
	go hub.ReadPump(client)
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
