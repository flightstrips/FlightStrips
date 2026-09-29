// Package httpresults is the candidate JSON-to-command boundary. It is not
// installed by the PostgreSQL application runtime.
package httpresults

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
)

type Commander interface {
	Execute(context.Context, *pb.CommandRequest) *pb.CommandReply
}

type OutcomeReader interface {
	Outcome(context.Context, string, *pb.Actor) *pb.CommandReply
}

// CommandID accepts exactly one canonical UUID. It is called after the route's
// ordinary authentication and JSON validation, before submitting a mutation.
func CommandID(r *http.Request) (string, bool) {
	values := r.Header.Values("Idempotency-Key")
	if len(values) != 1 || strings.TrimSpace(values[0]) != values[0] {
		return "", false
	}
	id, err := uuid.Parse(values[0])
	return values[0], err == nil && id.String() == values[0]
}

// Submit constructs a typed request; no HTTP body bytes cross this boundary.
func Submit(ctx context.Context, store Commander, commandID string, aggregate *pb.AggregateRef, actor *pb.Actor, expected *uint64, action *pb.ClientCommand) *pb.CommandReply {
	if store == nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: commandID, Status: pb.CommandReply_UNAVAILABLE}
	}
	return store.Execute(ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: aggregate, Actor: actor, ExpectedEntityRevision: expected, Command: &pb.CommandRequest_Client{Client: action}})
}

// SubmitPilot closes the race between an initial outcome lookup and a command
// committed by another request. It rebuilds the original typed envelope once;
// the durable ledger still rejects a changed action under the same ID.
func SubmitPilot(ctx context.Context, store Commander, outcomes OutcomeReader, commandID, cid string, aggregate *pb.AggregateRef, expected *uint64, action *pb.ClientCommand) *pb.CommandReply {
	reply := Submit(ctx, store, commandID, aggregate, Pilot(cid, aggregate.GetSession().Id), expected, action)
	if reply == nil || reply.Status != pb.CommandReply_INVALID_ARGUMENT || outcomes == nil {
		return reply
	}
	prior := outcomes.Outcome(ctx, commandID, &pb.Actor{Kind: pb.Actor_PILOT, Id: cid})
	if prior == nil || prior.Outcome == nil || prior.Outcome.Aggregate.GetSession() == nil {
		return reply
	}
	original := prior.Outcome
	return Submit(ctx, store, commandID, original.Aggregate, Pilot(cid, original.Aggregate.GetSession().Id), original.ExpectedEntityRevision, action)
}

func Session(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}

func Pilot(cid string, session int32) *pb.Actor {
	return &pb.Actor{Kind: pb.Actor_PILOT, Id: cid, SessionId: &session}
}

// WriteResult retains each route's success body and status. Domain failures use
// the route's existing JSON error formatter through writeFailure.
func WriteResult(w http.ResponseWriter, commandID string, reply *pb.CommandReply, successStatus int, body any, writeFailure func(http.ResponseWriter, int, string)) {
	w.Header().Set("X-FlightStrips-Command-ID", commandID)
	if reply == nil {
		writeFailure(w, http.StatusServiceUnavailable, "command outcome unavailable")
		return
	}
	if outcome := reply.GetOutcome(); outcome != nil {
		w.Header().Set("X-FlightStrips-Outcome", statusName(outcome.Status))
		if outcome.Status == pb.CommandOutcome_ACCEPTED {
			writeJSON(w, http.StatusAccepted, body)
			return
		}
		if outcome.Status == pb.CommandOutcome_SUCCEEDED {
			writeJSON(w, successStatus, body)
			return
		}
		writeFailure(w, failureCode(outcome.ReasonCode), outcome.Detail)
		return
	}
	switch reply.Status {
	case pb.CommandReply_INVALID_ARGUMENT:
		writeFailure(w, http.StatusBadRequest, reply.Detail)
	case pb.CommandReply_UNAUTHORIZED:
		writeFailure(w, http.StatusForbidden, reply.Detail)
	case pb.CommandReply_NOT_FOUND:
		writeFailure(w, http.StatusNotFound, reply.Detail)
	case pb.CommandReply_REVISION_CONFLICT:
		writeFailure(w, http.StatusConflict, reply.Detail)
	default:
		writeFailure(w, http.StatusServiceUnavailable, "command outcome unavailable")
	}
}

func failureCode(reason string) int {
	switch reason {
	case "INVALID_ARGUMENT":
		return http.StatusBadRequest
	case "NOT_FOUND":
		return http.StatusNotFound
	case "REVISION_CONFLICT":
		return http.StatusConflict
	default:
		return http.StatusConflict
	}
}

func statusName(status pb.CommandOutcome_Status) string {
	return strings.ToLower(status.String())
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type Query struct {
	Auth     shared.AuthenticationService
	Outcomes OutcomeReader
}

func (q Query) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /commands/{uuid}", q.ServeHTTP)
}

func (q Query) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
	if q.Auth == nil || header == "" || token == header || token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization header"})
		return
	}
	user, err := q.Auth.Validate(token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	id := r.PathValue("uuid")
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.String() != id {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid command ID"})
		return
	}
	if q.Outcomes == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "command outcomes unavailable"})
		return
	}
	// HTTP pilot sessions have a stable authenticated CID. Candidate command
	// creation uses this same actor, independent of callsign changes.
	reply := q.Outcomes.Outcome(r.Context(), id, &pb.Actor{Id: user.GetCid()})
	if reply == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "command outcomes unavailable"})
		return
	}
	switch reply.Status {
	case pb.CommandReply_NOT_FOUND:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "command not found"})
		return
	case pb.CommandReply_UNAUTHORIZED:
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "command belongs to another actor"})
		return
	}
	outcome := reply.GetOutcome()
	if outcome == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "command outcomes unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, struct {
		CommandID         string `json:"command_id"`
		Status            string `json:"status"`
		ReasonCode        string `json:"reason_code"`
		Detail            string `json:"detail"`
		AggregateRevision uint64 `json:"aggregate_revision"`
	}{id, statusName(outcome.Status), outcome.ReasonCode, outcome.Detail, outcome.AggregateRevision})
}
