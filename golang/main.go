package main

import (
	"FlightStrips/data"
	"context"
	_ "database/sql"
	"encoding/json"
	"flag"
	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net/http"
	"time"

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

// DBJob represents a database job request
type DBJob struct {
	Action func(ctx context.Context, q *data.Queries) (interface{}, error)
	Result chan<- interface{}
	Err    chan<- error
}

// dbWorker processes database jobs
func dbWorker(dbConn *pgxpool.Pool, jobs <-chan DBJob) {
	queries := data.New(dbConn)
	for job := range jobs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		// posible memory leak but I am not a good developer
		defer cancel()

		result, err := job.Action(ctx, queries)
		if err != nil {
			job.Err <- err
			continue
		}
		job.Result <- result
		job.Err <- nil
	}
}

// Server holds shared resources
type Server struct {
	DBPool *pgxpool.Pool
	// Jobs   chan DBJob Not needed with a PGX Pool
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

	// Create the parent server struct
	server := Server{
		DBPool: dbpool,
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

	log.Println("Server started on address:", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
