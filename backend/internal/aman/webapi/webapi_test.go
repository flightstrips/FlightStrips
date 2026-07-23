package webapi

import (
	"FlightStrips/internal/aman"
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

func TestFlightDetailReturnsOnDemandCalculationAndOperationalBasis(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	observed := now.Add(-time.Minute)
	groundspeed := 280.0
	altitude := 9000
	aircraftType, wakeCategory := "A320", "M"
	state := aman.AirportState{
		Airport: "EKCH", Revision: 14, GeneratedAt: now, PolicyVersion: "test", Mode: aman.ModeShadow,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 30}},
		Flights: []aman.AMANFlight{{
			ID: "flight-123", VATSIMCID: "1234567", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh,
			SelectedRunwayGroup: &group, LatestObservation: &aman.FlightObservation{FlightID: "flight-123", VATSIMCID: "1234567", Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AircraftType: &aircraftType, WakeCategory: &wakeCategory, SourceStatus: aman.DataFresh, ReconciledAt: now, Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.7, LongitudeDegrees: 12.2, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &observed}},
			Prediction: &aman.Prediction{RawTETA: now.Add(20 * time.Minute), RawRETA: timePointer(now.Add(19 * time.Minute)), OperationalTETA: now.Add(18 * time.Minute), OperationalReason: aman.OperationalReasonSmoothed, GeneratedAt: now, InputObservedAt: observed, Confidence: aman.ConfidenceHigh, Publishable: true, DatasetVersion: "2607", GeometryDigest: "digest", ModelVersion: "model", ConfigVersion: "config", Sources: []string{"vatsim"}, Calculation: &aman.PredictionCalculation{NoWindDuration: 18 * time.Minute, Duration: 20 * time.Minute, Legs: []aman.PredictionLeg{{ID: "leg-1", From: "SOK", To: "SOK-HF", StartLatitude: 55.7, StartLongitude: 12.2, EndLatitude: 55.6, EndLongitude: 12.4, DistanceNM: 15, CourseTrueDegrees: 120, NoWindDuration: 18 * time.Minute, Duration: 20 * time.Minute}}}},
			Slot:       &aman.Slot{Time: now.Add(17 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 14, Reason: "rate_wtc"}, FreezeReason: aman.FreezeNone, QueueOffers: []aman.QueueOffer{},
		}},
	}
	mux := http.NewServeMux()
	New(testAuth{}, stateReader{state: state}).RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodGet, "/aman/airports/EKCH/flights/flight-123/detail", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var detail flightDetail
	require.NoError(t, json.NewDecoder(response.Body).Decode(&detail))
	require.Equal(t, "SAS123", detail.Flight.Callsign)
	require.Equal(t, "A320", *detail.Flight.AircraftType)
	require.Equal(t, "M", *detail.Flight.WakeCategory)
	require.NotNil(t, detail.Position)
	require.NotNil(t, detail.Calculation)
	require.EqualValues(t, 1200, detail.Calculation.Legs[0].DurationSeconds)
	require.EqualValues(t, 1080, *detail.Calculation.Legs[0].NoWindDurationSeconds)
	require.NotNil(t, detail.TETABasis)
	require.Equal(t, "smoothed", detail.TETABasis.OperationalReason)
	require.NotNil(t, detail.SlotBasis)
	require.Equal(t, "rate_wtc", detail.SlotBasis.Reason)
	require.EqualValues(t, 30, detail.SlotBasis.RatePerHour)
}

func TestFlightDetailDoesNotExposeNonPublishablePrediction(t *testing.T) {
	now := time.Date(2026, time.July, 23, 10, 0, 0, 0, time.UTC)
	state := aman.AirportState{Airport: "EKCH", GeneratedAt: now, Mode: aman.ModeShadow, Flights: []aman.AMANFlight{{ID: "flight-123", VATSIMCID: "1234567", CurrentCallsign: "SAS123", State: aman.StateAirborne, DataStatus: aman.DataFresh, Prediction: &aman.Prediction{RawTETA: now.Add(time.Hour), OperationalTETA: now.Add(time.Hour), OperationalReason: aman.OperationalReasonPredicted, GeneratedAt: now, InputObservedAt: now, Confidence: aman.ConfidenceLow, DatasetVersion: "2607", GeometryDigest: "digest", ModelVersion: "model", ConfigVersion: "config", Sources: []string{}, Publishable: false}, FreezeReason: aman.FreezeNone, QueueOffers: []aman.QueueOffer{}}}}
	mux := http.NewServeMux()
	New(testAuth{}, stateReader{state: state}).RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodGet, "/aman/airports/EKCH/flights/flight-123/detail", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	var detail flightDetail
	require.NoError(t, json.NewDecoder(response.Body).Decode(&detail))
	require.Nil(t, detail.Calculation)
	require.Nil(t, detail.TETABasis)
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
