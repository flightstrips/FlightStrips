package efb

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (a *WebAPI) networkFlight(ctx context.Context, user shared.AuthenticatedUser) (vatsim.Flight, error) {
	if a.network == nil {
		return vatsim.Flight{}, errors.New("VATSIM flight source is not configured")
	}
	state := a.network.Snapshot()
	if _, found := state.FlightByCID(user.GetCid()); !found {
		if _, _, err := a.network.GetCallsignByCID(ctx, user.GetCid()); err != nil {
			return vatsim.Flight{}, fmt.Errorf("VATSIM pilot lookup: %w", err)
		}
		state = a.network.Snapshot()
	}
	if state.LastRefreshError != nil {
		return vatsim.Flight{}, fmt.Errorf("VATSIM feed refresh: %w", state.LastRefreshError)
	}
	if state.Timestamp.IsZero() || time.Since(state.Timestamp) > 2*time.Minute {
		return vatsim.Flight{}, errors.New("VATSIM feed is stale")
	}
	flight, exists := state.FlightByCID(user.GetCid())
	if !exists || flight.CID != user.GetCid() || (!flight.Online() && !flight.Prefile()) ||
		flight.FlightPlan.Origin == "" || flight.FlightPlan.Destination == "" {
		return vatsim.Flight{}, errNoOnline
	}
	return flight, nil
}

