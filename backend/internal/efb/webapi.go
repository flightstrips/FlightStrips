package efb

import (
	"FlightStrips/internal/metar"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type CallsignLookup interface {
	GetCallsignByCID(context.Context, string) (string, bool, error)
}

type FlightFinder interface {
	FindWebStripByCallsign(context.Context, string) (pdc.WebStripMatch, error)
}

type CDMUpdater interface {
	HandleTobtUpdate(context.Context, int32, string, string, string, string) error
}

type ATISLookup interface {
	GetATIS(airport string, departure bool) *metar.ATIS
}

type DepartureFrequencyLookup interface {
	ComputeDepartureFrequencyForStripContext(context.Context, *models.Strip, int32) (*string, error)
}

type WebAPIConfig struct {
	Auth        shared.AuthenticationService
	Callsigns   CallsignLookup
	Flights     FlightFinder
	Sessions    repository.SessionRepository
	Assignments repository.StandAssignmentRepository
	CDM         CDMUpdater
	CDMReady    bool
	Stands      *services.StandActionService
	ATIS        ATISLookup
	Departures  DepartureFrequencyLookup
	PDCReady    bool
	Live        bool
}

type WebAPI struct {
	auth        shared.AuthenticationService
	callsigns   CallsignLookup
	flights     FlightFinder
	sessions    repository.SessionRepository
	assignments repository.StandAssignmentRepository
	cdm         CDMUpdater
	cdmReady    bool
	stands      *services.StandActionService
	atis        ATISLookup
	departures  DepartureFrequencyLookup
	pdcReady    bool
	live        bool
}

func NewWebAPI(cfg WebAPIConfig) *WebAPI {
	return &WebAPI{auth: cfg.Auth, callsigns: cfg.Callsigns, flights: cfg.Flights, sessions: cfg.Sessions, assignments: cfg.Assignments, cdm: cfg.CDM, cdmReady: cfg.CDMReady, stands: cfg.Stands, atis: cfg.ATIS, departures: cfg.Departures, pdcReady: cfg.PDCReady, live: cfg.Live}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/efb/me", a.handleMe)
	mux.HandleFunc("/efb/flight", a.handleFlight)
	mux.HandleFunc("/efb/tobt", a.handleTobt)
	mux.HandleFunc("/efb/stand", a.handleStand)
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
	callsign, found, err := a.lookupCallsign(r.Context(), user, r.URL.Query().Get("callsign"))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "pilot lookup unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cid": user.GetCid(), "live_mode": a.live, "online_callsign": optional(found, callsign), "callsign_locked": a.live && found})
}

type snapshot struct {
	Callsign               string       `json:"callsign"`
	AircraftType           *string      `json:"aircraft_type"`
	Origin                 string       `json:"origin"`
	Destination            string       `json:"destination"`
	Route                  *string      `json:"route"`
	Phase                  string       `json:"phase"`
	Runway                 *string      `json:"runway"`
	SID                    *string      `json:"sid"`
	STAR                   *string      `json:"star"`
	ClearedAltitude        *int32       `json:"cleared_altitude"`
	Squawk                 *string      `json:"squawk"`
	DepartureFrequency     *string      `json:"departure_frequency"`
	Stand                  *string      `json:"stand"`
	StandVersion           *int32       `json:"stand_version"`
	EOBT                   *string      `json:"eobt"`
	TOBT                   *string      `json:"tobt"`
	TSAT                   *string      `json:"tsat"`
	TTOT                   *string      `json:"ttot"`
	CTOT                   *string      `json:"ctot"`
	CDMStatus              *string      `json:"cdm_status"`
	PDCState               string       `json:"pdc_state"`
	PDCAvailable           bool         `json:"pdc_available"`
	PDCCanSubmit           bool         `json:"pdc_can_submit"`
	PDCRequiresPilotAction bool         `json:"pdc_requires_pilot_action"`
	PDCClearanceText       *string      `json:"pdc_clearance_text"`
	ATIS                   *metar.ATIS  `json:"atis"`
	Capabilities           capabilities `json:"capabilities"`
}

type capabilities struct {
	PDC   bool `json:"pdc"`
	TOBT  bool `json:"tobt_update"`
	Stand bool `json:"stand_reassignment"`
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
	match, session, err := a.resolveFlight(r.Context(), user, r.URL.Query().Get("callsign"))
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.buildSnapshot(r.Context(), match, session))
}

