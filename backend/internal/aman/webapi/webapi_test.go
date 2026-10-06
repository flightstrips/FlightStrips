package webapi

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type sessionStateReader struct{ session int32 }

func (s *sessionStateReader) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	s.session = aman.SessionID(ctx)
	return aman.AirportState{Airport: airport, Flights: []aman.AMANFlight{}}, nil
}
func TestFlightDetailUsesAuthenticatedSessionAndRejectsOtherAirport(t *testing.T) {
	reader := &sessionStateReader{}
	mux := http.NewServeMux()
	New(testAuth{}, reader).WithSessionResolver(func(ctx context.Context, cid, airport string) (int32, error) {
		if airport != "EKCH" {
			return 0, errors.New("wrong airport")
		}
		return 12, nil
	}).RegisterRoutes(mux)
	for _, airport := range []string{"EKCH", "EKBI"} {
		request := httptest.NewRequest(http.MethodGet, "/aman/airports/"+airport+"/flights/SAS123/detail", nil)
		request.Header.Set("Authorization", "Bearer token")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if airport == "EKCH" {
			require.Equal(t, http.StatusNotFound, response.Code)
			require.Equal(t, int32(12), reader.session)
		} else {
			require.Equal(t, http.StatusForbidden, response.Code)
		}
	}
}

func TestFlightDetailReturnsOnDemandCalculationAndOperationalBasis(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	observed := now.Add(-time.Minute)
	groundspeed := 280.0
	altitude := 9000
	aircraftType, wakeCategory, filedRoute := "A320", "M", "NEXIL M725 KEMAX TUDLO"
	feederFix, starFamily := "TNO", "TESPI"
	feederETA, derivedFeederETA := now.Add(11*time.Minute), now.Add(10*time.Minute)
	state := aman.AirportState{
		Airport: "EKCH", Revision: 14, GeneratedAt: now, PolicyVersion: "test", Mode: aman.ModeShadow,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 30}},
		Flights: []aman.AMANFlight{{
			Callsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh,
			SelectedRunwayGroup: &group, SelectedSTARFamily: &starFamily, SelectedFeederFix: &feederFix,
			FeederETA: &aman.FeederETAState{ETA: &feederETA, Source: aman.FeederETASourceManual}, DerivedFeederETA: &aman.FeederETAState{ETA: &derivedFeederETA, Source: aman.FeederETASourceRoute},
			ActiveRouteFact:   &aman.RouteFact{Fix: "TNO", State: aman.RouteFactActive},
			LatestObservation: &aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AircraftType: &aircraftType, WakeCategory: &wakeCategory, FiledRoute: &filedRoute, SourceStatus: aman.DataFresh, ReconciledAt: now, Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.7, LongitudeDegrees: 12.2, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &observed}},
			Prediction:        &aman.Prediction{RawTETA: now.Add(20 * time.Minute), RawRETA: timePointer(now.Add(19 * time.Minute)), OperationalTETA: now.Add(18 * time.Minute), OperationalReason: aman.OperationalReasonSmoothed, GeneratedAt: now, InputObservedAt: observed, Confidence: aman.ConfidenceHigh, Publishable: true, DatasetVersion: "2607", GeometryDigest: "digest", ModelVersion: "model", ConfigVersion: "config", Basis: aman.PredictionBasisPerformanceWind, Sources: []string{"vatsim"}, Calculation: &aman.PredictionCalculation{NoWindDuration: 18 * time.Minute, Duration: 20 * time.Minute, Legs: []aman.PredictionLeg{{ID: "leg-1", From: "SOK", To: "SOK-HF", StartLatitude: 55.7, StartLongitude: 12.2, EndLatitude: 55.6, EndLongitude: 12.4, DistanceNM: 15, CourseTrueDegrees: 120, NoWindDuration: 18 * time.Minute, Duration: 20 * time.Minute}}, Segments: []aman.PredictionSegment{{RouteLegIndex: 0, PhaseID: "fl100_to_fl050", PhaseName: "Segment 4 · FL100 → FL050", PhaseFormula: "time = distance ÷ (TAS from 250 kt IAS + wind)", DistanceNM: 15, CourseTrueDegrees: 120, StartAltitudeFeet: 9000, EndAltitudeFeet: 4000, AltitudeFeet: 6500, NoWindGroundspeedKnots: 250, GroundspeedKnots: 230, NoWindDuration: 18 * time.Minute, Duration: 20 * time.Minute}}}},
			Slot:              &aman.Slot{Time: now.Add(17 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 14, Reason: "rate_wtc"}, FreezeReason: aman.FreezeNone, QueueOffers: []aman.QueueOffer{},
		}},
	}
	mux := http.NewServeMux()
	New(testAuth{}, stateReader{state: state}).RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodGet, "/aman/airports/EKCH/flights/SAS123/detail", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var detail flightDetail
	require.NoError(t, json.NewDecoder(response.Body).Decode(&detail))
	require.Equal(t, "SAS123", detail.Flight.Callsign)
	require.Equal(t, "ESSA", detail.Flight.Origin)
	require.Equal(t, "EKCH", detail.Flight.Destination)
	require.Equal(t, feederFix, *detail.Flight.FeederFix)
	require.Equal(t, "TNO", *detail.Flight.DirectTo)
	require.Equal(t, "2026-07-23T10:11:00.000Z", *detail.Flight.FeederETA)
	require.Equal(t, "2026-07-23T10:10:00.000Z", *detail.Flight.DerivedFeederETA)
	require.Equal(t, "A320", *detail.Flight.AircraftType)
	require.Equal(t, "M", *detail.Flight.WakeCategory)
	require.Equal(t, filedRoute, *detail.Flight.FiledRoute)
	require.NotNil(t, detail.Position)
	require.NotNil(t, detail.Calculation)
	require.EqualValues(t, 1200, detail.Calculation.Legs[0].DurationSeconds)
	require.EqualValues(t, 1080, *detail.Calculation.Legs[0].NoWindDurationSeconds)
	require.Len(t, detail.Calculation.Segments, 1)
	require.Equal(t, 0, detail.Calculation.Segments[0].RouteLegIndex)
	require.Equal(t, "fl100_to_fl050", detail.Calculation.Segments[0].PhaseID)
	require.Equal(t, "Segment 4 · FL100 → FL050", detail.Calculation.Segments[0].PhaseName)
	require.EqualValues(t, 230, detail.Calculation.Segments[0].GroundspeedKnots)
	require.EqualValues(t, 1200, detail.Calculation.Segments[0].DurationSeconds)
	require.NotNil(t, detail.TETABasis)
	require.Equal(t, "performance_wind", detail.TETABasis.PredictionBasis)
	require.Equal(t, "smoothed", detail.TETABasis.OperationalReason)
	require.NotNil(t, detail.SlotBasis)
	require.Equal(t, "rate_wtc", detail.SlotBasis.Reason)
	require.EqualValues(t, 30, detail.SlotBasis.RatePerHour)
	require.True(t, detail.SlotBasis.Infeasible)
}

