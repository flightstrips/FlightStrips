package pdc

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/httpresults"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type candidateAuth struct{}

func (candidateAuth) Validate(token string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser(token, 0, nil), nil
}

type candidateVerifier struct{}

func (candidateVerifier) VerifyPilotOwnsCallsign(_ context.Context, cid, callsign string) (bool, error) {
	return cid == "123" && callsign == "SAS101", nil
}

type candidateFlights struct{ flight *cluster.FlightSnapshot }

func (f *candidateFlights) FindFlight(_ context.Context, _ string) (*cluster.FlightSnapshot, error) {
	return f.flight, nil
}

type candidateLedger struct {
	flight  *candidateFlights
	outcome *pb.CommandOutcome
	writes  int
	status  pb.CommandOutcome_Status
}

func (l *candidateLedger) Execute(_ context.Context, request *pb.CommandRequest) *pb.CommandReply {
	hash, err := cluster.RequestHash(request)
	if err != nil {
		panic(err)
	}
	if l.outcome != nil {
		if !proto.Equal(request.Actor, l.outcome.Actor) {
			return &pb.CommandReply{Status: pb.CommandReply_UNAUTHORIZED}
		}
		if hash != l.outcome.RequestSha256 {
			return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "command ID has different content"}
		}
		return &pb.CommandReply{Status: pb.CommandReply_COMMITTED, Outcome: proto.Clone(l.outcome).(*pb.CommandOutcome)}
	}
	l.writes++
	expected := *request.ExpectedEntityRevision
	l.outcome = &pb.CommandOutcome{CommandId: request.CommandId, RequestSha256: hash, Actor: proto.Clone(request.Actor).(*pb.Actor), Aggregate: proto.Clone(request.Aggregate).(*pb.AggregateRef), ExpectedEntityRevision: &expected, Status: l.status, AggregateRevision: 8}
	l.flight.flight.PDC = &pb.PdcSequence{Callsign: "SAS101", State: "REQUESTED", RequestRemarks: request.GetClient().GetPdc().GetIssue().RequestRemarks}
	l.flight.flight.PDCRevision++
	status := pb.CommandReply_COMMITTED
	if l.status == pb.CommandOutcome_ACCEPTED {
		status = pb.CommandReply_PENDING
	}
	return &pb.CommandReply{Status: status, Outcome: proto.Clone(l.outcome).(*pb.CommandOutcome)}
}

func (l *candidateLedger) Outcome(_ context.Context, id string, actor *pb.Actor) *pb.CommandReply {
	if l.outcome == nil || id != l.outcome.CommandId {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND}
	}
	if actor.GetId() != l.outcome.Actor.Id || actor.Kind != pb.Actor_KIND_UNSPECIFIED && actor.Kind != l.outcome.Actor.Kind {
		return &pb.CommandReply{Status: pb.CommandReply_UNAUTHORIZED}
	}
	return &pb.CommandReply{Status: pb.CommandReply_COMMITTED, Outcome: proto.Clone(l.outcome).(*pb.CommandOutcome)}
}

func TestCandidateWebPDCIdempotencyAndPendingResult(t *testing.T) {
	flights := &candidateFlights{flight: &cluster.FlightSnapshot{SessionID: 1, Airport: "EKCH", Strip: &pb.Strip{Callsign: "SAS101", AircraftType: "A320"}}}
	ledger := &candidateLedger{flight: flights, status: pb.CommandOutcome_ACCEPTED}
	api := NewCandidateWebAPI(candidateAuth{}, candidateVerifier{}, true, flights, ledger, ledger)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	id := uuid.NewString()
	request := func(atis string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(requestWebPDCBody{Callsign: "SAS101", AircraftType: "A320", Atis: atis, Remarks: "test"})
		r := httptest.NewRequest(http.MethodPost, "/pdc/request", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer 123")
		r.Header.Set("Idempotency-Key", id)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for i := 0; i < 2; i++ {
		w := request("B")
		if w.Code != http.StatusAccepted || w.Header().Get("X-FlightStrips-Command-ID") != id || w.Header().Get("X-FlightStrips-Outcome") != "accepted" || w.Body.String() != "{\"callsign\":\"SAS101\"}\n" {
			t.Fatalf("request %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	if ledger.writes != 1 {
		t.Fatalf("duplicate committed %d times", ledger.writes)
	}
	if w := request("C"); w.Code != http.StatusBadRequest || ledger.writes != 1 {
		t.Fatalf("changed body: %d %s", w.Code, w.Body.String())
	}
	for _, tc := range []struct {
		token, key string
		want       int
	}{{"123", "", http.StatusBadRequest}, {"456", uuid.NewString(), http.StatusForbidden}} {
		r := httptest.NewRequest(http.MethodPost, "/pdc/request", bytes.NewBufferString(`{"callsign":"SAS101","aircraft_type":"A320","atis":"B"}`))
		r.Header.Set("Authorization", "Bearer "+tc.token)
		if tc.key != "" {
			r.Header.Set("Idempotency-Key", tc.key)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != tc.want || ledger.writes != 1 {
			t.Fatalf("precommit rejection: %d %s", w.Code, w.Body.String())
		}
	}
	query := http.NewServeMux()
	(httpresults.Query{Auth: candidateAuth{}, Outcomes: ledger}).RegisterRoutes(query)
	lookup := func(cid string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/commands/"+id, nil)
		r.Header.Set("Authorization", "Bearer "+cid)
		w := httptest.NewRecorder()
		query.ServeHTTP(w, r)
		return w
	}
	if w := lookup("456"); w.Code != http.StatusForbidden {
		t.Fatalf("other actor: %d %s", w.Code, w.Body.String())
	}
	if w := lookup("123"); w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"accepted"`)) {
		t.Fatalf("pending query: %d %s", w.Code, w.Body.String())
	}
	ledger.outcome.Status = pb.CommandOutcome_SUCCEEDED
	if w := lookup("123"); w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"succeeded"`)) {
		t.Fatalf("terminal query: %d %s", w.Code, w.Body.String())
	}
}
