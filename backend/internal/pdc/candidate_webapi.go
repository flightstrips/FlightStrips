package pdc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/httpresults"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
)

// CandidateFlightReader reads the NATS projection, never a legacy SQL row.
type CandidateFlightReader interface {
	FindFlight(context.Context, string) (*cluster.FlightSnapshot, error)
}

// CandidateWebAPI is installed only by the coordinated NATS runtime.
type CandidateWebAPI struct {
	auth     *WebAPI
	flights  CandidateFlightReader
	commands httpresults.Commander
	outcomes httpresults.OutcomeReader
}

func NewCandidateWebAPI(auth shared.AuthenticationService, verifier CallsignVerifier, requireLiveCIDVerification bool, flights CandidateFlightReader, commands httpresults.Commander, outcomes httpresults.OutcomeReader) *CandidateWebAPI {
	return &CandidateWebAPI{auth: NewWebAPI(auth, nil, verifier, requireLiveCIDVerification), flights: flights, commands: commands, outcomes: outcomes}
}

func (a *CandidateWebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/pdc/request", a.request)
	mux.HandleFunc("/pdc/status", a.status)
	mux.HandleFunc("/pdc/acknowledge", a.acknowledge)
	mux.HandleFunc("/pdc/unable", a.unable)
}

func (a *CandidateWebAPI) request(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.auth.authenticate(w, r)
	if !ok {
		return
	}
	var body requestWebPDCBody
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !a.auth.authorizeCallsign(w, r.Context(), user, body.Callsign) {
		return
	}
	atis := strings.ToUpper(strings.TrimSpace(body.Atis))
	aircraft := strings.ToUpper(strings.TrimSpace(body.AircraftType))
	if len(atis) != 1 || atis[0] < 'A' || atis[0] > 'Z' {
		writeJSONError(w, http.StatusBadRequest, "invalid ATIS letter")
		return
	}
	if aircraft == "" {
		writeJSONError(w, http.StatusBadRequest, "aircraft type is required")
		return
	}
	id, ok := httpresults.CommandID(r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid Idempotency-Key")
		return
	}
	key := strings.ToUpper(strings.TrimSpace(body.Callsign))
	ref, expected, _, ok := a.target(w, r, id, user.GetCid(), key)
	if !ok {
		return
	}
	action := &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: &pb.PdcAction{Callsign: key, Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestRemarks: strings.TrimSpace(body.Remarks), RequestChannel: "WEB", Atis: atis, Stand: strings.ToUpper(strings.TrimSpace(body.Stand)), AircraftType: aircraft}}}}}
	reply := httpresults.SubmitPilot(r.Context(), a.commands, a.outcomes, id, user.GetCid(), ref, expected, action)
	httpresults.WriteResult(w, id, reply, http.StatusCreated, map[string]string{"callsign": key}, candidateFailure)
}

func (a *CandidateWebAPI) status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.auth.authenticate(w, r)
	if !ok {
		return
	}
	key := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("callsign")))
	if !a.auth.authorizeCallsign(w, r.Context(), user, key) {
		return
	}
	flight, ok := a.flight(w, r, key)
	if !ok {
		return
	}
	if flight.PDC == nil {
		writeJSONError(w, http.StatusNotFound, "no web PDC request found for callsign")
		return
	}
	writeJSON(w, http.StatusOK, candidateStatus(flight.PDC))
}

func (a *CandidateWebAPI) acknowledge(w http.ResponseWriter, r *http.Request) {
	a.pilotResponse(w, r, false)
}
func (a *CandidateWebAPI) unable(w http.ResponseWriter, r *http.Request) { a.pilotResponse(w, r, true) }

