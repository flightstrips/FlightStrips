package main

import (
	"FlightStrips/data"
	"context"
	_ "database/sql"
	"encoding/json"
	"flag"
	"github.com/jackc/pgx/v5/pgtype"
	"log"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", ":2994", "http service address")

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

func (s *Server) handleServerLogging(logging chan interface{}) {
	for {
		msg := <-logging

		var dataToInsert data.InsertIntoEventsParams
		switch msg := msg.(type) {
		case Event:
			dataToInsert = data.InsertIntoEventsParams{
				Type: pgtype.Text{
					String: string(msg.Type),
				},
				Timestamp: pgtype.Text{
					String: msg.TimeStamp.String(),
				},
				Cid: pgtype.Text{
					String: msg.Cid,
				},
				Data: pgtype.Text{
					String: msg.Payload.(string),
				},
			}
		default:
			dataToInsert = data.InsertIntoEventsParams{
				Type: pgtype.Text{
					String: "Unknown",
				},
				Timestamp: pgtype.Text{
					String: time.Now().String(),
				},
				Cid: pgtype.Text{
					String: "Server",
				},
				Data: pgtype.Text{
					String: msg.(string),
				},
			}

		}

		database := data.New(s.DBPool)

		err := database.InsertIntoEvents(context.Background(), dataToInsert)
		if err != nil {
			log.Println("error logging event")
		}
	}
}

// Server holds shared resources
type Server struct {
	DBPool  *pgxpool.Pool
	logging chan interface{}
}

func (s *Server) log(msg string) {
	s.logging <- msg
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

//go:embed schema.sql
var ddl string

func main() {
	flag.Parse()
	log.SetFlags(0)

	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, "postgresql://theoa:theoa@postgres/fsdb?sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	defer dbpool.Close()

	logging := make(chan interface{})

	// Create the parent server struct
	server := Server{
		DBPool:  dbpool,
		logging: logging,
	}

	//check that the dbpool is working
	_, err = dbpool.Exec(ctx, ddl)
	if err != nil {
		log.Println("error checking connection to postgres database")
		log.Println(err.Error())
		log.Fatal(err)
	}

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", server.euroscopeEvents)
	http.HandleFunc("/frontEndEvents", server.frontEndEvents)

	// Start background tasks.
	go handleFrontEndBroadcast()
	go periodicMessages()
	go server.handleServerLogging(logging)

	log.Println("Server started on address:", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
