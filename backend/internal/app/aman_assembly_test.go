package app

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	internalEuroscope "FlightStrips/internal/euroscope"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	pkgModels "FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
)

type testAMANHealthReporter struct {
	authorityAllowed bool
	report           *aman.TechnicalHealth
}

func (testAMANHealthReporter) Name() string { return "test AMAN health" }
func (r testAMANHealthReporter) TechnicalHealth(context.Context) aman.TechnicalHealth {
	if r.report != nil {
		return *r.report
	}
	return aman.TechnicalHealth{AuthorityAllowed: r.authorityAllowed}
}

func TestAMANTransportUsesLiveVATSIMCacheForFrontendHealth(t *testing.T) {
	now := time.Date(2026, time.September, 13, 18, 45, 10, 0, time.UTC)
	ready := aman.ComponentHealth{Status: aman.HealthReady}
	lagging := aman.EvaluateTechnicalHealth(
		aman.ModeAuthoritative,
		aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "source_not_observed"},
		ready, ready, ready, ready, ready,
	)
	transport := &amanTransport{
		health:           testAMANHealthReporter{report: &lagging},
		vatsimSource:     amanHealthSnapshotSource{snapshot: vatsim.Snapshot{Timestamp: now.Add(-10 * time.Second)}},
		vatsimStaleAfter: time.Minute,
		now:              func() time.Time { return now },
	}

	health := transport.currentTechnicalHealth(context.Background())
	require.Equal(t, aman.HealthReady, health.VATSIM.Status)
	require.True(t, health.Ready)
	require.True(t, health.AuthorityAllowed)
}

func TestAMANTransportAlwaysMarksGainLossAuthoritative(t *testing.T) {
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: time.Now().UTC(), Authoritative: false}

	unhealthy, err := (&amanTransport{health: testAMANHealthReporter{}}).newGainLossEvent(context.Background(), state)
	require.NoError(t, err)
	require.True(t, unhealthy.Authoritative)

	healthy, err := (&amanTransport{health: testAMANHealthReporter{authorityAllowed: true}}).newGainLossEvent(context.Background(), state)
	require.NoError(t, err)
	require.True(t, healthy.Authoritative)
}

func TestAMANTransportProjectsConfirmedHoldingReleaseAsTopSkyEAT(t *testing.T) {
	release := time.Date(2026, time.September, 13, 14, 22, 37, 0, time.UTC)
	holdingID := "EKCH-OLPIB-PRIMARY"
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{{
		CurrentCallsign:  "SAS123",
		SelectedHolding:  &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}}}

	events := holdingEATTransport(holdingID, "OLPIB").newHoldingEATEvents(context.Background(), state)

	require.Equal(t, []euroscopeEvents.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}}, events)
}

func TestAMANTransportPublishesHoldingEATWithdrawalWhenProjectionDisappears(t *testing.T) {
	release := time.Date(2026, time.September, 13, 14, 22, 0, 0, time.UTC)
	holdingID := "EKCH-OLPIB-PRIMARY"
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{{
		CurrentCallsign:  "SAS123",
		SelectedHolding:  &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}}}
	transport := holdingEATTransport(holdingID, "OLPIB")

	require.Equal(t,
		[]euroscopeEvents.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}},
		transport.newHoldingEATPublication(context.Background(), state),
	)

	state.Flights[0].HoldingStack.Confirmed = false
	require.Equal(t,
		[]euroscopeEvents.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute"}},
		transport.newHoldingEATPublication(context.Background(), state),
	)
	require.Empty(t, transport.newHoldingEATPublication(context.Background(), state))
}

func TestAMANTransportPublishesInitialHoldingEATWhenClearanceAlreadyMatches(t *testing.T) {
	release := time.Date(2026, time.September, 13, 14, 22, 0, 0, time.UTC)
	holdingID := "EKCH-OLPIB-PRIMARY"
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{{
		CurrentCallsign:  "SAS123",
		SelectedHolding:  &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: "1422"},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}}}
	transport := holdingEATTransport(holdingID, "OLPIB")

	require.Equal(t,
		[]euroscopeEvents.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}},
		transport.newHoldingEATPublication(context.Background(), state),
	)
	require.Empty(t, transport.newHoldingEATPublication(context.Background(), state))
}

