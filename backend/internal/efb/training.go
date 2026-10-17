package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/shared"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

var errTrainingForbidden = errors.New("Sweatbox mode is unavailable or the selected session is not Sweatbox")
var errTrainingSelection = errors.New("a valid Sweatbox session and callsign are required")

func hasTrainingSession(r *http.Request) bool {
	return r.URL.Query().Has("training_session")
}

func isSweatbox(session *models.Session) bool {
	return session != nil && strings.EqualFold(strings.TrimSpace(session.Name), "Sweatbox")
}

func (a *WebAPI) handleTrainingSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := a.authenticate(w, r); !ok {
		return
	}
	if !a.training || a.sessions == nil {
		writeError(w, http.StatusForbidden, errTrainingForbidden.Error())
		return
	}
	sessions, err := a.sessions.List(r.Context())
	if err != nil {
		slog.WarnContext(r.Context(), "EFB Sweatbox sessions lookup failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "Sweatbox sessions unavailable")
		return
	}
	type trainingSession struct {
		ID      int32  `json:"id"`
		Name    string `json:"name"`
		Airport string `json:"airport"`
	}
	result := make([]trainingSession, 0)
	for _, session := range sessions {
		if isSweatbox(session) {
			result = append(result, trainingSession{session.ID, session.Name, session.Airport})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": result})
}

func (a *WebAPI) resolveRequestedFlight(r *http.Request, user shared.AuthenticatedUser, callsign string) (pdc.WebStripMatch, *models.Session, error) {
	if hasTrainingSession(r) {
		return a.resolveTrainingFlight(r, callsign)
	}
	return a.resolveFlight(r.Context(), user, callsign)
}

func (a *WebAPI) resolveTrainingFlight(r *http.Request, callsign string) (pdc.WebStripMatch, *models.Session, error) {
	if !a.training || a.sessions == nil || a.trainingStrips == nil {
		return pdc.WebStripMatch{}, nil, errTrainingForbidden
	}
	id, err := strconv.ParseInt(r.URL.Query().Get("training_session"), 10, 32)
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if err != nil || id <= 0 || callsign == "" {
		return pdc.WebStripMatch{}, nil, errTrainingSelection
	}
	session, err := a.sessions.GetByID(r.Context(), int32(id))
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	if !isSweatbox(session) {
		return pdc.WebStripMatch{}, nil, errTrainingForbidden
	}
	strip, err := a.trainingStrips.GetByCallsign(r.Context(), session.ID, callsign)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) || (err == nil && strip == nil) {
		return pdc.WebStripMatch{}, nil, pdc.ErrWebStripNotFound
	}
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	return pdc.WebStripMatch{SessionID: session.ID, Strip: strip}, session, nil
}

func (a *WebAPI) handleTrainingFlight(w http.ResponseWriter, r *http.Request) {
	match, session, err := a.resolveTrainingFlight(r, r.URL.Query().Get("callsign"))
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	result := a.buildSnapshot(r.Context(), match, session)
	result.Capabilities = capabilities{TOBT: a.cdm != nil && a.cdmReady && result.Phase == "DEPARTURE", Stand: a.stands != nil && a.assignments != nil}
	result.PDCState, result.PDCAvailable, result.PDCCanSubmit, result.PDCRequiresPilotAction, result.PDCClearanceText = "", false, false, false, nil
	// Keep the departure's sensed stand while loading assignment concurrency metadata.
	stand, state := result.Stand, result.StandState
	a.addNetworkStandAssignment(r.Context(), &result, match)
	if result.Phase == "DEPARTURE" {
		result.Stand, result.StandState = stand, state
	}
	applyDepartureRules(&result)
	writeJSON(w, http.StatusOK, result)
}

func (a *WebAPI) handleTrainingTobt(w http.ResponseWriter, r *http.Request, user shared.AuthenticatedUser, callsign, tobt string) {
	match, session, err := a.resolveTrainingFlight(r, callsign)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if a.cdm == nil || !a.cdmReady {
		writeError(w, http.StatusServiceUnavailable, "training CDM updates unavailable")
		return
	}
	if !strings.EqualFold(match.Strip.Origin, session.Airport) {
		writeError(w, http.StatusConflict, "TOBT can only be updated for departures in this Sweatbox session")
		return
	}
	if err := a.cdm.HandleTobtUpdate(r.Context(), session.ID, match.Strip.Callsign, tobt, "EFB:"+user.GetCid(), "PILOT"); err != nil {
		slog.WarnContext(r.Context(), "EFB Sweatbox TOBT update failed", "session", session.ID, "callsign", match.Strip.Callsign, "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"callsign": match.Strip.Callsign, "tobt": tobt})
}