func TestFlightDetailDoesNotExposeNonPublishablePrediction(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	state := aman.AirportState{Airport: "EKCH", GeneratedAt: now, Mode: aman.ModeShadow, Flights: []aman.AMANFlight{{Callsign: "SAS123", State: aman.StateAirborne, DataStatus: aman.DataFresh, Prediction: &aman.Prediction{RawTETA: now.Add(time.Hour), OperationalTETA: now.Add(time.Hour), OperationalReason: aman.OperationalReasonPredicted, GeneratedAt: now, InputObservedAt: now, Confidence: aman.ConfidenceLow, DatasetVersion: "2607", GeometryDigest: "digest", ModelVersion: "model", ConfigVersion: "config", Sources: []string{}, Publishable: false}, FreezeReason: aman.FreezeNone, QueueOffers: []aman.QueueOffer{}}}}
	mux := http.NewServeMux()
	New(testAuth{}, stateReader{state: state}).RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodGet, "/aman/airports/EKCH/flights/SAS123/detail", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var detail flightDetail
	require.NoError(t, json.NewDecoder(response.Body).Decode(&detail))
	require.Nil(t, detail.Calculation)
	require.Nil(t, detail.TETABasis)
}

func TestMapHoldingPlanExposesEntryDurationAndRelease(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	mapped, err := mapHoldingPlan(aman.HoldingPlan{HoldingEntryTime: now.Add(10 * time.Minute), ApproachReleaseTime: now.Add(18 * time.Minute), ExpectedHoldingDuration: 8 * time.Minute, PostHoldingTransit: 10 * time.Minute})
	require.NoError(t, err)
	require.Equal(t, "2026-07-23T10:10:00.000Z", mapped.HoldingEntryTime)
	require.Equal(t, "2026-07-23T10:18:00.000Z", mapped.ApproachReleaseTime)
	require.EqualValues(t, 480, mapped.ExpectedHoldingSeconds)
	require.EqualValues(t, 600, mapped.PostHoldingTransitSeconds)
}

