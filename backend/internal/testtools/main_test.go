package testtools

import (
	"FlightStrips/internal/pdc/testdata"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}
