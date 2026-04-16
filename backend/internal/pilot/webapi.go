package pilot

import (
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrFlightNotFound    = errors.New("no flight found for callsign")
	ErrAmbiguousCallsign = errors.New("callsign matched multiple sessions")
)

// FlightInfo describes the current state of a pilot's flight including any PDC information.
type FlightInfo struct {
	Callsign               string  `json:"callsign"`
	Origin                 string  `json:"origin"`
	Destination            string  `json:"destination"`
	IsDeparture            bool    `json:"is_departure"`
	Cleared                bool    `json:"cleared"`
	PdcAvailable           bool    `json:"pdc_available"`
	PdcCanSubmit           bool    `json:"pdc_can_submit"`
	PdcState               string  `json:"pdc_state"`
	PdcClearanceText       *string `json:"pdc_clearance_text,omitempty"`
	PdcRequestRemarks      *string `json:"pdc_request_remarks,omitempty"`
	PdcAcknowledgedAt      *string `json:"pdc_acknowledged_at,omitempty"`
	PdcRequiresPilotAction bool    `json:"pdc_requires_pilot_action"`
}

// FlightLookup resolves a callsign to its current FlightInfo.
type FlightLookup interface {
	GetFlightInfo(ctx context.Context, callsign string) (*FlightInfo, error)
}

type CallsignLookup interface {
	GetCallsignByCID(ctx context.Context, cid string) (string, bool, error)
}

type WebAPI struct {
	authenticationService      shared.AuthenticationService
	callsignLookup             CallsignLookup
	flightLookup               FlightLookup
	requireLiveCIDVerification bool
}

type pilotMeResponse struct {
	CID            string  `json:"cid"`
	OnlineCallsign *string `json:"online_callsign,omitempty"`
	CallsignLocked bool    `json:"callsign_locked"`
	LiveMode       bool    `json:"live_mode"`
}

func NewWebAPI(authenticationService shared.AuthenticationService, callsignLookup CallsignLookup, flightLookup FlightLookup, requireLiveCIDVerification bool) *WebAPI {
	return &WebAPI{
		authenticationService:      authenticationService,
		callsignLookup:             callsignLookup,
		flightLookup:               flightLookup,
		requireLiveCIDVerification: requireLiveCIDVerification,
	}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/pilot/me", a.handleMe)
	mux.HandleFunc("/pilot/flight", a.handleFlight)
}

func (a *WebAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	response := pilotMeResponse{
		CID:            user.GetCid(),
		CallsignLocked: false,
		LiveMode:       a.requireLiveCIDVerification,
	}

	if a.requireLiveCIDVerification && a.callsignLookup != nil {
		callsign, found, err := a.callsignLookup.GetCallsignByCID(r.Context(), user.GetCid())
		if err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "pilot lookup unavailable")
			return
		}
		if found {
			response.OnlineCallsign = &callsign
			response.CallsignLocked = true
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *WebAPI) handleFlight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	callsign := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("callsign")))
	if callsign == "" {
		writeJSONError(w, http.StatusBadRequest, "callsign is required")
		return
	}

	if a.requireLiveCIDVerification {
		if a.callsignLookup == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "callsign verification unavailable")
			return
		}
		onlineCallsign, found, err := a.callsignLookup.GetCallsignByCID(r.Context(), user.GetCid())
		if err != nil {
			writeJSONError(w, http.StatusServiceUnavailable, "callsign verification unavailable")
			return
		}
		if !found || !strings.EqualFold(onlineCallsign, callsign) {
			writeJSONError(w, http.StatusForbidden, "you do not currently own this callsign on VATSIM")
			return
		}
	}

	if a.flightLookup == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
		return
	}

	info, err := a.flightLookup.GetFlightInfo(r.Context(), callsign)
	if err != nil {
		switch {
		case errors.Is(err, ErrFlightNotFound):
			writeJSONError(w, http.StatusNotFound, "no flight found for callsign")
		case errors.Is(err, ErrAmbiguousCallsign):
			writeJSONError(w, http.StatusConflict, "callsign matched multiple sessions")
		default:
			writeJSONError(w, http.StatusInternalServerError, "flight lookup failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, info)
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) (shared.AuthenticatedUser, bool) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
		return shared.AuthenticatedUser{}, false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token == authHeader || token == "" {
		writeJSONError(w, http.StatusUnauthorized, "invalid authorization header")
		return shared.AuthenticatedUser{}, false
	}

	user, err := a.authenticationService.Validate(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return shared.AuthenticatedUser{}, false
	}

	return user, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
