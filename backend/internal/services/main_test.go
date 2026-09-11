package services

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/pdc/testdata"
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
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}
