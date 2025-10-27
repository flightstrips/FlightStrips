package main

import (
	"FlightStrips/internal/database"
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var (
		dbPath        string
		migrationsDir string
	)
	flag.StringVar(&dbPath, "dsn", "user=postgres dbname=appdb sslmode=disable", "Postgres DSN (e.g., 'user=postgres dbname=appdb sslmode=disable' or URL form)")
	flag.StringVar(&migrationsDir, "migrations", "migrations", "Directory containing SQL migration files")
	flag.Parse()

	// Check migrations dir exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		log.Fatalf("Migrations directory %q does not exist", migrationsDir)
	}

	err := database.Migrate(dbPath, migrationsDir)
	if err != nil {
		log.Fatalf("Migration failed: %v", err)
	}
	fmt.Println("Migration finished successfully.")
}
