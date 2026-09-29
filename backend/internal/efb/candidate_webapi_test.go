package efb

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
)

type candidateAuth struct{}

func (candidateAuth) Validate(token string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser(token, 0, nil), nil
}

type candidateFlights struct{ flight *cluster.FlightSnapshot }

func (f candidateFlights) FindFlight(context.Context, string) (*cluster.FlightSnapshot, error) {
	return f.flight, nil
}

type candidateCommands struct {
	request *pb.CommandRequest
	status  pb.CommandOutcome_Status
}

func (c *candidateCommands) Execute(_ context.Context, request *pb.CommandRequest) *pb.CommandReply {
	c.request = request
	return &pb.CommandReply{Status: pb.CommandReply_COMMITTED, Outcome: &pb.CommandOutcome{Status: c.status, CommandId: request.CommandId}}
}
func (c *candidateCommands) Outcome(context.Context, string, *pb.Actor) *pb.CommandReply {
	return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND}
}

func TestCandidateEFBTypedMutationsKeepJSONShapes(t *testing.T) {
	flight := &cluster.FlightSnapshot{SessionID: 1, Airport: "EKCH", Strip: &pb.Strip{Callsign: "SAS101", Departure: "EKCH", Destination: "ENGM", AircraftType: "A320"}}
	commands := &candidateCommands{status: pb.CommandOutcome_SUCCEEDED}
	api := NewCandidateWebAPI(WebAPIConfig{Auth: candidateAuth{}, Live: false, CDMReady: true, PDCReady: true}, candidateFlights{flight}, commands, commands, nil)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	request := func(path, method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer 123")
		r.Header.Set("Idempotency-Key", uuid.NewString())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	stand := request("/efb/stand", http.MethodPut, `{"stand":"B1","version":0,"callsign":"SAS101"}`)
	if stand.Code != http.StatusOK || stand.Body.String() != "{\"callsign\":\"SAS101\",\"stand\":\"B1\",\"version\":0}\n" || commands.request.GetClient().GetStand().GetManual() == nil || commands.request.GetActor().GetId() != "123" {
		t.Fatalf("stand: %d %s %v", stand.Code, stand.Body.String(), commands.request)
	}
	commands.status = pb.CommandOutcome_ACCEPTED
	tobt := request("/efb/tobt", http.MethodPut, `{"tobt":"1030","callsign":"SAS101"}`)
	if tobt.Code != http.StatusAccepted || tobt.Header().Get("X-FlightStrips-Outcome") != "accepted" || tobt.Body.String() != "{\"callsign\":\"SAS101\",\"tobt\":\"1030\"}\n" || commands.request.GetClient().GetCdm().GetSetTobt().GetHhmmUtc() != "1030" {
		t.Fatalf("TOBT: %d %s %v", tobt.Code, tobt.Body.String(), commands.request)
	}
	bad := request("/efb/tobt", http.MethodPut, `{"tobt":"bad","callsign":"SAS101"}`)
	if bad.Code != http.StatusBadRequest || bad.Body.String() != "{\"error\":\"tobt must be HHMM\"}\n" {
		t.Fatalf("validation: %d %s", bad.Code, bad.Body.String())
	}
	read := request("/efb/flight?callsign=SAS101", http.MethodGet, "")
	if read.Code != http.StatusOK || !bytes.Contains(read.Body.Bytes(), []byte(`"callsign":"SAS101"`)) || !bytes.Contains(read.Body.Bytes(), []byte(`"pdc_state":"NONE"`)) {
		t.Fatalf("read: %d %s", read.Code, read.Body.String())
	}
}