func (a *CandidateWebAPI) pilotResponse(w http.ResponseWriter, r *http.Request, unable bool) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.auth.authenticate(w, r)
	if !ok {
		return
	}
	var body acknowledgeWebPDCBody
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !a.auth.authorizeCallsign(w, r.Context(), user, body.Callsign) {
		return
	}
	id, ok := httpresults.CommandID(r)
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid Idempotency-Key")
		return
	}
	key := strings.ToUpper(strings.TrimSpace(body.Callsign))
	ref, expected, flight, ok := a.target(w, r, id, user.GetCid(), key)
	if !ok {
		return
	}
	if flight.PDC == nil {
		writeJSONError(w, http.StatusNotFound, "no web PDC request found for callsign")
		return
	}
	change := &pb.PdcAction_Acknowledge{Acknowledge: &pb.AcknowledgePdc{}}
	var action *pb.PdcAction
	if unable {
		action = &pb.PdcAction{Callsign: key, Change: &pb.PdcAction_Unable{Unable: &pb.UnablePdc{}}}
	} else {
		action = &pb.PdcAction{Callsign: key, Change: change}
	}
	reply := httpresults.SubmitPilot(r.Context(), a.commands, a.outcomes, id, user.GetCid(), ref, expected, &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: action}})
	if reply != nil && reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		if latest, err := a.flights.FindFlight(r.Context(), key); err == nil {
			flight = latest
		}
	}
	httpresults.WriteResult(w, id, reply, http.StatusOK, candidateStatus(flight.PDC), candidateFailure)
}

func (a *CandidateWebAPI) target(w http.ResponseWriter, r *http.Request, id, cid, callsign string) (*pb.AggregateRef, *uint64, *cluster.FlightSnapshot, bool) {
	if a.outcomes == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, nil, false
	}
	prior := a.outcomes.Outcome(r.Context(), id, &pb.Actor{Kind: pb.Actor_PILOT, Id: cid})
	if prior == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, nil, false
	}
	if prior.Status == pb.CommandReply_UNAUTHORIZED {
		writeJSONError(w, http.StatusForbidden, "command belongs to another actor")
		return nil, nil, nil, false
	}
	if prior.Status != pb.CommandReply_NOT_FOUND && prior.Outcome == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, nil, false
	}
	flight, ok := a.flight(w, r, callsign)
	if !ok {
		return nil, nil, nil, false
	}
	if prior.Outcome != nil {
		if prior.Outcome.Aggregate.GetSession() == nil {
			writeJSONError(w, http.StatusBadRequest, "command ID belongs to another action")
			return nil, nil, nil, false
		}
		return prior.Outcome.Aggregate, prior.Outcome.ExpectedEntityRevision, flight, true
	}
	return httpresults.Session(flight.SessionID), &flight.PDCRevision, flight, true
}

func (a *CandidateWebAPI) flight(w http.ResponseWriter, r *http.Request, callsign string) (*cluster.FlightSnapshot, bool) {
	if a.flights == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
		return nil, false
	}
	flight, err := a.flights.FindFlight(r.Context(), callsign)
	if err != nil {
		switch {
		case errors.Is(err, cluster.ErrFlightNotFound):
			writeJSONError(w, http.StatusNotFound, "no strip found for callsign")
		case errors.Is(err, cluster.ErrAmbiguousFlight):
			writeJSONError(w, http.StatusConflict, "callsign matched multiple sessions")
		default:
			writeJSONError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
		}
		return nil, false
	}
	return flight, true
}

func candidateStatus(sequence *pb.PdcSequence) webPDCStatusResponse {
	result := webPDCStatusResponse{Callsign: sequence.Callsign, State: presentedWebState(sequence.State), CanSubmit: WebPDCCanSubmit(sequence.State)}
	result.RequiresPilotAction = result.State == string(StateCleared)
	result.RequestRemarks = optionalString(sequence.RequestRemarks)
	result.ClearanceText = optionalString(sequence.ClearanceText)
	if sequence.PilotAcknowledgedAt != nil {
		value := sequence.PilotAcknowledgedAt.AsTime().UTC().Format(time.RFC3339)
		result.PilotAcknowledgedAt = &value
	}
	return result
}

func candidateFailure(w http.ResponseWriter, status int, message string) {
	switch {
	case strings.Contains(message, "already pending"):
		writeJSONError(w, http.StatusConflict, "a web PDC has already been submitted for this aircraft")
	case strings.Contains(message, "invalid ATIS or aircraft type"):
		writeJSONError(w, http.StatusConflict, "aircraft type does not match the live strip")
	case strings.Contains(message, "strip not found"):
		writeJSONError(w, http.StatusNotFound, "no strip found for callsign")
	default:
		writeJSONError(w, status, message)
	}
}
