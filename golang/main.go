package main

import (
	"FlightStrips/data"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"

	_ "embed"
	_ "github.com/mattn/go-sqlite3"

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
func dbWorker(dbConn *sql.DB, jobs <-chan DBJob) {
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
	DBConn *sql.DB
	Jobs   chan DBJob
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

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		log.Fatal(err)
	}
	// create tables
	if _, err := db.ExecContext(ctx, ddl); err != nil {
		log.Fatal(err)
	}
	// I think this is to close the DB connection when the main function exits
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {

		}
	}(db)

	// Create a jobs channel and workers
	jobs := make(chan DBJob, 10)
	for i := 0; i < 5; i++ {
		go dbWorker(db, jobs)
	}

	// Create the parent server struct
	server := Server{
		DBConn: db,
		Jobs:   jobs,
	}

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	//http.HandleFunc("/euroscopeEvents", euroscopeEvents)
	http.HandleFunc("/frontEndEvents", server.frontEndEvents)

	// Start background tasks.
	go handleFrontEndBroadcast()
	go periodicMessages()

	log.Fatal(http.ListenAndServe(*addr, nil))
}
