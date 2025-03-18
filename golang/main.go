package main

import (
	"FlightStrips/data"
	"context"
	_ "database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var addr = flag.String("addr", "127.0.0.1:2994", "http service address")

var upgrader = websocket.Upgrader{} // use default options

// FrontEndClient structure.
type FrontEndClient struct {
	conn     *websocket.Conn
	send     chan []byte // Channel for outgoing messages.
	cid      string
	airport  string
	position string
}

// Global variables for managing clients.
var frontEndClients = make(map[*FrontEndClient]bool) // Map to track connected FrontEnd clients.
var frontEndBroadcast = make(chan []byte)            // Channel for broadcasting messages for the FrontEnd.

// Goroutine to handle outgoing messages for each client.
func handleOutgoingMessages(client *FrontEndClient) {
	//TODO: Store Message somewhere?
	//How do we determine when a message is out of lifespan?
	for msg := range client.send {
		log.Printf("send to all FE Clients: %s", msg)
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
		time.Sleep(50 * time.Second)
		serealisedHeartbeatEvent, err := json.Marshal(NewHeartBeatEvent("Server heartbeat"))
		if err != nil {
			log.Println("error serialising heartbeat event")
			continue
		}
		frontEndBroadcast <- serealisedHeartbeatEvent
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

// Server holds shared resources
type Server struct {
	DBPool          *pgxpool.Pool
	AuthServerURL   string
	AuthSigningAlgo string
}

type Session struct {
	Id      int32
	Name    string
	Airport string
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetOrCreateSession(airport string, name string) (Session, error) {
	db := data.New(s.DBPool)

	arg := data.GetSessionParams{Name: name, Airport: airport}
	session, err := db.GetSession(context.Background(), arg)

	if err == nil {
		return Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
	}

	if err == pgx.ErrNoRows {
		insertArg := data.InsertSessionParams{Name: name, Airport: airport}
		id, err := db.InsertSession(context.Background(), insertArg)

		return Session{Name: name, Airport: airport, Id: id}, err
	}

	return Session{}, nil
}

//go:embed schema.sql
var ddl string

func main() {
	flag.Parse()
	log.SetFlags(0)

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, os.Getenv("DATABASE_CONNECTIONSTRING"))
	if err != nil {
		log.Fatal(err)
	}
	defer dbpool.Close()

	// Create the parent server struct
	server := Server{
		DBPool:          dbpool,
		AuthServerURL:   os.Getenv("OIDC_AUTHORITY"),
		AuthSigningAlgo: os.Getenv("OIDC_SIGNING_ALGO"),
	}

	//check that the dbpool is working
	_, err = dbpool.Exec(ctx, ddl)
	if err != nil {
		log.Println("error checking connection to postgres database")
		log.Println(err.Error())
		log.Fatal(err)
	}

	// TODO remove
	db := data.New(dbpool)
	db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", server.euroscopeEvents)
	http.HandleFunc("/frontEndEvents", server.frontEndEvents)

	// Start background tasks.
	go handleFrontEndBroadcast()
	go periodicMessages()
	go server.monitorSessions()

	log.Println("Server started on address:", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
