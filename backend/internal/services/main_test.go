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

	if err := config.InitConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
	os.Exit(m.Run())
}
