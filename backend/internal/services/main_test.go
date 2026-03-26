package services

import (
	"FlightStrips/internal/config"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}

	config.InitConfig()
	os.Exit(m.Run())
}