func TestAMANTransportSuppressesUnsafeOrDuplicateHoldingEAT(t *testing.T) {
	release := time.Date(2026, time.September, 13, 14, 22, 0, 0, time.UTC)
	holdingID := "EKCH-OLPIB-PRIMARY"
	flight := aman.AMANFlight{
		CurrentCallsign: "SAS123", SelectedHolding: &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}
	transport := holdingEATTransport(holdingID, "OLPIB")

	require.Empty(t, transport.newHoldingEATEvents(context.Background(), aman.AirportState{Authoritative: false, Flights: []aman.AMANFlight{flight}}))
	transport.holdingEATEnabled = false
	require.Empty(t, transport.newHoldingEATEvents(context.Background(), aman.AirportState{Authoritative: true, Flights: []aman.AMANFlight{flight}}))
	transport.holdingEATEnabled = true
	flight.HoldingStack.Confirmed = false
	require.Empty(t, transport.newHoldingEATEvents(context.Background(), aman.AirportState{Authoritative: true, Flights: []aman.AMANFlight{flight}}))
	flight.HoldingStack.Confirmed = true
	flight.HoldingClearance.HoldEAT = "1422"
	require.Empty(t, transport.newHoldingEATEvents(context.Background(), aman.AirportState{Authoritative: true, Flights: []aman.AMANFlight{flight}}))
}

func TestAMANTransportReplaysStoredHoldingEATOnReconnect(t *testing.T) {
	release := time.Date(2026, time.September, 13, 14, 22, 0, 0, time.UTC)
	holdingID := "EKCH-OLPIB-PRIMARY"
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{{
		CurrentCallsign: "SAS123", SelectedHolding: &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute, HoldEAT: "1422"},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}}}
	transport := holdingEATTransport(holdingID, "OLPIB")
	transport.repository = amanStateReaderStub{state: state}

	events, err := transport.CurrentAMANHoldingEAT(context.Background(), "EKCH")

	require.NoError(t, err)
	require.Equal(t, []euroscopeEvents.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}}, events)
}

func TestAMANTransportSuppressesEATWhenClearanceDoesNotMatchSelectedHoldingFix(t *testing.T) {
	holdingID := "EKCH-TIDVU-PRIMARY"
	release := time.Date(2026, time.September, 13, 14, 22, 0, 0, time.UTC)
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, Flights: []aman.AMANFlight{{
		CurrentCallsign: "SAS123", SelectedHolding: &holdingID,
		HoldingClearance: &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack:     &aman.HoldingStackState{HoldingID: holdingID, Confirmed: true},
		Prediction:       &aman.Prediction{HoldingPlan: &aman.HoldingPlan{ApproachReleaseTime: release}},
	}}}

	require.Empty(t, holdingEATTransport(holdingID, "TIDVU").newHoldingEATEvents(context.Background(), state))
}

func TestAMANTransportReevaluatesHoldingEATOnUnchangedTickWithoutGainLoss(t *testing.T) {
	geometryCalls := 0
	transport := &amanTransport{
		health: testAMANHealthReporter{authorityAllowed: true}, holdingEATEnabled: true,
		geometry: countingTimelineGeometry{calls: &geometryCalls},
	}
	transport.setHubs(nil, &internalEuroscope.Hub{})

	require.NoError(t, transport.PublishAMANAuthority(context.Background(), aman.AirportState{Airport: "EKCH", Authoritative: true}))
	require.Equal(t, 1, geometryCalls)
}

func holdingEATTransport(holdingID, fix string) *amanTransport {
	return &amanTransport{
		health: testAMANHealthReporter{authorityAllowed: true}, holdingEATEnabled: true,
		geometry: testTimelineGeometry{snapshot: navdata.ActiveGeometrySnapshot{
			Holdings: []navdata.HoldingPattern{{ID: navdata.HoldingID(holdingID), Fix: navdata.FixID(fix)}},
		}},
	}
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

type amanStateReaderStub struct{ state aman.AirportState }

func (r amanStateReaderStub) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	return r.state, nil
}

func (g testTimelineGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return g.snapshot, nil
}

type countingTimelineGeometry struct{ calls *int }

func TestAMANPublicationLoadsGeometryOnceAndRefreshesOnNextPublication(t *testing.T) {
	calls := 0
	ready := aman.ComponentHealth{Status: aman.HealthReady}
	health := aman.EvaluateTechnicalHealth(aman.ModeAuthoritative, ready, ready, ready, ready, ready, ready)
	transport := &amanTransport{
		health: testAMANHealthReporter{report: &health}, holdingEATEnabled: true,
		geometry: countingTimelineGeometry{calls: &calls},
	}
	transport.setHubs(nil, &internalEuroscope.Hub{})
	state := aman.AirportState{
		Airport: "EKCH", GeneratedAt: time.Now().UTC(), PolicyVersion: "policy-v1",
		Mode: aman.ModeAuthoritative, Authoritative: true,
		Flights: []aman.AMANFlight{}, RunwayGroups: []aman.RunwayGroupPolicy{},
	}
	require.NoError(t, transport.PublishAMANState(context.Background(), state))
	require.Equal(t, 1, calls, "timeline and holding EAT must share one geometry snapshot")
	require.NoError(t, transport.PublishAMANState(context.Background(), state))
	require.Equal(t, 2, calls, "a subsequent publication must see newly activated geometry")
}

func (g countingTimelineGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	*g.calls++
	return navdata.ActiveGeometrySnapshot{}, nil
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
