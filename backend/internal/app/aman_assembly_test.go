package app

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
)

type testAMANHealthReporter struct{ authorityAllowed bool }

func (testAMANHealthReporter) Name() string { return "test AMAN health" }
func (r testAMANHealthReporter) TechnicalHealth(context.Context) aman.TechnicalHealth {
	return aman.TechnicalHealth{AuthorityAllowed: r.authorityAllowed}
}

func TestAMANTransportAppliesCurrentAuthorityGateToGainLoss(t *testing.T) {
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: time.Now().UTC(), Authoritative: true}

	blocked, err := (&amanTransport{health: testAMANHealthReporter{}}).newGainLossEvent(context.Background(), state)
	require.NoError(t, err)
	require.False(t, blocked.Authoritative)

	allowed, err := (&amanTransport{health: testAMANHealthReporter{authorityAllowed: true}}).newGainLossEvent(context.Background(), state)
	require.NoError(t, err)
	require.True(t, allowed.Authoritative)
}

func TestAMANTransportProjectsAvailableTimelineConfiguration(t *testing.T) {
	now := time.Now().UTC()
	state := aman.AirportState{Airport: "EKCH", GeneratedAt: now, PolicyVersion: "policy-v1", Mode: aman.ModeDisabled, Flights: []aman.AMANFlight{}, RunwayGroups: []aman.RunwayGroupPolicy{}}
	health := aman.EvaluateTechnicalHealth(aman.ModeDisabled, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{}, aman.ComponentHealth{})
	family := navdata.STARFamilyID("NORTH")
	transport := &amanTransport{geometry: testTimelineGeometry{snapshot: navdata.ActiveGeometrySnapshot{
		TerminalVersion: "mapping-v1", TimelineMappings: []navdata.TimelineMapping{{ID: 1, Left: &family}},
	}}}

	event, err := transport.newStateEvent(context.Background(), state, health)
	require.NoError(t, err)
	require.Equal(t, "mapping-v1", event.Data.TimelineConfig.Version)
	require.Equal(t, "NORTH", *event.Data.TimelineConfig.Mappings[0].Left)
}

type testTimelineGeometry struct {
	snapshot navdata.ActiveGeometrySnapshot
}

func (g testTimelineGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return g.snapshot, nil
}

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
