package app

import (
	"context"
	"testing"

	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestValidateTerminalAirportCoverage(t *testing.T) {
	configuration := terminal.Configuration{Airport: navdata.AirportID("EKCH")}

	require.NoError(t, validateTerminalAirportCoverage(configuration, []string{"ekch"}))
	require.ErrorContains(t, validateTerminalAirportCoverage(configuration, []string{"EKCH", "EGLL"}), "requires exactly that enabled airport")
	require.ErrorContains(t, validateTerminalAirportCoverage(configuration, []string{"EGLL"}), "requires exactly that enabled airport")
}

func TestSessionArrivalRunwaySourceUsesOneConfiguredArrivalRunway(t *testing.T) {
	source := sessionArrivalRunwaySource{sessions: testSessionLister{sessions: []*models.Session{
		{Airport: "EKCH", ActiveRunways: pkgModels.ActiveRunways{ArrivalRunways: []string{"22l"}}},
		{Airport: "ENGM", ActiveRunways: pkgModels.ActiveRunways{ArrivalRunways: []string{"01L"}}},
	}}}
	runway, err := source.ActiveArrivalRunway(context.Background(), "EKCH")
	require.NoError(t, err)
	require.Equal(t, "22L", runway)
}

func TestSessionArrivalRunwaySourceRejectsAmbiguousArrivalRunways(t *testing.T) {
	source := sessionArrivalRunwaySource{sessions: testSessionLister{sessions: []*models.Session{{
		Airport: "EKCH", ActiveRunways: pkgModels.ActiveRunways{ArrivalRunways: []string{"22L", "22R"}},
	}}}}
	_, err := source.ActiveArrivalRunway(context.Background(), "EKCH")
	require.ErrorContains(t, err, "multiple active arrival runways")
}

type testSessionLister struct{ sessions []*models.Session }

func (s testSessionLister) List(context.Context) ([]*models.Session, error) { return s.sessions, nil }