func (a *WebAPI) buildSnapshot(ctx context.Context, match pdc.WebStripMatch, session *models.Session) snapshot {
	s := match.Strip
	departure := strings.EqualFold(s.Origin, session.Airport)
	phase := "ARRIVAL"
	if departure {
		phase = "DEPARTURE"
	}
	state := s.PdcState
	if state == "REQUESTED_WITH_FAULTS" {
		state = "REQUESTED"
	}
	result := snapshot{Callsign: s.Callsign, AircraftType: s.AircraftType, Origin: s.Origin, Destination: s.Destination, Route: s.Route, Phase: phase, Runway: s.Runway, SID: s.Sid, STAR: s.Star, ClearedAltitude: s.ClearedAltitude, Squawk: s.AssignedSquawk, Stand: nonEmptyString(s.Stand), EOBT: s.EffectiveEobt(), TOBT: s.EffectiveTobt(), TSAT: normalizeClock(s.EffectiveTsat()), TTOT: s.EffectiveTtot(), CTOT: s.EffectiveCtot(), PDCState: state, PDCAvailable: a.pdcReady && departure && !s.Cleared, PDCCanSubmit: a.pdcReady && departure && !s.Cleared && pdc.WebPDCCanSubmit(s.PdcState), PDCRequiresPilotAction: state == "CLEARED", Capabilities: capabilities{PDC: a.pdcReady && departure, TOBT: a.cdmReady && departure, Stand: a.stands != nil && a.assignments != nil}}
	if departure && a.departures != nil {
		if frequency, err := a.departures.ComputeDepartureFrequencyForStripContext(ctx, s, match.SessionID); err == nil {
			result.DepartureFrequency = nonEmptyString(frequency)
		}
	}
	if s.CdmData != nil {
		result.CDMStatus = s.CdmData.EffectiveStatus()
	}
	if s.PdcData != nil && s.PdcData.Web != nil {
		result.PDCClearanceText = s.PdcData.Web.ClearanceText
	}
	if a.assignments != nil {
		if assignment, err := a.assignments.GetAssignment(ctx, match.SessionID, s.Callsign); err == nil && assignment != nil {
			if result.Stand == nil {
				result.Stand = nonEmptyString(&assignment.Stand)
			}
			result.StandVersion = &assignment.Version
		}
	}
	if a.atis != nil {
		airport := s.Destination
		if departure {
			airport = s.Origin
		}
		result.ATIS = a.atis.GetATIS(airport, departure)
	}
	return result
}

func (a *WebAPI) handleTobt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		TOBT     string `json:"tobt"`
		Callsign string `json:"callsign"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || !validHHMM(body.TOBT) {
		writeError(w, http.StatusBadRequest, "tobt must be HHMM")
		return
	}
	match, session, err := a.resolveFlight(r.Context(), user, body.Callsign)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if !strings.EqualFold(match.Strip.Origin, session.Airport) {
		writeError(w, http.StatusConflict, "TOBT is only available for departures")
		return
	}
	if a.cdm == nil || !a.cdmReady {
		writeError(w, http.StatusServiceUnavailable, "CDM unavailable")
		return
	}
	if err := a.cdm.HandleTobtUpdate(r.Context(), match.SessionID, match.Strip.Callsign, body.TOBT, "PILOT:"+user.GetCid(), "pilot"); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"callsign": match.Strip.Callsign, "tobt": body.TOBT})
}

func (a *WebAPI) handleStand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	var body struct {
		Stand    string `json:"stand"`
		Version  int32  `json:"version"`
		Callsign string `json:"callsign"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Stand) == "" {
		writeError(w, http.StatusBadRequest, "stand is required")
		return
	}
	match, session, err := a.resolveFlight(r.Context(), user, body.Callsign)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if a.stands == nil {
		writeError(w, http.StatusServiceUnavailable, "stand assignment unavailable")
		return
	}
	result, err := a.stands.AssignForPilot(r.Context(), match.SessionID, session.Airport, user.GetCid(), match.Strip.Callsign, body.Stand, body.Version)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, services.ErrStandActionUnauthorized) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callsign": result.Assignment.Callsign, "stand": result.Assignment.Stand, "version": result.Assignment.Version})
}

var errNoOnline = errors.New("no current VATSIM flight")

func (a *WebAPI) lookupCallsign(ctx context.Context, user shared.AuthenticatedUser, requested string) (string, bool, error) {
	if !a.live {
		value := strings.ToUpper(strings.TrimSpace(requested))
		return value, value != "", nil
	}
	if a.callsigns == nil {
		return "", false, errors.New("callsign lookup unavailable")
	}
	return a.callsigns.GetCallsignByCID(ctx, user.GetCid())
}

func (a *WebAPI) resolveFlight(ctx context.Context, user shared.AuthenticatedUser, requested string) (pdc.WebStripMatch, *models.Session, error) {
	callsign, found, err := a.lookupCallsign(ctx, user, requested)
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	if !found {
		return pdc.WebStripMatch{}, nil, errNoOnline
	}
	if a.flights == nil {
		return pdc.WebStripMatch{}, nil, errors.New("flight lookup unavailable")
	}
	match, err := a.flights.FindWebStripByCallsign(ctx, callsign)
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	session, err := a.sessions.GetByID(ctx, match.SessionID)
	return match, session, err
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) (shared.AuthenticatedUser, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	if header == "" || token == header || token == "" {
		writeError(w, http.StatusUnauthorized, "invalid authorization header")
		return shared.AuthenticatedUser{}, false
	}
	user, err := a.auth.Validate(token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return shared.AuthenticatedUser{}, false
	}
	return user, true
}

func (a *WebAPI) writeResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoOnline), errors.Is(err, pdc.ErrWebStripNotFound):
		writeError(w, http.StatusNotFound, "no flight found for authenticated pilot")
	case errors.Is(err, pdc.ErrWebAmbiguousCallsign):
		writeError(w, http.StatusConflict, "callsign matched multiple sessions")
	default:
		writeError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
	}
}
func validHHMM(v string) bool {
	v = strings.TrimSpace(v)
	if len(v) != 4 {
		return false
	}
	t, err := time.Parse("1504", v)
	return err == nil && t.Format("1504") == v
}

func normalizeClock(value *string) *string {
	value = nonEmptyString(value)
	if value == nil {
		return nil
	}
	if len(*value) == 6 && allDigits(*value) {
		normalized := (*value)[:4]
		return &normalized
	}
	return value
}

func nonEmptyString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
}

func allDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func optional(ok bool, value string) any {
	if !ok {
		return nil
	}
	return value
}
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