func TestFlightDetailExposesInitialAndScheduledFeederTimes(t *testing.T) {
	now := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.UTC)
	feeder := "ROSBI"
	initialETA, initialSTA := now.Add(5*time.Minute), now.Add(8*time.Minute)
	flight := aman.AMANFlight{
		Callsign: "SAS123", State: aman.StateStable, SelectedFeederFix: &feeder,
		InitialTiming: &aman.FlightInitialTiming{FeederETA: &initialETA, FeederSTA: &initialSTA, RunwayETA: timePointer(now.Add(15 * time.Minute)), RunwaySTA: timePointer(now.Add(18 * time.Minute))},
		Prediction:    &aman.Prediction{Publishable: true, RawTETA: now.Add(20 * time.Minute), OperationalTETA: now.Add(20 * time.Minute), GeneratedAt: now, InputObservedAt: now, Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: feeder, Duration: 5 * time.Minute}, {To: "EKCH", Duration: 10 * time.Minute}}}},
		Slot:          &aman.Slot{Time: now.Add(25 * time.Minute)},
	}
	mapped, err := New(nil, nil).mapDetail(context.Background(), aman.AirportState{Airport: "EKCH", GeneratedAt: now}, flight)
	require.NoError(t, err)
	require.NotNil(t, mapped.InitialTiming)
	require.Equal(t, "2026-10-05T09:05:00.000Z", *mapped.InitialTiming.FeederETA)
	require.Equal(t, "2026-10-05T09:08:00.000Z", *mapped.InitialTiming.FeederSTA)
	require.Equal(t, "2026-10-05T09:15:00.000Z", *mapped.InitialTiming.RunwayETA)
	require.Equal(t, "2026-10-05T09:18:00.000Z", *mapped.InitialTiming.RunwaySTA)
	require.Equal(t, "2026-10-05T09:15:00.000Z", *mapped.Flight.FeederSTA)
}

func TestSTARRouteExcludesEnrouteApproachAndRunwayAndRetainsElapsedTime(t *testing.T) {
	now := time.Date(2026, time.October, 5, 9, 0, 0, 0, time.UTC)
	group, family, feeder := aman.RunwayGroupID("ARRIVAL-22L"), "TESPI", "TNO"
	api := New(nil, nil).WithTerminal(terminal.Configuration{
		ConfigVersion: "terminal-v1",
		FixAliases:    []terminal.FixAlias{{Alias: "STAR-ALIAS", Canonical: "CH626"}},
		Paths: []terminal.Path{
			{Feeder: "TESPI", FeederFix: "TNO", RunwayGroup: "ARRIVAL-22R", Fixes: []navdata.FixID{"OTHER"}},
			{Feeder: "TESPI", FeederFix: "TNO", RunwayGroup: group, Fixes: []navdata.FixID{"TESPI", "TNO", "STAR-ALIAS", "ABEGI"}},
		},
	})
	flight := aman.AMANFlight{
		SelectedRunwayGroup: &group, SelectedSTARFamily: &family, SelectedFeederFix: &feeder,
		Prediction: &aman.Prediction{Publishable: true, ConfigVersion: "terminal-v1", InputObservedAt: now,
			Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{
				{To: "ENROUTE", Duration: 3 * time.Minute}, {To: "CH626", Duration: 2 * time.Minute},
				{To: "ABEGI", Duration: time.Minute}, {To: "CH2LF", Duration: time.Minute}, {To: "RWY-22L", Duration: time.Minute},
			}},
		},
	}
	require.Equal(t, []starWaypoint{{Fix: "CH626", ETA: "2026-10-05T09:05:00.000Z"}, {Fix: "ABEGI", ETA: "2026-10-05T09:06:00.000Z"}}, api.mapSTARRoute(flight))
	flight.Prediction.ConfigVersion = "old-config"
	require.Empty(t, api.mapSTARRoute(flight), "a different configuration must not classify prediction legs")
	flight.Prediction.ConfigVersion = "terminal-v1"
	*flight.SelectedFeederFix = "UNRESOLVED"
	require.Empty(t, api.mapSTARRoute(flight), "explicit feeder selection must not fall back to another STAR")
}

type stateReader struct{ state aman.AirportState }

func (r stateReader) LoadAirportState(_ context.Context, airport string) (aman.AirportState, error) {
	if airport != r.state.Airport {
		return aman.AirportState{}, errors.New("not found")
	}
	return r.state, nil
}

type testAuth struct{}

func (testAuth) Validate(string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("1234567", 0, nil), nil
}
func timePointer(value time.Time) *time.Time { return &value }