func (a *WebAPI) handleNetworkFlight(w http.ResponseWriter, r *http.Request, user shared.AuthenticatedUser) {
	var flight vatsim.Flight
	var err error
	if a.callsignLookup {
		flight, err = a.networkFlightByCallsign(r.URL.Query().Get("callsign"))
	} else {
		flight, err = a.networkFlight(r.Context(), user)
	}
	if err != nil {
		slog.WarnContext(r.Context(), "EFB VATSIM flight lookup failed", "error", err)
		a.writeResolveError(w, err)
		return
	}
	airport := a.airport()
	departure := strings.EqualFold(flight.FlightPlan.Origin, airport)
	if !a.validateNetworkAirport(w, flight) {
		return
	}
	phase := "ARRIVAL"
	if departure {
		phase = "DEPARTURE"
	}
	result := snapshot{
		ReadOnly: a.callsignLookup, FlightState: flight.State,
		Callsign: flight.Callsign, Origin: flight.FlightPlan.Origin,
		Destination: flight.FlightPlan.Destination, Phase: phase,
		AircraftType: nonEmptyString(&flight.FlightPlan.Aircraft),
		Route:        nonEmptyString(&flight.FlightPlan.Route), EOBT: nonEmptyString(&flight.FlightPlan.EOBT),
		Squawk: nonEmptyString(&flight.FlightPlan.AssignedSquawk),
	}
	if result.AircraftType == nil {
		result.AircraftType = nonEmptyString(&flight.FlightPlan.AircraftShort)
	}
	if flight.Online() {
		result.Groundspeed = &flight.Groundspeed
	}
	if a.atis != nil {
		atisAirport := result.Destination
		if departure {
			atisAirport = result.Origin
		}
		result.ATIS = a.atis.GetATIS(atisAirport, departure)
	}
	timing, err := a.networkTiming(r.Context(), flight.Callsign)
	if err != nil {
		slog.WarnContext(r.Context(), "EFB vIFF timing lookup failed", "callsign", flight.Callsign, "error", err)
		result.DataErrors = append(result.DataErrors, "vIFF timing is unavailable; remote TOBT updates are disabled.")
	} else {
		result.TOBT = viffClock(timing.CDMData.TOBT, timing.TOBT)
		result.TSAT = viffClock(timing.CDMData.TSAT)
		result.CTOT = viffClock(timing.CTOT, timing.CDMData.CTOT)
		result.Capabilities.TOBT = !strings.EqualFold(result.Origin, "EKCH")
	}
	if departure {
		result.SID = filedSIDIndicator(flight.FlightPlan.Route, flight.FlightPlan.Origin)
		if result.ATIS != nil && !result.ATIS.Stale {
			result.Runway = departureRunwayFromATIS(strings.Join(result.ATIS.Text, " "))
		}
	}
	a.addNetworkStripData(r.Context(), &result)
	if !departure {
		a.addArrivalGuidance(r.Context(), &result)
	}
	if departure && !a.callsignLookup {
		a.addPilotDepartureStand(r.Context(), &result, flight)
	}
	applyDepartureRules(&result)
	if a.callsignLookup {
		result.Capabilities = capabilities{}
		result.PDCAvailable, result.PDCCanSubmit, result.PDCRequiresPilotAction = false, false, false
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *WebAPI) networkFlightByCallsign(callsign string) (vatsim.Flight, error) {
	if a.network == nil {
		return vatsim.Flight{}, errors.New("VATSIM flight source is not configured")
	}
	state := a.network.Snapshot()
	if state.LastRefreshError != nil {
		return vatsim.Flight{}, fmt.Errorf("VATSIM feed refresh: %w", state.LastRefreshError)
	}
	if state.Timestamp.IsZero() || time.Since(state.Timestamp) > 2*time.Minute {
		return vatsim.Flight{}, errors.New("VATSIM feed is stale")
	}
	flight, found := state.FlightByCallsign(strings.ToUpper(strings.TrimSpace(callsign)))
	if !found || flight.FlightPlan.Origin == "" || flight.FlightPlan.Destination == "" {
		return vatsim.Flight{}, errNoOnline
	}
	return flight, nil
}

func (a *WebAPI) networkTiming(ctx context.Context, callsign string) (*cdm.IFPSData, error) {
	if a.viff == nil {
		return nil, errors.New("vIFF is not configured")
	}
	timing, err := a.viff.FlightByCallsign(ctx, callsign)
	if err != nil {
		return nil, err
	}
	if timing == nil || !strings.EqualFold(timing.Callsign, callsign) {
		return nil, errors.New("vIFF returned a different flight")
	}
	return timing, nil
}

func (a *WebAPI) addNetworkStripData(ctx context.Context, result *snapshot) {
	if a.flights == nil {
		return
	}
	match, err := a.flights.FindWebStripByCallsign(ctx, result.Callsign)
	if errors.Is(err, pdc.ErrWebStripNotFound) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "EFB optional stand lookup failed", "callsign", result.Callsign, "error", err)
		result.DataErrors = append(result.DataErrors, "FlightStrips stand information is unavailable.")
		return
	}
	if match.Strip == nil || !strings.EqualFold(match.Strip.Origin, result.Origin) || !strings.EqualFold(match.Strip.Destination, result.Destination) {
		slog.WarnContext(ctx, "EFB optional stand belongs to a different flight plan", "callsign", result.Callsign)
		result.DataErrors = append(result.DataErrors, "FlightStrips stand information does not match the current VATSIM flight.")
		return
	}
	if result.Phase == "DEPARTURE" {
		if strings.EqualFold(result.Origin, "EKCH") && a.cdm != nil {
			if _, err := a.networkCDMSession(ctx, match, result.Origin, result.Destination); err != nil {
				slog.WarnContext(ctx, "EFB authoritative CDM lookup failed", "callsign", result.Callsign, "error", err)
				result.DataErrors = append(result.DataErrors, "Local CDM timing is unavailable; TOBT updates are disabled.")
			} else {
				result.TOBT = normalizeClock(match.Strip.EffectiveTobt())
				result.TSAT = normalizeClock(match.Strip.EffectiveTsat())
				result.TTOT = normalizeClock(match.Strip.EffectiveTtot())
				result.Capabilities.TOBT = a.cdmReady
			}
		}
		state := match.Strip.PdcState
		if state == "REQUESTED_WITH_FAULTS" {
			state = "REQUESTED"
		}
		result.PDCState = state
		result.PDCAvailable = a.pdcReady && !match.Strip.Cleared
		result.PDCCanSubmit = result.PDCAvailable && pdc.WebPDCCanSubmit(match.Strip.PdcState)
		result.PDCRequiresPilotAction = state == "CLEARED"
		result.Capabilities.PDC = a.pdcReady
		if match.Strip.PdcData != nil && match.Strip.PdcData.Web != nil {
			result.PDCClearanceText = match.Strip.PdcData.Web.ClearanceText
		}
		if sid := nonEmptyString(match.Strip.Sid); sid != nil {
			result.SID = sid
		}
		if runway := nonEmptyString(match.Strip.Runway); runway != nil {
			result.Runway = runway
		}
		result.ClearedAltitude = match.Strip.ClearedAltitude
		if a.departures != nil && !strings.EqualFold(result.Origin, "EKCH") {
			frequency, err := a.departures.ComputeDepartureFrequencyForStripContext(ctx, match.Strip, match.SessionID)
			if err != nil {
				slog.WarnContext(ctx, "EFB departure frequency lookup failed", "callsign", result.Callsign, "error", err)
				result.DataErrors = append(result.DataErrors, "Departure handover frequency is unavailable.")
			} else {
				result.DepartureFrequency = nonEmptyString(frequency)
			}
		}
		result.Stand = nonEmptyString(match.Strip.Stand)
		if result.Stand != nil {
			state := "SENSED"
			result.StandState = &state
		}
		return
	}
	result.STAR = nonEmptyString(match.Strip.Star)
	result.Runway = nonEmptyString(match.Strip.Runway)
	result.ArrivalETA = arrivalETA(match.Strip.ArrivalETA)
	a.addNetworkStandAssignment(ctx, result, match)
}

