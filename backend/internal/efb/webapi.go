package efb

import (
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/metar"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

type ViffClient interface {
	FlightByCallsign(context.Context, string) (*cdm.IFPSData, error)
	IFPSSetTobt(context.Context, string, string, int) error
}

type StandActions interface {
	AssignForPilot(context.Context, int32, string, string, string, string, int32, bool) (*services.StandAllocationResult, error)
	AvailableForPilot(context.Context, int32, string, string) ([]services.StandAvailability, error)
}

type PilotStandActions interface {
	Open(context.Context, int32, *models.Strip, vatsim.DepartureFlightInfo) (*services.PilotDepartureStand, error)
	Read(context.Context, int32, vatsim.DepartureFlightInfo) (*services.PilotDepartureStand, error)
	Select(context.Context, int32, *models.Strip, vatsim.DepartureFlightInfo, string, int32, bool) (*services.PilotDepartureStand, error)
}

type ATISLookup interface {
	GetATIS(airport string, departure bool) *metar.ATIS
}

type DepartureFrequencyLookup interface {
	ComputeDepartureFrequencyForStripContext(context.Context, *models.Strip, int32) (*string, error)
}

type WebAPIConfig struct {
	Auth           shared.AuthenticationService
	Callsigns      CallsignLookup
	Flights        FlightFinder
	Sessions       repository.SessionRepository
	Assignments    repository.StandAssignmentRepository
	CDM            CDMUpdater
	CDMReady       bool
	Viff           ViffClient
	Stands         StandActions
	ATIS           ATISLookup
	Departures     DepartureFrequencyLookup
	PDCReady       bool
	Live           bool
	Terminal       terminal.Configuration
	Navigation     navdata.GeometrySnapshotReader
	Network        vatsim.FlightSource
	CallsignLookup bool
	Training       bool
	TrainingStrips repository.StripRepository
	PilotStands    PilotStandActions
}

type WebAPI struct {
	auth           shared.AuthenticationService
	callsigns      CallsignLookup
	flights        FlightFinder
	sessions       repository.SessionRepository
	assignments    repository.StandAssignmentRepository
	cdm            CDMUpdater
	cdmReady       bool
	viff           ViffClient
	stands         StandActions
	atis           ATISLookup
	departures     DepartureFrequencyLookup
	pdcReady       bool
	live           bool
	terminal       terminal.Configuration
	navigation     navdata.GeometrySnapshotReader
	network        vatsim.FlightSource
	callsignLookup bool
	training       bool
	trainingStrips repository.StripRepository
	pilotStands    PilotStandActions
}

func NewWebAPI(cfg WebAPIConfig) *WebAPI {
	return &WebAPI{auth: cfg.Auth, callsigns: cfg.Callsigns, flights: cfg.Flights, sessions: cfg.Sessions, assignments: cfg.Assignments, cdm: cfg.CDM, cdmReady: cfg.CDMReady, viff: cfg.Viff, stands: cfg.Stands, atis: cfg.ATIS, departures: cfg.Departures, pdcReady: cfg.PDCReady, live: cfg.Live, terminal: cfg.Terminal, navigation: cfg.Navigation, network: cfg.Network, callsignLookup: cfg.CallsignLookup && !cfg.Live, training: cfg.Training && !cfg.Live, trainingStrips: cfg.TrainingStrips, pilotStands: cfg.PilotStands}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/efb/me", a.handleMe)
	mux.HandleFunc("/efb/flight", a.handleFlight)
	mux.HandleFunc("/efb/tobt", a.handleTobt)
	mux.HandleFunc("/efb/stand", a.handleStand)
	mux.HandleFunc("/efb/stand/open", a.handleStandOpen)
	mux.HandleFunc("/efb/stands", a.handleStandAvailability)
	mux.HandleFunc("/efb/training-sessions", a.handleTrainingSessions)
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
	if a.callsignLookup {
		writeJSON(w, http.StatusOK, map[string]any{"cid": user.GetCid(), "live_mode": false, "callsign_lookup": true, "training_available": a.training, "online_callsign": nil, "callsign_locked": false})
		return
	}
	callsign, found, err := a.lookupCallsign(r.Context(), user, r.URL.Query().Get("callsign"))
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "pilot lookup unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cid": user.GetCid(), "live_mode": a.live || a.network != nil, "training_available": a.training, "online_callsign": optional(found, callsign), "callsign_locked": (a.live || a.network != nil) && found})
}

