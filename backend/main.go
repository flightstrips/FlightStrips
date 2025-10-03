package main

import (
	"FlightStrips/config"
	"FlightStrips/database"
	"context"
	_ "database/sql"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var addr = flag.String("addr", "0.0.0.0:2994", "http service address")

var upgrader = websocket.Upgrader{} // use default options

type Session struct {
	Id      int32
	Name    string
	Airport string
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) GetOrCreateSession(airport string, name string) (Session, error) {
	db := database.New(s.DBPool)

	arg := database.GetSessionParams{Name: name, Airport: airport}
	session, err := db.GetSession(context.Background(), arg)

	if err == nil {
		return Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("Creating session:", name, "for airport:", airport)
		insertArg := database.InsertSessionParams{Name: name, Airport: airport}
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
	config.InitConfig()
	dbpool, err := pgxpool.New(ctx, os.Getenv("DATABASE_CONNECTIONSTRING"))
	if err != nil {
		log.Fatal(err)
	}
	defer dbpool.Close()

	// Create the parent server struct
	server := Server{
		DBPool:                 dbpool,
		AuthServerURL:          os.Getenv("OIDC_AUTHORITY"),
		AuthSigningAlgo:        os.Getenv("OIDC_SIGNING_ALGO"),
		FrontendEventHandlers:  GetFrontendEventHandlers(),
		EuroscopeEventHandlers: GetEuroscopeEventHandlers(),
	}

	server.FrontendHub = NewFrontendHub(&server)
	server.EuroscopeHub = NewEuroscopeHub(&server)

	//check that the dbpool is working
	_, err = dbpool.Exec(ctx, ddl)
	if err != nil {
		log.Println("error checking connection to postgres database")
		log.Println(err.Error())
		log.Fatal(err)
	}

	// TODO remove
	db := database.New(dbpool)
	db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", server.EuroscopeEventsHandler)
	http.HandleFunc("/frontEndEvents", server.FrontendEventsHandler)

	// Start background tasks.
	go server.monitorSessions()
	go server.FrontendHub.Run()
	go server.EuroscopeHub.Run()

	log.Println("Server started on address:", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