func (a *WebAPI) addNetworkStandAssignment(ctx context.Context, result *snapshot, match pdc.WebStripMatch) {
	if a.assignments == nil {
		return
	}
	assignment, err := a.assignments.GetAssignment(ctx, match.SessionID, result.Callsign)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		slog.WarnContext(ctx, "EFB optional stand assignment lookup failed", "callsign", result.Callsign, "error", err)
		result.DataErrors = append(result.DataErrors, "FlightStrips stand information is unavailable.")
		return
	}
	result.Capabilities.Stand = a.stands != nil
	if assignment == nil {
		return
	}
	result.StandVersion = &assignment.Version
	if assignment.ExpiresAt != nil && !assignment.ExpiresAt.After(time.Now()) {
		return
	}
	switch assignment.Stage {
	case services.StageEstimated, services.StageAssigned, services.StageConfirmed:
		result.Stand = nonEmptyString(&assignment.Stand)
		result.StandState = &assignment.Stage
	}
}

var departureRunwayPattern = regexp.MustCompile(`(?i)\b(?:DEPARTURE\s+|DEP\s+|TAKE[- ]?OFF\s+)?(?:RWYS?|RUNWAYS?)\s+(?:IN\s+USE\s+)?((?:0[1-9]|[12][0-9]|3[0-6])[LRC]?)\b`)
var filedSpeedLevelPattern = regexp.MustCompile(`^(?:N\d{4}|K\d{4}|M\d{3})(?:F\d{3}|A\d{3}|S\d{4}|M\d{4})$`)
var filedWaypointPattern = regexp.MustCompile(`^[A-Z0-9]{2,11}$`)
var departureSIDPattern = regexp.MustCompile(`^([A-Z]{5})(?:[0-9]+[A-Z])?$`)

func applyDepartureRules(result *snapshot) {
	if result.Phase != "DEPARTURE" || !strings.EqualFold(result.Origin, "EKCH") {
		return
	}
	if result.Runway != nil {
		var climb string
		switch strings.ToUpper(strings.TrimSpace(*result.Runway)) {
		case "22R", "22L", "04R", "04L":
			climb = "FL70"
		case "12", "30":
			climb = "4000ft"
		}
		result.InitialClimb = nonEmptyString(&climb)
	}
	if result.SID != nil {
		sid := departureSIDPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(*result.SID)))
		if sid == nil {
			return
		}
		var frequency string
		switch sid[1] {
		case "NEXEN", "LANGO", "KOPEX", "SALLO", "SIMEG", "KEMAX":
			frequency = "124.980"
		case "ODDON", "GOLGA", "VEDAR":
			frequency = "120.255"
		}
		result.DepartureFrequency = nonEmptyString(&frequency)
	}
}

