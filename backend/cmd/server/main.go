package main

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/server"
	"FlightStrips/internal/services"
	"FlightStrips/internal/websocket"
	pkgEuroscope "FlightStrips/pkg/events/euroscope"
	pkgFrontend "FlightStrips/pkg/events/frontend"
	"context"
	_ "database/sql"
	"flag"
	"log/slog"
	"net/http"
	"os"

	_ "github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	_ "embed"

	_ "github.com/jackc/pgx/v5/pgtype"

	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

var addr = flag.String("addr", "0.0.0.0:2994", "http service address")

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func main() {
	flag.Parse()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	err := godotenv.Load()
	if err != nil {
		slog.Error("Error loading .env file", slog.Any("error", err))
		os.Exit(1)
	}

	ctx := context.Background()
	config.InitConfig()
	dbpool, err := pgxpool.New(ctx, os.Getenv("DATABASE_CONNECTIONSTRING"))
	if err != nil {
		slog.Error("Failed to connect to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer dbpool.Close()

	authenticationService, err := services.NewAuthenticationService(os.Getenv("OIDC_SIGNING_ALGO"), os.Getenv("OIDC_AUTHORITY"))
	if err != nil {
		slog.Error("Failed to initialize authentication service", slog.Any("error", err))
		os.Exit(1)
	}

	cdmKey := os.Getenv("CDM_KEY")
	cdmClient := cdm.NewClient(cdm.WithAPIKey(cdmKey))

	// Initialize repositories
	stripRepo := postgres.NewStripRepository(dbpool)
	controllerRepo := postgres.NewControllerRepository(dbpool)
	sessionRepo := postgres.NewSessionRepository(dbpool)
	sectorRepo := postgres.NewSectorOwnerRepository(dbpool)
	coordRepo := postgres.NewCoordinationRepository(dbpool)

	// Initialize services
	stripService := services.NewStripService(stripRepo)
	cdmService := cdm.NewCdmService(cdmClient, stripRepo, sessionRepo)

	// Initialize PDC Service
	hoppieLogon := os.Getenv("HOPPIE_LOGON")
	var pdcService *pdc.Service
	if hoppieLogon != "" {
		hoppieClient := pdc.NewClient(hoppieLogon)
		pdcService = pdc.NewPDCService(hoppieClient, sessionRepo, stripRepo, sectorRepo)
		pdcService.SetStripService(stripService)
		slog.Info("PDC Service initialized")
	} else {
		slog.Warn("PDC Service not initialized - HOPPIE_LOGON")
	}

	frontendHub := frontend.NewHub(stripService)
	euroscopeHub := euroscope.NewHub(stripService)

	stripService.SetFrontendHub(frontendHub)
	stripService.SetEuroscopeHub(euroscopeHub)
	cdmService.SetFrontendHub(frontendHub)
	if pdcService != nil {
		pdcService.SetFrontendHub(frontendHub)
	}

	fsServer := server.NewServer(dbpool, euroscopeHub, frontendHub, cdmService, pdcService, stripRepo, controllerRepo, sessionRepo, sectorRepo, coordRepo)

	frontendHub.SetServer(fsServer)
	euroscopeHub.SetServer(fsServer)

	frontendUpgrader := websocket.NewConnectionUpgrader[pkgFrontend.EventType, *frontend.Client](frontendHub, authenticationService)
	euroscopeUpgrader := websocket.NewConnectionUpgrader[pkgEuroscope.EventType, *euroscope.Client](euroscopeHub, authenticationService)

	go cdmService.Start(ctx)
	if pdcService != nil {
		go pdcService.Start(ctx)
	}

	// TODO remove
	db := database.New(dbpool)
	_ = db.InsertAirport(context.Background(), "EKCH")

	// Health Function for local Dev
	http.HandleFunc("/healthz", healthz)
	http.HandleFunc("/euroscopeEvents", euroscopeUpgrader.Upgrade)
	http.HandleFunc("/frontEndEvents", frontendUpgrader.Upgrade)

	slog.Info("Server started", slog.String("address", *addr))
	if err := http.ListenAndServe(*addr, nil); err != nil {
		slog.Error("Server failed", slog.Any("error", err))
		os.Exit(1)
	}
}
