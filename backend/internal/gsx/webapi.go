// Package gsx publishes the operational stand for a callsign to the GSX
// handler script running inside a pilot's simulator.
//
// The endpoint exists because GSX's scripting API can only issue plain HTTP
// GET: fetchJson(url, timeout, etag) sends no headers, so the Bearer token the
// rest of the web API relies on cannot reach it. It is therefore
// unauthenticated and deliberately minimal — it answers "which stand does this
// callsign hold right now", and nothing else. No route, no CID, no PDC state,
// no flight plan. Callsign and position are already public on the VATSIM
// datafeed and the stand is passed to the pilot over the air, so the endpoint
// discloses nothing that is not already available.
//
// It reads strips.stand, which is the authoritative operational value: both
// SAT's automatic allocation and a controller's manual override converge there.
package gsx

import (
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"database/sql"

	"github.com/jackc/pgx/v5"
)

// liveSessionName is the only session a public consumer may read. Sweatbox and
// playback sessions must never reach a pilot's simulator.
const liveSessionName = "LIVE"

// SessionLookup is the slice of repository.SessionRepository this API needs.
type SessionLookup interface {
	GetByNameAndAirport(ctx context.Context, name string, airport string) (*models.Session, error)
	GetByNames(ctx context.Context, name string) ([]*models.Session, error)
}

// StripLookup is the slice of repository.StripRepository this API needs.
type StripLookup interface {
	GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error)
}

type WebAPI struct {
	sessions SessionLookup
	strips   StripLookup
	// liveOnly restricts lookups to LIVE sessions. Always true in a live
	// environment; development environments may search every session so the
	// feed can be exercised against a sweatbox.
	liveOnly bool
}

func NewWebAPI(sessions SessionLookup, strips StripLookup, liveOnly bool) *WebAPI {
	return &WebAPI{sessions: sessions, strips: strips, liveOnly: liveOnly}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/gsx/stand", a.handleStand)
}

// standResponse is the whole contract. revision is the value the handler script
// compares between polls: it is the stand itself, so an unrelated strip update
// does not look like a reassignment and make the script re-select the same
// stand.
type standResponse struct {
	Stand    *string `json:"stand"`
	Revision string  `json:"revision"`
}

const noStandRevision = "none"

func (a *WebAPI) handleStand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	callsign := normalizeCallsign(r.URL.Query().Get("callsign"))
	if callsign == "" {
		writeJSONError(w, http.StatusBadRequest, "callsign is required")
		return
	}

	airport := normalizeAirport(r.URL.Query().Get("icao"))
	if airport == "" && strings.TrimSpace(r.URL.Query().Get("icao")) != "" {
		writeJSONError(w, http.StatusBadRequest, "icao must be a 3-4 character airport code")
		return
	}

	stand, err := a.lookupStand(r.Context(), callsign, airport)
	if err != nil {
		// A lookup failure is not the pilot's problem. Answer 503 so the script
		// keeps its current stand and retries on the next poll.
		writeJSONError(w, http.StatusServiceUnavailable, "stand lookup unavailable")
		return
	}

	response := standResponse{Stand: stand, Revision: noStandRevision}
	if stand != nil {
		response.Revision = *stand
	}

	// The script polls with fetchJson(..., etag=True), so an unchanged stand
	// costs a 304 with no body.
	etag := `"` + response.Revision + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if matchesETag(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	writeJSON(w, http.StatusOK, response)
}

// lookupStand returns the stand held by callsign, or nil when the callsign is
// not on a strip or holds no stand. Both are ordinary answers: most aircraft at
// the airport are not being tracked by this feed.
func (a *WebAPI) lookupStand(ctx context.Context, callsign, airport string) (*string, error) {
	sessions, err := a.candidateSessions(ctx, airport)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		if session == nil {
			continue
		}
		// Belt and braces: candidateSessions already filters, but a public feed
		// must never read a sweatbox or playback strip.
		if a.liveOnly && !strings.EqualFold(session.Name, liveSessionName) {
			continue
		}
		if airport != "" && !strings.EqualFold(session.Airport, airport) {
			continue
		}

		strip, lookupErr := a.strips.GetByCallsign(ctx, session.ID, callsign)
		if lookupErr != nil {
			if errors.Is(lookupErr, sql.ErrNoRows) || errors.Is(lookupErr, pgx.ErrNoRows) {
				continue
			}
			return nil, lookupErr
		}
		if strip == nil || strip.Stand == nil {
			continue
		}
		stand := strings.TrimSpace(*strip.Stand)
		if stand == "" {
			continue
		}
		return &stand, nil
	}

	return nil, nil
}

func (a *WebAPI) candidateSessions(ctx context.Context, airport string) ([]*models.Session, error) {
	if airport != "" {
		session, err := a.sessions.GetByNameAndAirport(ctx, liveSessionName, airport)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return nil, err
		}
		if session != nil {
			return []*models.Session{session}, nil
		}
		if a.liveOnly {
			return nil, nil
		}
	}
	return a.sessions.GetByNames(ctx, liveSessionName)
}

func normalizeCallsign(raw string) string {
	callsign := strings.ToUpper(strings.TrimSpace(raw))
	if len(callsign) == 0 || len(callsign) > 12 {
		return ""
	}
	for _, r := range callsign {
		isDigit := r >= '0' && r <= '9'
		isLetter := r >= 'A' && r <= 'Z'
		if !isDigit && !isLetter {
			return ""
		}
	}
	return callsign
}

func normalizeAirport(raw string) string {
	airport := strings.ToUpper(strings.TrimSpace(raw))
	if len(airport) < 3 || len(airport) > 4 {
		return ""
	}
	for _, r := range airport {
		isDigit := r >= '0' && r <= '9'
		isLetter := r >= 'A' && r <= 'Z'
		if !isDigit && !isLetter {
			return ""
		}
	}
	return airport
}

// matchesETag implements the If-None-Match comparison this endpoint needs:
// a list of entity tags, or "*". Weak comparison is correct for a cache
// revalidation on a GET.
func matchesETag(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	if header == "*" {
		return true
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
