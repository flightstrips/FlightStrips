// Command testdb runs the Go test suite against one PostgreSQL server. Individual
// tests still receive isolated databases cloned from the migrated testdb template.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"FlightStrips/internal/database"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const testDatabaseServerURLEnv = "FLIGHTSTRIPS_TEST_DATABASE_SERVER_URL"

func main() {
	os.Exit(run())
}

func run() (code int) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("postgres"),
		postgres.WithPassword("postgres"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second)),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start shared PostgreSQL test server:", err)
		return 1
	}

	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			fmt.Fprintln(os.Stderr, "stop shared PostgreSQL test server:", err)
			if code == 0 {
				code = 1
			}
		}
	}()

	serverURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "get shared PostgreSQL test server URL:", err)
		return 1
	}
	migrationsPath, err := filepath.Abs("migrations")
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve migrations path:", err)
		return 1
	}
	if err := database.Migrate(serverURL, migrationsPath); err != nil {
		fmt.Fprintln(os.Stderr, "migrate shared PostgreSQL test server:", err)
		return 1
	}

	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"./..."}
	}
	command := exec.CommandContext(ctx, "go", append([]string{"test"}, args...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = append(os.Environ(), testDatabaseServerURLEnv+"="+serverURL)
	if err := command.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			code = exitError.ExitCode()
		} else {
			fmt.Fprintln(os.Stderr, "run Go tests:", err)
			code = 1
		}
	}
	return code
}
