package efb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/httpresults"
	"FlightStrips/internal/services"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CandidateFlightReader interface {
	FindFlight(context.Context, string) (*cluster.FlightSnapshot, error)
}
type CandidateStandAvailability interface {
	AvailableForPilot(context.Context, int32, string, string) ([]services.StandAvailability, error)
}

// CandidateWebAPI uses typed projection reads and command replies. It is
// selected only by the coordinated NATS runtime.
type CandidateWebAPI struct {
	base         *WebAPI
	flights      CandidateFlightReader
	commands     httpresults.Commander
	outcomes     httpresults.OutcomeReader
	availability CandidateStandAvailability
}

func NewCandidateWebAPI(cfg WebAPIConfig, flights CandidateFlightReader, commands httpresults.Commander, outcomes httpresults.OutcomeReader, availability CandidateStandAvailability) *CandidateWebAPI {
	return &CandidateWebAPI{base: NewWebAPI(cfg), flights: flights, commands: commands, outcomes: outcomes, availability: availability}
}

func (a *CandidateWebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/efb/me", a.base.handleMe)
	mux.HandleFunc("/efb/flight", a.flight)
	mux.HandleFunc("/efb/tobt", a.tobt)
	mux.HandleFunc("/efb/stand", a.stand)
	mux.HandleFunc("/efb/stands", a.stands)
}

func (a *CandidateWebAPI) resolve(w http.ResponseWriter, r *http.Request, user shared.AuthenticatedUser, requested string) (*cluster.FlightSnapshot, bool) {
	callsign, found, err := a.base.lookupCallsign(r.Context(), user, requested)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "pilot lookup unavailable")
		return nil, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "no flight found for authenticated pilot")
		return nil, false
	}
	if a.flights == nil {
		writeError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
		return nil, false
	}
	flight, err := a.flights.FindFlight(r.Context(), callsign)
	if err != nil {
		switch {
		case errors.Is(err, cluster.ErrFlightNotFound):
			writeError(w, http.StatusNotFound, "no flight found for authenticated pilot")
		case errors.Is(err, cluster.ErrAmbiguousFlight):
			writeError(w, http.StatusConflict, "callsign matched multiple sessions")
		default:
			writeError(w, http.StatusServiceUnavailable, "flight lookup unavailable")
		}
		return nil, false
	}
	return flight, true
}

func (a *CandidateWebAPI) flight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.base.authenticate(w, r)
	if !ok {
		return
	}
	flight, ok := a.resolve(w, r, user, r.URL.Query().Get("callsign"))
	if !ok {
		return
	}
	s := flight.Strip
	departure := strings.EqualFold(s.Departure, flight.Airport)
	phase := "ARRIVAL"
	if departure {
		phase = "DEPARTURE"
	}
	result := snapshot{Callsign: s.Callsign, AircraftType: nonEmptyString(&s.AircraftType), Origin: s.Departure, Destination: s.Destination, Route: nonEmptyString(&s.Route), Phase: phase, Runway: nonEmptyString(&s.Runway), SID: nonEmptyString(&s.Sid), STAR: nonEmptyString(&s.Star), ClearedAltitude: s.ClearedAltitude, Squawk: nonEmptyString(&s.AssignedSquawk), Stand: nonEmptyString(&s.Stand), EOBT: candidateClock(s.Eobt), TOBT: candidateClock(s.Tobt), TSAT: candidateClock(s.Tsat), TTOT: candidateClock(s.Ttot), CTOT: candidateClock(s.Ctot), PDCState: "NONE", PDCAvailable: a.base.pdcReady && departure, Capabilities: capabilities{PDC: a.base.pdcReady && departure, TOBT: a.base.cdmReady && departure, Stand: a.availability != nil}}
	if flight.PDC != nil {
		result.PDCState = presentedCandidatePDCState(flight.PDC.State)
		result.PDCCanSubmit = flight.PDC.State == "NONE" || flight.PDC.State == "FAILED" || flight.PDC.State == "REVERT_TO_VOICE"
		result.PDCRequiresPilotAction = result.PDCState == "CLEARED"
		result.PDCClearanceText = nonEmptyString(&flight.PDC.ClearanceText)
	} else {
		result.PDCCanSubmit = result.PDCAvailable
	}
	if flight.Stand != nil {
		result.Stand = nonEmptyString(&flight.Stand.Stand)
		v := int32(flight.Stand.Revision)
		result.StandVersion = &v
	}
	if a.base.atis != nil {
		airport := s.Destination
		if departure {
			airport = s.Departure
		}
		result.ATIS = a.base.atis.GetATIS(airport, departure)
	}
	writeJSON(w, http.StatusOK, result)
}

