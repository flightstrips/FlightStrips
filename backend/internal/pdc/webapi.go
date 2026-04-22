package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type CallsignVerifier interface {
	VerifyPilotOwnsCallsign(ctx context.Context, cid string, callsign string) (bool, error)
}

type WebAPI struct {
	authenticationService      shared.AuthenticationService
	service                    *Service
	callsignVerifier           CallsignVerifier
	requireLiveCIDVerification bool
}

type requestWebPDCBody struct {
	Callsign     string `json:"callsign"`
	AircraftType string `json:"aircraft_type"`
	Atis         string `json:"atis"`
	Stand        string `json:"stand"`
	Remarks      string `json:"remarks"`
}

type acknowledgeWebPDCBody struct {
	Callsign string `json:"callsign"`
}

type webPDCStatusResponse struct {
	Callsign            string  `json:"callsign"`
	State               string  `json:"state"`
	RequestRemarks      *string `json:"request_remarks,omitempty"`
	ClearanceText       *string `json:"clearance_text,omitempty"`
	PilotAcknowledgedAt *string `json:"pilot_acknowledged_at,omitempty"`
	CanSubmit           bool    `json:"can_submit"`
	RequiresPilotAction bool    `json:"requires_pilot_action"`
}

func NewWebAPI(authenticationService shared.AuthenticationService, service *Service, callsignVerifier CallsignVerifier, requireLiveCIDVerification bool) *WebAPI {
	return &WebAPI{
		authenticationService:      authenticationService,
		service:                    service,
		callsignVerifier:           callsignVerifier,
		requireLiveCIDVerification: requireLiveCIDVerification,
	}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/pdc/request", a.handleRequest)
	mux.HandleFunc("/pdc/status", a.handleStatus)
	mux.HandleFunc("/pdc/acknowledge", a.handleAcknowledge)
	mux.HandleFunc("/pdc/unable", a.handleUnable)
}

func (a *WebAPI) handleRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	var body requestWebPDCBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !a.authorizeCallsign(w, r.Context(), user, body.Callsign) {
		return
	}

	if err := a.service.SubmitWebPDCRequest(r.Context(), body.Callsign, body.Atis, body.Stand, body.Remarks, body.AircraftType); err != nil {
		status, message := mapWebRequestError(err)
		writeJSONError(w, status, message)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"callsign": strings.ToUpper(strings.TrimSpace(body.Callsign)),
	})
}

func (a *WebAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	callsign := r.URL.Query().Get("callsign")
	if !a.authorizeCallsign(w, r.Context(), user, callsign) {
		return
	}

	match, err := a.service.FindWebStripByCallsign(r.Context(), callsign)
	if err != nil {
		status, message := mapWebRequestError(err)
		writeJSONError(w, status, message)
		return
	}

	if !isWebPDCRequest(match.Strip) {
		writeJSONError(w, http.StatusNotFound, "no web PDC request found for callsign")
		return
	}

	writeJSON(w, http.StatusOK, buildWebPDCStatus(match.Strip))
}

func (a *WebAPI) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	var body acknowledgeWebPDCBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !a.authorizeCallsign(w, r.Context(), user, body.Callsign) {
		return
	}

	match, err := a.service.FindWebStripByCallsign(r.Context(), body.Callsign)
	if err != nil {
		status, message := mapWebRequestError(err)
		writeJSONError(w, status, message)
		return
	}

	if !isWebPDCRequest(match.Strip) {
		writeJSONError(w, http.StatusNotFound, "no web PDC request found for callsign")
		return
	}

	if match.Strip.PdcState != string(StateCleared) && match.Strip.PdcState != string(StateConfirmed) {
		writeJSONError(w, http.StatusConflict, "web PDC clearance is not ready to acknowledge")
		return
	}

	if err := a.service.confirmPilotAcknowledgement(
		r.Context(),
		match.SessionID,
		match.Strip,
		shared.BAY_CLEARED,
		valueOrEmpty(match.Strip.PdcData.IssuedByCid),
		true,
	); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to confirm clearance")
		return
	}
	writeJSON(w, http.StatusOK, buildWebPDCStatus(match.Strip))
}