func departureRunwayFromATIS(text string) *string {
	// A condition report alone is not a runway-in-use declaration.
	for _, match := range departureRunwayPattern.FindAllStringSubmatch(strings.ToUpper(text), -1) {
		declaration := match[0]
		if strings.Contains(declaration, "IN USE") || strings.HasPrefix(declaration, "DEP") || strings.HasPrefix(declaration, "TAKE") {
			return &match[1]
		}
	}
	return nil
}

func filedSIDIndicator(route, origin string) *string {
	for _, token := range strings.Fields(strings.ToUpper(route)) {
		token = strings.SplitN(token, "/", 2)[0]
		if token == "DCT" || token == strings.ToUpper(origin) || filedSpeedLevelPattern.MatchString(token) {
			continue
		}
		if filedWaypointPattern.MatchString(token) {
			return &token
		}
		return nil
	}
	return nil
}

func (a *WebAPI) handleNetworkTobt(w http.ResponseWriter, r *http.Request, user shared.AuthenticatedUser, tobt string) {
	flight, err := a.networkFlight(r.Context(), user)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	if !a.validateNetworkAirport(w, flight) {
		return
	}
	if strings.EqualFold(flight.FlightPlan.Origin, "EKCH") {
		if a.cdm == nil || !a.cdmReady || a.flights == nil {
			writeError(w, http.StatusServiceUnavailable, "authoritative CDM TOBT updates unavailable")
			return
		}
		match, err := a.flights.FindWebStripByCallsign(r.Context(), flight.Callsign)
		if err == nil {
			_, err = a.networkCDMSession(r.Context(), match, flight.FlightPlan.Origin, flight.FlightPlan.Destination)
		}
		if err != nil {
			slog.WarnContext(r.Context(), "EFB authoritative CDM flight lookup failed", "callsign", flight.Callsign, "error", err)
			writeError(w, http.StatusServiceUnavailable, "a matching LIVE departure is required for authoritative CDM TOBT updates")
			return
		}
		if err := a.cdm.HandleTobtUpdate(r.Context(), match.SessionID, flight.Callsign, tobt, "EFB:"+user.GetCid(), "PILOT"); err != nil {
			slog.WarnContext(r.Context(), "EFB authoritative CDM TOBT update failed", "callsign", flight.Callsign, "error", err)
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"callsign": flight.Callsign, "tobt": tobt})
		return
	}
	timing, err := a.networkTiming(r.Context(), flight.Callsign)
	if err == nil {
		err = a.viff.IFPSSetTobt(r.Context(), flight.Callsign, tobt, timing.Taxi)
	}
	if err != nil {
		slog.WarnContext(r.Context(), "EFB vIFF TOBT update failed", "callsign", flight.Callsign, "error", err)
		writeError(w, http.StatusBadGateway, "vIFF did not confirm the TOBT update")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"callsign": flight.Callsign, "tobt": tobt})
}

func (a *WebAPI) validateNetworkAirport(w http.ResponseWriter, flight vatsim.Flight) bool {
	airport := a.airport()
	if !strings.EqualFold(flight.FlightPlan.Origin, airport) && !strings.EqualFold(flight.FlightPlan.Destination, airport) {
		writeError(w, http.StatusUnprocessableEntity, "EFB is available for flights departing from or arriving at "+airport)
		return false
	}
	return true
}

func (a *WebAPI) networkCDMSession(ctx context.Context, match pdc.WebStripMatch, origin, destination string) (*models.Session, error) {
	if match.Strip == nil || !strings.EqualFold(match.Strip.Origin, origin) || !strings.EqualFold(match.Strip.Destination, destination) || a.sessions == nil {
		return nil, errors.New("matching CDM departure is unavailable")
	}
	session, err := a.sessions.GetByID(ctx, match.SessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || !strings.EqualFold(session.Name, "LIVE") || !strings.EqualFold(session.Airport, origin) {
		return nil, errors.New("CDM departure requires a matching LIVE airport session")
	}
	return session, nil
}