type snapshot struct {
	ReadOnly               bool               `json:"read_only"`
	FlightState            vatsim.FlightState `json:"flight_state,omitempty"`
	Groundspeed            *int               `json:"groundspeed"`
	DataErrors             []string           `json:"data_errors,omitempty"`
	Callsign               string             `json:"callsign"`
	AircraftType           *string            `json:"aircraft_type"`
	Origin                 string             `json:"origin"`
	Destination            string             `json:"destination"`
	Route                  *string            `json:"route"`
	Phase                  string             `json:"phase"`
	Runway                 *string            `json:"runway"`
	SID                    *string            `json:"sid"`
	STAR                   *string            `json:"star"`
	ArrivalETA             *string            `json:"arrival_eta"`
	TerminalFix            *string            `json:"terminal_fix"`
	PublishedHoldingFix    *string            `json:"published_holding_fix"`
	PublishedHoldingDetail *string            `json:"published_holding_detail"`
	PublishedHeading       *int32             `json:"published_heading"`
	ClearedAltitude        *int32             `json:"cleared_altitude"`
	InitialClimb           *string            `json:"initial_climb"`
	Squawk                 *string            `json:"squawk"`
	DepartureFrequency     *string            `json:"departure_frequency"`
	Stand                  *string            `json:"stand"`
	StandVersion           *int32             `json:"stand_version"`
	StandState             *string            `json:"stand_state"`
	StandExpiresAt         *time.Time         `json:"stand_expires_at,omitempty"`
	EOBT                   *string            `json:"eobt"`
	TOBT                   *string            `json:"tobt"`
	TSAT                   *string            `json:"tsat"`
	TTOT                   *string            `json:"ttot"`
	CTOT                   *string            `json:"ctot"`
	CDMStatus              *string            `json:"cdm_status"`
	PDCState               string             `json:"pdc_state"`
	PDCAvailable           bool               `json:"pdc_available"`
	PDCCanSubmit           bool               `json:"pdc_can_submit"`
	PDCRequiresPilotAction bool               `json:"pdc_requires_pilot_action"`
	PDCClearanceText       *string            `json:"pdc_clearance_text"`
	ATIS                   *metar.ATIS        `json:"atis"`
	Capabilities           capabilities       `json:"capabilities"`
}

type capabilities struct {
	PDC                 bool `json:"pdc"`
	TOBT                bool `json:"tobt_update"`
	Stand               bool `json:"stand_reassignment"`
	StandInitialization bool `json:"stand_initialization,omitempty"`
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
	if hasTrainingSession(r) {
		a.handleTrainingFlight(w, r)
		return
	}
	a.handleNetworkFlight(w, r, user)
}