func (a *WebAPI) handleUnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}

	var body acknowledgeWebPDCBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !a.authorizeCallsign(w, r.Context(), user, body.Callsign) {
		return
	}

	match, err := a.service.FindWebStripByCallsign(r.Context(), body.Callsign)
	if err != nil {
		status, message := mapWebRequestError(err)
		writeJSONError(w, status, message)
		return
	}

	if !isWebPDCRequest(match.Strip) {
		writeJSONError(w, http.StatusNotFound, "no web PDC request found for callsign")
		return
	}

	if match.Strip.PdcState != string(StateCleared) {
		writeJSONError(w, http.StatusConflict, "web PDC clearance cannot be rejected in its current state")
		return
	}

	if err := a.service.HandleUnable(r.Context(), strings.ToUpper(strings.TrimSpace(body.Callsign)), sessionInformation{id: match.SessionID}); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to process unable")
		return
	}

	match.Strip.PdcState = string(StateFailed)
	writeJSON(w, http.StatusOK, buildWebPDCStatus(match.Strip))
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

func (a *WebAPI) authorizeCallsign(w http.ResponseWriter, ctx context.Context, user shared.AuthenticatedUser, callsign string) bool {
	normalizedCallsign := strings.ToUpper(strings.TrimSpace(callsign))
	if normalizedCallsign == "" {
		writeJSONError(w, http.StatusBadRequest, "callsign is required")
		return false
	}

	if !a.requireLiveCIDVerification {
		return true
	}

	if a.callsignVerifier == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "callsign verification unavailable")
		return false
	}

	ok, err := a.callsignVerifier.VerifyPilotOwnsCallsign(ctx, user.GetCid(), normalizedCallsign)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to verify web PDC callsign ownership", slog.String("callsign", normalizedCallsign), slog.Any("error", err))
		writeJSONError(w, http.StatusServiceUnavailable, "callsign verification unavailable")
		return false
	}
	if !ok {
		writeJSONError(w, http.StatusForbidden, "you do not currently own this callsign on VATSIM")
		return false
	}

	return true
}

func mapWebRequestError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrWebInvalidAtis):
		return http.StatusBadRequest, "invalid ATIS letter"
	case errors.Is(err, ErrWebAircraftTypeRequired):
		return http.StatusBadRequest, "aircraft type is required"
	case errors.Is(err, ErrWebAircraftTypeMismatch):
		return http.StatusConflict, "aircraft type does not match the live strip"
	case errors.Is(err, ErrWebAlreadyRequested):
		return http.StatusConflict, "a web PDC has already been submitted for this aircraft"
	case errors.Is(err, ErrWebStripNotFound):
		return http.StatusNotFound, "no strip found for callsign"
	case errors.Is(err, ErrWebAmbiguousCallsign):
		return http.StatusConflict, "callsign matched multiple sessions"
	case errors.Is(err, ErrWebAlreadyCleared):
		return http.StatusConflict, "aircraft is already cleared"
	default:
		return http.StatusInternalServerError, fmt.Sprintf("request failed: %v", err)
	}
}

func presentedWebState(state string) string {
	if state == string(StateRequestedWithFaults) {
		return string(StateRequested)
	}
	return state
}

func buildWebPDCStatus(strip *models.Strip) webPDCStatusResponse {
	presentedState := presentedWebState(strip.PdcState)
	response := webPDCStatusResponse{
		Callsign:            strip.Callsign,
		State:               presentedState,
		RequestRemarks:      optionalString(valueOrEmpty(strip.PdcRequestRemarks)),
		CanSubmit:           WebPDCCanSubmit(strip.PdcState),
		RequiresPilotAction: presentedState == string(StateCleared),
	}

	if strip.PdcData == nil || strip.PdcData.Web == nil {
		return response
	}

	response.ClearanceText = strip.PdcData.Web.ClearanceText
	if strip.PdcData.Web.PilotAcknowledgedAt != nil {
		formatted := strip.PdcData.Web.PilotAcknowledgedAt.UTC().Format(time.RFC3339)
		response.PilotAcknowledgedAt = &formatted
	}

	return response
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
