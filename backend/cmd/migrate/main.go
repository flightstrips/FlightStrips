package main

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/envconfig"
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
	dsnDefault, err := envconfig.Value("DATABASE_CONNECTIONSTRING")
	if err != nil {
		slog.Error("Failed to load database connection secret", slog.Any("error", err))
		os.Exit(1)
	}
	if dsnDefault == "" {
		dsnDefault = "user=postgres dbname=appdb sslmode=disable"
	}
	flag.StringVar(&dbPath, "dsn", dsnDefault, "Postgres DSN (e.g., 'user=postgres dbname=appdb sslmode=disable' or URL form)")
	flag.StringVar(&migrationsDir, "migrations", "migrations", "Directory containing SQL migration files")
	flag.Parse()

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Check migrations dir exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		slog.Error("Migrations directory does not exist", slog.String("directory", migrationsDir))
		os.Exit(1)
	}

	err = database.Migrate(dbPath, migrationsDir)
	if err != nil {
		slog.Error("Migration failed", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Migration finished successfully")
}