func viffClock(values ...string) *string {
	for _, value := range values {
		if value = strings.TrimSuffix(strings.TrimSpace(value), "Z"); value != "" {
			return normalizeClock(&value)
		}
	}
	return nil
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
	publishedHeading, publishedHoldingFix, publishedHoldingDetail := publishedArrivalData(ctx, a.navigation, a.terminal, s.Destination, s.Runway, s.Star)
	result := snapshot{Callsign: s.Callsign, AircraftType: s.AircraftType, Origin: s.Origin, Destination: s.Destination, Route: s.Route, Phase: phase, Runway: s.Runway, SID: s.Sid, STAR: s.Star, ArrivalETA: arrivalETA(s.ArrivalETA), TerminalFix: terminalFix(a.terminal, s.Destination, s.Runway, s.Star), PublishedHeading: publishedHeading, PublishedHoldingFix: publishedHoldingFix, PublishedHoldingDetail: publishedHoldingDetail, ClearedAltitude: s.ClearedAltitude, Squawk: s.AssignedSquawk, Stand: nonEmptyString(s.Stand), EOBT: s.EffectiveEobt(), TOBT: s.EffectiveTobt(), TSAT: normalizeClock(s.EffectiveTsat()), TTOT: s.EffectiveTtot(), CTOT: s.EffectiveCtot(), PDCState: state, PDCAvailable: a.pdcReady && departure && !s.Cleared, PDCCanSubmit: a.pdcReady && departure && !s.Cleared && pdc.WebPDCCanSubmit(s.PdcState), PDCRequiresPilotAction: state == "CLEARED", Capabilities: capabilities{PDC: a.pdcReady && departure, TOBT: a.cdmReady && departure, Stand: a.stands != nil && a.assignments != nil}}
	if !departure {
		result.Stand = nil
	} else if result.Stand != nil {
		sensed := "SENSED"
		result.StandState = &sensed
	}
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
	if a.atis != nil {
		airport := s.Destination
		if departure {
			airport = s.Origin
		}
		result.ATIS = a.atis.GetATIS(airport, departure)
	}
	return result
}

func arrivalETA(value *models.ArrivalETA) *string {
	if value == nil || value.Time.IsZero() {
		return nil
	}
	formatted := value.Time.UTC().Format("1504")
	return &formatted
}

func terminalFix(config terminal.Configuration, airport string, runway, star *string) *string {
	path := terminalPath(config, airport, runway, star)
	if path == nil || len(path.Fixes) == 0 {
		return nil
	}
	value := string(path.Fixes[len(path.Fixes)-1])
	return &value
}

func publishedArrivalData(ctx context.Context, navigation navdata.GeometrySnapshotReader, config terminal.Configuration, airport string, runway, star *string) (*int32, *string, *string) {
	configuredPath := terminalPath(config, airport, runway, star)
	if configuredPath == nil {
		return nil, nil, nil
	}
	var heading *int32
	var fix, detail *string
	if configuredPath.PublishedHeadingMagneticDeg != nil {
		value := int32(*configuredPath.PublishedHeadingMagneticDeg)
		heading = &value
	}
	for _, holding := range config.OverlayHoldings {
		if holding.ID == configuredPath.SelectedHolding {
			fixValue := string(holding.Fix)
			detailValue := fmt.Sprintf("%03.0f/%s", holding.InboundCourseTrueDeg, strings.ToUpper(string(holding.TurnDirection)))
			fix, detail = &fixValue, &detailValue
			break
		}
	}
	if navigation == nil {
		return heading, fix, detail
	}
	snapshot, err := navigation.ActiveGeometrySnapshot(ctx, navdata.AirportID(strings.ToUpper(strings.TrimSpace(airport))))
	if err != nil {
		slog.WarnContext(ctx, "EFB navigation snapshot unavailable; using configured published arrival data", "airport", airport, "error", err)
		return heading, fix, detail
	}
	for _, path := range snapshot.TerminalPaths {
		if path.Feeder != configuredPath.Feeder || path.RunwayGroup != configuredPath.RunwayGroup {
			continue
		}
		if path.PublishedHeadingMagneticDeg != nil {
			value := int32(*path.PublishedHeadingMagneticDeg)
			heading = &value
		}
		if len(path.HoldingIDs) == 0 {
			return heading, fix, detail
		}
		holdingID := path.HoldingIDs[0]
		for _, holding := range snapshot.Holdings {
			if holding.ID != holdingID {
				continue
			}
			fix := string(holding.Fix)
			detail := fmt.Sprintf("%03.0f/%s", holding.InboundCourseTrueDeg, strings.ToUpper(string(holding.TurnDirection)))
			return heading, &fix, &detail
		}
	}
	return heading, fix, detail
}

