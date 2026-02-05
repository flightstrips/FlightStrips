package main

import (
	"FlightStrips/internal/database"
	"flag"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
)

func main() {
	var (
		dbPath        string
		migrationsDir string
	)
	flag.StringVar(&dbPath, "dsn", "user=postgres dbname=appdb sslmode=disable", "Postgres DSN (e.g., 'user=postgres dbname=appdb sslmode=disable' or URL form)")
	flag.StringVar(&migrationsDir, "migrations", "migrations", "Directory containing SQL migration files")
	flag.Parse()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Check migrations dir exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		slog.Error("Migrations directory does not exist", slog.String("directory", migrationsDir))
		os.Exit(1)
	}

	err := database.Migrate(dbPath, migrationsDir)
	if err != nil {
		slog.Error("Migration failed", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Migration finished successfully")
}
