package pdc

import (
	"FlightStrips/internal/config"
	"os"
	"testing"
)

// Policy tests use the same checked-in airport configuration as the runtime,
// without constructing a database or a provider worker.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic(err)
	}
	if err := config.InitConfig(); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