func terminalPath(config terminal.Configuration, airport string, runway, star *string) *terminal.Path {
	if star == nil || runway == nil || !strings.EqualFold(strings.TrimSpace(airport), string(config.Airport)) {
		return nil
	}
	procedure := strings.ToUpper(strings.TrimSpace(*star))
	if procedure == "" {
		return nil
	}
	var feeder string
	for _, candidate := range config.Feeders {
		if candidate.ID != "" && strings.HasPrefix(procedure, string(candidate.ID)) {
			feeder = string(candidate.ID)
			break
		}
		for _, alias := range candidate.Aliases {
			if alias != "" && strings.HasPrefix(procedure, string(alias)) {
				feeder = string(candidate.ID)
				break
			}
		}
		if feeder != "" {
			break
		}
	}
	if feeder == "" {
		return nil
	}
	assignedRunway := strings.ToUpper(strings.TrimSpace(*runway))
	for _, group := range config.RunwayGroups {
		if !containsRunway(group, assignedRunway) {
			continue
		}
		for index := range config.Paths {
			path := &config.Paths[index]
			if string(path.Feeder) != feeder || path.RunwayGroup != group.ID {
				continue
			}
			return path
		}
	}
	return nil
}

func containsRunway(group terminal.RunwayGroup, runway string) bool {
	for _, candidate := range group.Runways {
		if string(candidate) == runway {
			return true
		}
	}
	return false
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
	if a.callsignLookup && !hasTrainingSession(r) {
		writeError(w, http.StatusForbidden, "callsign lookup is read-only")
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
	if hasTrainingSession(r) {
		a.handleTrainingTobt(w, r, user, body.Callsign, body.TOBT)
		return
	}
	a.handleNetworkTobt(w, r, user, body.TOBT)
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
	if a.callsignLookup && !hasTrainingSession(r) {
		writeError(w, http.StatusForbidden, "callsign lookup is read-only")
		return
	}
	var body struct {
		Stand           string `json:"stand"`
		Version         int32  `json:"version"`
		Callsign        string `json:"callsign"`
		AllowUnsuitable bool   `json:"allow_unsuitable"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Stand) == "" {
		writeError(w, http.StatusBadRequest, "stand is required")
		return
	}
	if !hasTrainingSession(r) && a.network != nil {
		flight, err := a.networkFlight(r.Context(), user)
		if err != nil {
			a.writeResolveError(w, err)
			return
		}
		if strings.EqualFold(flight.FlightPlan.Origin, a.airport()) {
			a.handlePilotDepartureSelection(w, r, flight, body.Stand, body.Version, body.AllowUnsuitable)
			return
		}
	}
	match, session, err := a.resolveRequestedFlight(r, user, body.Callsign)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if a.stands == nil {
		writeError(w, http.StatusServiceUnavailable, "stand assignment unavailable")
		return
	}
	result, err := a.stands.AssignForPilot(r.Context(), match.SessionID, session.Airport, user.GetCid(), match.Strip.Callsign, body.Stand, body.Version, body.AllowUnsuitable)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, services.ErrStandActionUnauthorized) {
			status = http.StatusForbidden
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callsign": result.Assignment.Callsign, "stand": result.Assignment.Stand, "stand_state": result.Assignment.Stage, "version": result.Assignment.Version})
}

func (a *WebAPI) handleStandAvailability(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	match, session, err := a.resolveRequestedFlight(r, user, r.URL.Query().Get("callsign"))
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if a.stands == nil {
		writeError(w, http.StatusServiceUnavailable, "stand availability unavailable")
		return
	}
	stands, err := a.stands.AvailableForPilot(r.Context(), match.SessionID, session.Airport, match.Strip.Callsign)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "stand availability unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stands": stands})
}

var errNoOnline = errors.New("no current VATSIM flight")

func (a *WebAPI) lookupCallsign(ctx context.Context, user shared.AuthenticatedUser, requested string) (string, bool, error) {
	if a.network != nil {
		flight, err := a.networkFlight(ctx, user)
		if errors.Is(err, errNoOnline) {
			return "", false, nil
		}
		return flight.Callsign, err == nil, err
	}
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
	case errors.Is(err, errTrainingForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, errTrainingSelection):
		writeError(w, http.StatusBadRequest, err.Error())
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
