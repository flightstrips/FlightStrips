package main

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	_ "database/sql"
	"flag"
	"log"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/joho/godotenv"
)

var addr = flag.String("addr", "0.0.0.0:2994", "http service address")

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

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

	authenticationService, err := services.NewAuthenticationService(os.Getenv("OIDC_SIGNING_ALGO"), os.Getenv("OIDC_AUTHORITY"))
	if err != nil {
		log.Fatal(err)
	}

	cdmKey := os.Getenv("CDM_KEY")
	cdmClient := cdm.NewClient(cdm.WithAPIKey(cdmKey))

	stripService := services.NewStripService(dbpool)
	cdmService := cdm.NewCdmService(cdmClient, dbpool)

	frontendHub := frontend.NewHub(stripService)
	euroscopeHub := euroscope.NewHub(stripService)

	stripService.SetFrontendHub(frontendHub)
	cdmService.SetFrontendHub(frontendHub)

	fsServer := server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)

	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](frontendHub, authenticationService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](euroscopeHub, authenticationService)

	go cdmService.Start(ctx)

	// TODO remove
	db := database.New(dbpool)
	db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	http.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)

	log.Println("Server started on address:", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
