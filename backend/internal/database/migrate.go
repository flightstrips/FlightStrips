package database

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration struct {
	ID   int
	Name string
	Path string
}

func parseMigrations(dir string) ([]Migration, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	re := regexp.MustCompile(`^(\d+)-([a-zA-Z0-9_\-]+)\.sql$`)
	var migrations []Migration
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(file.Name())
		if len(matches) == 3 {
			id, _ := strconv.Atoi(matches[1])
			name := matches[2]
			migrations = append(migrations, Migration{
				ID:   id,
				Name: name,
				Path: filepath.Join(dir, file.Name()),
			})
		}
	}
	// Sort by ID
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].ID < migrations[j].ID
	})

	return migrations, nil
}

// parseDSN uses pgxpool.ParseConfig to robustly parse and rewrite the DSN for main and maintenance DBs.
func parseDSN(dsn string) (mainDSN string, baseDSN string, dbname string, err error) {
	mainCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "", "", "", err
	}
	dbname = mainCfg.ConnConfig.Database
	baseCfg := mainCfg.Copy()
	baseCfg.ConnConfig.Database = "postgres"
	mainDSN = mainCfg.ConnString()
	baseDSN = baseCfg.ConnString()
	return mainDSN, baseDSN, dbname, nil
}

// ensureDB connects to postgres using the provided DSN and creates the DB if missing.
// Returns a *pgxpool.Pool connected to the desired database.
func ensureDB(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	mainDSN, baseDSN, dbname, err := parseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid DSN: %w", err)
	}
	// Try connecting to main database first
	mainPool, err := pgxpool.New(ctx, mainDSN)
	if err == nil {
		if err := mainPool.Ping(ctx); err == nil {
			return mainPool, nil
		}
		mainPool.Close()
	}
	// Only proceed to create if the error indicates the db does not exist
	// Connect to default/maintenance db and create the target db
	basePool, err := pgxpool.New(ctx, baseDSN)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to base (maintenance) db: %w", err)
	}
	defer basePool.Close()
	_, err = basePool.Exec(ctx, fmt.Sprintf("CREATE DATABASE '%s'", dbname))
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}
	// Now connect again to main database
	mainPool, err = pgxpool.New(ctx, mainDSN)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to new database: %w", err)
	}
	if err := mainPool.Ping(ctx); err != nil {
		mainPool.Close()
		return nil, fmt.Errorf("unable to ping new database: %w", err)
	}
	return mainPool, nil
}

func getAppliedMigrations(q *Queries, ctx context.Context) (map[int]bool, error) {
	applied := map[int]bool{}
	ids, err := q.GetAllDatabaseVersions(ctx)
	if err != nil {
		// If the error is "relation does not exist", it's the first migration
		if strings.Contains(err.Error(), "does not exist") {
			return applied, nil // Table missing means nothing has run yet
		}
		return nil, err
	}
	for _, id := range ids {
		applied[int(id.ID)] = true
	}
	return applied, nil
}

func applyMigration(db *pgxpool.Pool, m Migration, qgen *Queries, ctx context.Context) error {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return fmt.Errorf("error reading migration %s: %w", m.Path, err)
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback(ctx)
			panic(p)
		}
	}()
	qtx := qgen.WithTx(tx)
	if _, err := tx.Exec(ctx, string(content)); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("error applying migration %s: %w", m.Path, err)
	}

	if err := qtx.InsertDatabaseVersion(ctx, InsertDatabaseVersionParams{
		ID:   int32(m.ID),
		Name: m.Name,
	}); err != nil {
		tx.Rollback(ctx)
		return fmt.Errorf("error recording migration in versions: %w", err)
	}
	return tx.Commit(ctx)
}

func Migrate(dsn, migrationsDir string) error {
	migrations, err := parseMigrations(migrationsDir)
	if err != nil {
		return err
	}
	db, err := ensureDB(context.Background(), dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx := context.Background()
	q := New(db)

	applied, err := getAppliedMigrations(q, ctx)
	if err != nil {
		return err
	}

	var toApply []Migration
	for _, m := range migrations {
		if !applied[m.ID] {
			toApply = append(toApply, m)
		}
	}
	if len(toApply) == 0 {
		fmt.Println("No new migrations to apply.")
		return nil
	}
	for _, m := range toApply {
		fmt.Printf("Applying migration %d: %s\n", m.ID, m.Name)
		if err := applyMigration(db, m, q, ctx); err != nil {
			return err
		}
		fmt.Printf("Applied migration %d: %s\n", m.ID, m.Name)
	}
	fmt.Printf("Migrations complete. %d applied.\n", len(toApply))
	return nil
}
