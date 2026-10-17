package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pilotStandStub struct {
	state           *services.PilotDepartureStand
	opens           int
	selections      int
	flight          vatsim.DepartureFlightInfo
	allowUnsuitable bool
}

func (s *pilotStandStub) Open(_ context.Context, _ int32, _ *models.Strip, flight vatsim.DepartureFlightInfo) (*services.PilotDepartureStand, error) {
	s.opens++
	s.flight = flight
	s.state = &services.PilotDepartureStand{Stand: "A12", State: services.PilotStandAssigned, Version: 1}
	return s.state, nil
}

func (s *pilotStandStub) Read(_ context.Context, _ int32, flight vatsim.DepartureFlightInfo) (*services.PilotDepartureStand, error) {
	s.flight = flight
	return s.state, nil
}

func (s *pilotStandStub) Select(_ context.Context, _ int32, _ *models.Strip, flight vatsim.DepartureFlightInfo, stand string, version int32, allowUnsuitable bool) (*services.PilotDepartureStand, error) {
	s.allowUnsuitable = allowUnsuitable
	s.flight = flight
	s.selections++
	s.state = &services.PilotDepartureStand{Stand: stand, State: services.PilotStandSelected, Version: version + 1}
	return s.state, nil
}

func pilotStandAPI(t *testing.T, online bool, sessionName string) (*WebAPI, *pilotStandStub) {
	t.Helper()
	stands := &pilotStandStub{}
	api := NewWebAPI(WebAPIConfig{
		Auth: authStub{}, Network: networkTestFlight(t, online, time.Now(), "SAS123", "EKCH", "ESSA"),
		PilotStands: stands, Viff: &networkTimingStub{},
		Flights: &finderStub{match: pdc.WebStripMatch{SessionID: 7, Strip: &models.Strip{Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA"}}},
		Sessions: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
			return &models.Session{ID: 7, Name: sessionName, Airport: "EKCH"}, nil
		}},
	})
	return api, stands
}

func TestPilotStandReadRequiresExplicitFirstOpen(t *testing.T) {
	api, stands := pilotStandAPI(t, false, "LIVE")
	flightRequest := func() snapshot {
		req := httptest.NewRequest(http.MethodGet, "/efb/flight?callsign=OTHER", nil)
		req.Header.Set("Authorization", "Bearer token")
		rec := httptest.NewRecorder()
		api.handleFlight(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var result snapshot
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
		return result
	}
	result := flightRequest()
	assert.Equal(t, vatsim.FlightStatePrefile, result.FlightState)
	assert.True(t, result.Capabilities.StandInitialization)
	assert.Zero(t, stands.opens, "GET flight must not allocate")
	req := httptest.NewRequest(http.MethodPost, "/efb/stand/open?callsign=OTHER", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleStandOpen(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, 1, stands.opens)
	assert.Equal(t, "SAS123", stands.flight.Callsign)
	assert.Equal(t, "1234567", stands.flight.CID)
	result = flightRequest()
	require.NotNil(t, result.StandState)
	assert.Equal(t, services.PilotStandAssigned, *result.StandState)
	assert.True(t, result.Capabilities.Stand)
	assert.False(t, result.Capabilities.StandInitialization)
	req = httptest.NewRequest(http.MethodPut, "/efb/stand", strings.NewReader(`{"callsign":"OTHER","stand":"B4","version":1,"allow_unsuitable":true}`))
	req.Header.Set("Authorization", "Bearer token")
	rec = httptest.NewRecorder()
	api.handleStand(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, stands.selections)
	assert.True(t, stands.allowUnsuitable)
	assert.Equal(t, "SAS123", stands.flight.Callsign)
	assert.Contains(t, rec.Body.String(), `"stand_state":"SELECTED"`)
}

func TestPilotStandRejectsNonLiveAndEvaluationMutation(t *testing.T) {
	for _, source := range []string{"SWEATBOX", "PLAYBACK", "callsign lookup"} {
		t.Run(source, func(t *testing.T) {
			api, stands := pilotStandAPI(t, false, source)
			if source == "callsign lookup" {
				api.callsignLookup = true
			}
			req := httptest.NewRequest(http.MethodPost, "/efb/stand/open?callsign=SAS123", nil)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleStandOpen(rec, req)
			assert.NotEqual(t, http.StatusNoContent, rec.Code)
			assert.Zero(t, stands.opens)
		})
	}
}

func TestPilotStandOnlineSelectionForbidden(t *testing.T) {
	api, stands := pilotStandAPI(t, true, "LIVE")
	req := httptest.NewRequest(http.MethodPut, "/efb/stand", strings.NewReader(`{"stand":"A12","version":1}`))
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleStand(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Zero(t, stands.selections)
}

func TestPilotStandUnavailableDoesNotUsePhysicalStripStand(t *testing.T) {
	api, stands := pilotStandAPI(t, true, "SWEATBOX")
	req := httptest.NewRequest(http.MethodGet, "/efb/flight", nil)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	api.handleFlight(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"stand_state":"STAND ASSIGNMENT UNAVAILABLE"`)
	assert.Zero(t, stands.opens)
}

func TestPilotStandOpenRequiresAuthentication(t *testing.T) {
	api, stands := pilotStandAPI(t, false, "LIVE")
	req := httptest.NewRequest(http.MethodPost, "/efb/stand/open", nil)
	rec := httptest.NewRecorder()
	api.handleStandOpen(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Zero(t, stands.opens)
}

func TestPilotStandOpenRejectsArrivalAndTrainingSources(t *testing.T) {
	for _, training := range []bool{false, true} {
		t.Run(map[bool]string{false: "arrival", true: "training"}[training], func(t *testing.T) {
			api, stands := pilotStandAPI(t, false, "LIVE")
			url := "/efb/stand/open"
			if training {
				url += "?training_session=7"
			} else {
				api.network = networkTestFlight(t, true, time.Now(), "SAS123", "ESSA", "EKCH")
			}
			req := httptest.NewRequest(http.MethodPost, url, nil)
			req.Header.Set("Authorization", "Bearer token")
			rec := httptest.NewRecorder()
			api.handleStandOpen(rec, req)
			assert.GreaterOrEqual(t, rec.Code, 400)
			assert.Zero(t, stands.opens)
		})
	}
}
