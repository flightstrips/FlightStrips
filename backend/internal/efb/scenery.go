package efb

import (
	"FlightStrips/internal/scenery"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// PilotSceneryStore persists the airport scenery a pilot declared in the EFB.
type PilotSceneryStore interface {
	PilotScenery(ctx context.Context, cid int64, airport string) (string, error)
	SetPilotScenery(ctx context.Context, cid int64, airport, scenery string) error
}

type sceneryPayload struct {
	Airport string  `json:"airport"`
	Scenery *string `json:"scenery"`
}

// handleScenery reads (GET) or declares (PUT) the signed-in pilot's scenery at
// the EFB airport. Stand allocation uses it to avoid stands the scenery lacks.
func (a *WebAPI) handleScenery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	if a.sceneries == nil {
		writeError(w, http.StatusServiceUnavailable, "scenery preferences unavailable")
		return
	}
	cid, err := strconv.ParseInt(strings.TrimSpace(user.GetCid()), 10, 64)
	if err != nil || cid <= 0 {
		writeError(w, http.StatusForbidden, "a VATSIM CID is required")
		return
	}
	airport := a.airport()
	if !scenery.Supported(airport) {
		writeError(w, http.StatusNotFound, "no scenery choice for this airport")
		return
	}
	if r.Method == http.MethodPut {
		var body sceneryPayload
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil || body.Scenery == nil {
			writeError(w, http.StatusBadRequest, "scenery is required")
			return
		}
		declared, known := scenery.Lookup(airport, *body.Scenery)
		if !known {
			writeError(w, http.StatusBadRequest, "unknown scenery")
			return
		}
		if err := a.sceneries.SetPilotScenery(r.Context(), cid, airport, declared.ID); err != nil {
			slog.WarnContext(r.Context(), "EFB scenery update failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "scenery preferences unavailable")
			return
		}
		writeJSON(w, http.StatusOK, sceneryPayload{Airport: airport, Scenery: &declared.ID})
		return
	}
	value, err := a.sceneries.PilotScenery(r.Context(), cid, airport)
	if err != nil {
		slog.WarnContext(r.Context(), "EFB scenery lookup failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "scenery preferences unavailable")
		return
	}
	payload := sceneryPayload{Airport: airport}
	if declared, known := scenery.Lookup(airport, value); known {
		payload.Scenery = &declared.ID
	}
	writeJSON(w, http.StatusOK, payload)
}
