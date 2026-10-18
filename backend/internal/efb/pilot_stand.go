package efb

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	"FlightStrips/internal/vatsim"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

const standAssignmentUnavailable = "STAND ASSIGNMENT UNAVAILABLE"

func (a *WebAPI) airport() string {
	if airport := string(a.terminal.Airport); airport != "" {
		return airport
	}
	return "EKCH"
}

func pilotDepartureFlight(flight vatsim.Flight) vatsim.DepartureFlightInfo {
	aircraft := flight.FlightPlan.AircraftShort
	if aircraft == "" {
		aircraft = strings.Split(flight.FlightPlan.Aircraft, "/")[0]
	}
	return vatsim.DepartureFlightInfo{
		Callsign: flight.Callsign, CID: flight.CID, Online: flight.Online(),
		Revision: flight.FlightPlan.Revision, Origin: flight.FlightPlan.Origin,
		Destination: flight.FlightPlan.Destination, AircraftType: aircraft,
		Latitude: flight.Latitude, Longitude: flight.Longitude,
	}
}

func (a *WebAPI) pilotStandFlight(ctx context.Context, flight vatsim.Flight) (pdc.WebStripMatch, *models.Session, error) {
	if !strings.EqualFold(flight.FlightPlan.Origin, a.airport()) {
		return pdc.WebStripMatch{}, nil, errors.New("pilot departure stand initialization is departure-only")
	}
	if a.pilotStands == nil || a.flights == nil || a.sessions == nil {
		return pdc.WebStripMatch{}, nil, errors.New("stand allocator is unavailable")
	}
	match, err := a.flights.FindWebStripByCallsign(ctx, flight.Callsign)
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	session, err := a.sessions.GetByID(ctx, match.SessionID)
	if err != nil {
		return pdc.WebStripMatch{}, nil, err
	}
	if session == nil || !strings.EqualFold(session.Name, "LIVE") || !strings.EqualFold(session.Airport, flight.FlightPlan.Origin) ||
		match.Strip == nil || !strings.EqualFold(match.Strip.Origin, flight.FlightPlan.Origin) || !strings.EqualFold(match.Strip.Destination, flight.FlightPlan.Destination) {
		return pdc.WebStripMatch{}, nil, errors.New("stand assignment requires a matching LIVE departure strip")
	}
	return match, session, nil
}

func (a *WebAPI) addPilotDepartureStand(ctx context.Context, result *snapshot, flight vatsim.Flight) {
	result.Stand, result.StandVersion, result.StandExpiresAt = nil, nil, nil
	result.StandState = stringRef(standAssignmentUnavailable)
	result.Capabilities.Stand = false
	match, _, err := a.pilotStandFlight(ctx, flight)
	if err != nil {
		slog.WarnContext(ctx, "EFB departure stand unavailable", "callsign", flight.Callsign, "error", err)
		return
	}
	state, err := a.pilotStands.Read(ctx, match.SessionID, pilotDepartureFlight(flight))
	if err != nil {
		slog.WarnContext(ctx, "EFB departure stand lookup failed", "callsign", flight.Callsign, "error", err)
		result.DataErrors = append(result.DataErrors, "Stand assignment is unavailable.")
		return
	}
	if state == nil {
		result.StandState = nil
		result.Capabilities.StandInitialization = true
		return
	}
	result.Stand, result.StandState, result.StandVersion, result.StandExpiresAt = nonEmptyString(&state.Stand), &state.State, &state.Version, state.ExpiresAt
	result.Capabilities.Stand = flight.Prefile()
}

func stringRef(value string) *string { return &value }

func (a *WebAPI) handleStandOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.authenticate(w, r)
	if !ok {
		return
	}
	if a.callsignLookup || hasTrainingSession(r) {
		writeError(w, http.StatusForbidden, "pilot departure stand initialization is unavailable for this source")
		return
	}
	flight, err := a.networkFlight(r.Context(), user)
	if err != nil {
		a.writeResolveError(w, err)
		return
	}
	match, _, err := a.pilotStandFlight(r.Context(), flight)
	if err == nil {
		_, err = a.pilotStands.Open(r.Context(), match.SessionID, match.Strip, pilotDepartureFlight(flight))
	}
	if err != nil {
		slog.WarnContext(r.Context(), "EFB departure stand initialization failed", "callsign", flight.Callsign, "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *WebAPI) handlePilotDepartureSelection(w http.ResponseWriter, r *http.Request, flight vatsim.Flight, stand string, version int32, allowUnsuitable bool) {
	if flight.Online() {
		writeError(w, http.StatusForbidden, "online departures follow their detected stand; manual selection is prefile-only")
		return
	}
	match, _, err := a.pilotStandFlight(r.Context(), flight)
	var result *services.PilotDepartureStand
	if err == nil {
		result, err = a.pilotStands.Select(r.Context(), match.SessionID, match.Strip, pilotDepartureFlight(flight), stand, version, allowUnsuitable)
	}
	if err != nil {
		slog.WarnContext(r.Context(), "EFB departure stand selection failed", "callsign", flight.Callsign, "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"callsign": flight.Callsign, "stand": result.Stand, "stand_state": result.State, "version": result.Version})
}