func presentedCandidatePDCState(value string) string {
	if value == "REQUESTED_WITH_FAULTS" {
		return "REQUESTED"
	}
	return value
}
func candidateClock(value *timestamppb.Timestamp) *string {
	if value == nil {
		return nil
	}
	clock := value.AsTime().UTC().Format("1504")
	return &clock
}

func (a *CandidateWebAPI) target(w http.ResponseWriter, r *http.Request, id, cid string, flight *cluster.FlightSnapshot, revision uint64) (*pb.AggregateRef, *uint64, bool) {
	if a.outcomes == nil {
		writeError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, false
	}
	prior := a.outcomes.Outcome(r.Context(), id, &pb.Actor{Kind: pb.Actor_PILOT, Id: cid})
	if prior == nil {
		writeError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, false
	}
	if prior.Status == pb.CommandReply_UNAUTHORIZED {
		writeError(w, http.StatusForbidden, "command belongs to another actor")
		return nil, nil, false
	}
	if prior.Status != pb.CommandReply_NOT_FOUND && prior.Outcome == nil {
		writeError(w, http.StatusServiceUnavailable, "command outcomes unavailable")
		return nil, nil, false
	}
	if prior.Outcome != nil {
		if prior.Outcome.Aggregate.GetSession() == nil {
			writeError(w, http.StatusBadRequest, "command ID belongs to another action")
			return nil, nil, false
		}
		return prior.Outcome.Aggregate, prior.Outcome.ExpectedEntityRevision, true
	}
	return httpresults.Session(flight.SessionID), &revision, true
}

func (a *CandidateWebAPI) tobt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.base.authenticate(w, r)
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
	flight, ok := a.resolve(w, r, user, body.Callsign)
	if !ok {
		return
	}
	if !strings.EqualFold(flight.Strip.Departure, flight.Airport) {
		writeError(w, http.StatusConflict, "TOBT is only available for departures")
		return
	}
	if !a.base.cdmReady {
		writeError(w, http.StatusServiceUnavailable, "CDM unavailable")
		return
	}
	id, ok := httpresults.CommandID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid Idempotency-Key")
		return
	}
	ref, expected, ok := a.target(w, r, id, user.GetCid(), flight, flight.CDMRevision)
	if !ok {
		return
	}
	action := &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: flight.Strip.Callsign, Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{HhmmUtc: body.TOBT}}}}}
	reply := httpresults.SubmitPilot(r.Context(), a.commands, a.outcomes, id, user.GetCid(), ref, expected, action)
	httpresults.WriteResult(w, id, reply, http.StatusOK, map[string]string{"callsign": flight.Strip.Callsign, "tobt": body.TOBT}, writeError)
}

func (a *CandidateWebAPI) stand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.base.authenticate(w, r)
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
	flight, ok := a.resolve(w, r, user, body.Callsign)
	if !ok {
		return
	}
	id, ok := httpresults.CommandID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid Idempotency-Key")
		return
	}
	if body.Version < 0 {
		writeError(w, http.StatusConflict, "invalid stand version")
		return
	}
	ref, expected, ok := a.target(w, r, id, user.GetCid(), flight, uint64(body.Version))
	if !ok {
		return
	}
	action := &pb.ClientCommand{Action: &pb.ClientCommand_Stand{Stand: &pb.StandAction{Callsign: flight.Strip.Callsign, Stand: strings.ToUpper(strings.TrimSpace(body.Stand)), Change: &pb.StandAction_Manual{Manual: &pb.ManualStand{Reason: "pilot request"}}}}}
	reply := httpresults.SubmitPilot(r.Context(), a.commands, a.outcomes, id, user.GetCid(), ref, expected, action)
	if reply != nil && reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		if latest, err := a.flights.FindFlight(r.Context(), flight.Strip.Callsign); err == nil {
			flight = latest
		}
	}
	version := body.Version
	assignedStand := strings.ToUpper(strings.TrimSpace(body.Stand))
	if flight.Stand != nil {
		version = int32(flight.Stand.Revision)
		assignedStand = flight.Stand.Stand
	}
	httpresults.WriteResult(w, id, reply, http.StatusOK, map[string]any{"callsign": flight.Strip.Callsign, "stand": assignedStand, "version": version}, writeError)
}

func (a *CandidateWebAPI) stands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	user, ok := a.base.authenticate(w, r)
	if !ok {
		return
	}
	flight, ok := a.resolve(w, r, user, r.URL.Query().Get("callsign"))
	if !ok {
		return
	}
	if a.availability == nil {
		writeError(w, http.StatusServiceUnavailable, "stand availability unavailable")
		return
	}
	stands, err := a.availability.AvailableForPilot(r.Context(), flight.SessionID, flight.Airport, flight.Strip.Callsign)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "stand availability unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stands": stands})
}
