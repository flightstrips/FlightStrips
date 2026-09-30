package cdm

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
)

// CandidateSequenceInput contains accepted projection/configuration values.
// The endpoint preserves the existing public JSON and persisted-row policy.
type CandidateSequenceInput struct {
	Session *models.Session
	Strips  []*models.Strip
	Config  *CdmAirportConfig
}

type CandidateWebAPI struct {
	auth shared.AuthenticationService
	read func(context.Context) ([]CandidateSequenceInput, error)
	Now  func() time.Time
}

func NewCandidateWebAPI(auth shared.AuthenticationService, read func(context.Context) ([]CandidateSequenceInput, error)) (*CandidateWebAPI, error) {
	if auth == nil || read == nil {
		return nil, fmt.Errorf("CDM HTTP requires authentication and accepted projection reader")
	}
	return &CandidateWebAPI{auth: auth, read: read, Now: time.Now}, nil
}

func (a *CandidateWebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/cdm/sequence", a.sequence)
}

func (a *CandidateWebAPI) sequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if _, ok := (&WebAPI{authenticationService: a.auth}).authenticate(w, r); !ok {
		return
	}
	inputs, err := a.read(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "cdm sequence unavailable")
		return
	}
	now := a.Now().UTC()
	response := sequenceResponse{GeneratedAt: now.Format(time.RFC3339), Sessions: []sequenceSessionResponse{}}
	for _, input := range inputs {
		if input.Session == nil || input.Config == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "cdm sequence unavailable")
			return
		}
		s := input.Session
		result := sequenceSessionResponse{SessionID: s.ID, Name: s.Name, Airport: s.Airport, DepartureRunways: append([]string(nil), s.ActiveRunways.DepartureRunways...), ArrivalRunways: append([]string(nil), s.ActiveRunways.ArrivalRunways...), DepartureRates: []departureRateResponse{}, Rows: []sequenceRowResponse{}}
		seen := map[string]bool{}
		for _, runway := range result.DepartureRunways {
			runway = strings.ToUpper(strings.TrimSpace(runway))
			if runway != "" && !seen[runway] {
				result.DepartureRates = append(result.DepartureRates, departureRateResponse{Runway: runway, DeparturesHour: input.Config.RateForRunway(runway)})
				seen[runway] = true
			}
		}
		sort.Slice(result.DepartureRates, func(i, j int) bool { return result.DepartureRates[i].Runway < result.DepartureRates[j].Runway })
		for _, row := range buildPersistedSequenceRows(input.Strips, true, now) {
			result.Rows = append(result.Rows, row.response)
		}
		response.Sessions = append(response.Sessions, result)
	}
	sort.SliceStable(response.Sessions, func(i, j int) bool {
		l, r := response.Sessions[i], response.Sessions[j]
		if l.Airport != r.Airport {
			return l.Airport < r.Airport
		}
		if l.Name != r.Name {
			return l.Name < r.Name
		}
		return l.SessionID < r.SessionID
	})
	writeJSON(w, http.StatusOK, response)
}
