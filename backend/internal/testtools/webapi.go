package testtools

import (
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

type WebAPI struct {
	service   *Service
	auth      shared.AuthenticationService
	readiness appconfig.StandAssignmentReadiness
}

func NewWebAPI(service *Service, auth shared.AuthenticationService, readiness appconfig.StandAssignmentReadiness) *WebAPI {
	return &WebAPI{service: service, auth: auth, readiness: readiness}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/test/status", a.authenticated(a.handleStatus))
	mux.HandleFunc("/test/sat/scenarios", a.authenticated(a.handleScenarios))
	mux.HandleFunc("/test/sat/scenarios/", a.authenticated(a.handleScenario))
	mux.HandleFunc("/test/sat/blocks", a.authenticated(a.handleBlocks))
}

func (a *WebAPI) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
		if header == "" || token == "" || token == header {
			writeError(w, http.StatusUnauthorized, "invalid authorization header")
			return
		}
		if _, err := a.auth.Validate(token); err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, r)
	}
}

func (a *WebAPI) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	sessions, err := a.service.Sessions(r.Context())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":        true,
		"simulated_time": a.service.Now(),
		"sessions":       sessions,
		"sat":            map[string]any{"enabled": a.readiness.Enabled, "ready": a.readiness.Ready, "reason": a.readiness.Reason},
	})
}

func (a *WebAPI) handleScenarios(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessionID, _ := strconv.ParseInt(r.URL.Query().Get("session_id"), 10, 32)
		scenarios, err := a.service.ListScenarios(r.Context(), int32(sessionID))
		if err != nil {
			writeServiceError(w, err)
			return
		}
		blocks, err := a.service.Blocks(r.Context(), int32(sessionID))
		if err != nil && sessionID != 0 {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"scenarios": scenarios, "blocks": blocks, "simulated_time": a.service.Now()})
	case http.MethodPost:
		var request CreateScenarioRequest
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scenario, err := a.service.CreateScenario(r.Context(), request)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, scenario)
	case http.MethodDelete:
		if err := a.service.Reset(r.Context()); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"reset": true, "simulated_time": a.service.Now()})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *WebAPI) handleScenario(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/test/sat/scenarios/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := a.service.DeleteScenario(r.Context(), parts[0]); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "commands" && r.Method == http.MethodPost {
		var command ScenarioCommand
		if err := decodeJSON(r, &command); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		scenario, err := a.service.Command(r.Context(), parts[0], command)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, scenario)
		return
	}
	http.NotFound(w, r)
}

func (a *WebAPI) handleBlocks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var request struct {
			SessionID int32  `json:"session_id"`
			Stand     string `json:"stand"`
			Reason    string `json:"reason"`
		}
		if err := decodeJSON(r, &request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		block, err := a.service.CreateBlock(r.Context(), request.SessionID, request.Stand, request.Reason)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, block)
	case http.MethodDelete:
		sessionID, sessionErr := strconv.ParseInt(r.URL.Query().Get("session_id"), 10, 32)
		id, idErr := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		version, versionErr := strconv.ParseInt(r.URL.Query().Get("version"), 10, 32)
		if sessionErr != nil || idErr != nil || versionErr != nil {
			writeError(w, http.StatusBadRequest, "session_id, id, and version are required")
			return
		}
		if err := a.service.DeleteBlock(r.Context(), int32(sessionID), id, int32(version)); err != nil {
			writeServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func decodeJSON(r *http.Request, destination any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(destination)
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
