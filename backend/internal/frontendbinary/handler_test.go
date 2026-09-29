package frontendbinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type fixtureAuth struct{}

func (fixtureAuth) Validate(token string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser("123456", 1, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())}}), nil
}

type fixtureRouter struct {
	mu      sync.Mutex
	request *pb.CommandRequest
}

func (r *fixtureRouter) Route(_ context.Context, request *pb.CommandRequest) *pb.CommandReply {
	r.mu.Lock()
	r.request = proto.Clone(request).(*pb.CommandRequest)
	r.mu.Unlock()
	return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_COMMITTED,
		Outcome: &pb.CommandOutcome{CommandId: request.CommandId, Status: pb.CommandOutcome_SUCCEEDED, AggregateRevision: 4}}
}

type fixtureProjection struct {
	session, airport, global *cluster.Aggregate
	updates                  chan *pb.FrontendDelta
	observations             chan *pb.FrontendObservation
	buffered                 *pb.FrontendDelta
	bufferedObservation      *pb.FrontendObservation
	initialObservation       *pb.FrontendObservation
}

func (p *fixtureProjection) Ready() error { return nil }
func (p *fixtureProjection) Read(ref *pb.AggregateRef) (*cluster.Aggregate, error) {
	switch ref.Target.(type) {
	case *pb.AggregateRef_Global:
		return p.global, nil
	case *pb.AggregateRef_Airport:
		return p.airport, nil
	default:
		return p.session, nil
	}
}
func (p *fixtureProjection) SubscribeInitial(ref *pb.AggregateRef) (*cluster.Aggregate, <-chan *pb.FrontendDelta, func(), error) {
	if ref.GetSession() != nil {
		if p.buffered != nil {
			p.updates <- p.buffered
			p.buffered = nil
		}
		return p.session, p.updates, func() {}, nil
	}
	return p.airport, make(chan *pb.FrontendDelta), func() {}, nil
}
func (p *fixtureProjection) SubscribeObservedInitial(int32) (*pb.FrontendInitial, <-chan *pb.FrontendDelta, <-chan *pb.FrontendObservation, func(), error) {
	initial := &pb.FrontendInitial{SessionId: 7, Airport: "EKCH", SessionName: "TEST",
		AggregateRevision: p.session.Revision, StreamSequence: p.session.StreamSequence, Writable: true}
	for _, entity := range p.session.Entities {
		initial.Entities = append(initial.Entities, proto.Clone(entity).(*pb.EntitySnapshot))
	}
	if p.initialObservation != nil {
		initial.TaggedObservations = append(initial.TaggedObservations, p.initialObservation)
	}
	if p.buffered != nil {
		p.updates <- p.buffered
		p.buffered = nil
	}
	if p.bufferedObservation != nil {
		p.observations <- p.bufferedObservation
		p.bufferedObservation = nil
	}
	return initial, p.updates, p.observations, func() {}, nil
}

func frontendFixture() *fixtureProjection {
	globalRef := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	airportRef := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	sessionRef := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 7}}}
	global := cluster.NewAggregate(globalRef)
	registry := &pb.EntitySnapshot{Key: "7", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionRegistry{SessionRegistry: &pb.SessionRegistry{Id: 7, Airport: "EKCH", Name: "TEST", State: pb.SessionRegistry_ACTIVE}}}}
	global.Indexes[pb.EntityKind_SESSION_REGISTRY] = map[string]*pb.EntitySnapshot{"7": registry}
	session := cluster.NewAggregate(sessionRef)
	session.Revision, session.StreamSequence = 3, 30
	for _, item := range []*pb.EntitySnapshot{
		{Key: "7", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 7, Airport: "EKCH", Name: "TEST"}}}},
		{Key: "123456", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: &pb.Controller{Cid: "123456", Callsign: "EKCH_TWR", Position: "TWR"}}}},
	} {
		session.Entities[item.Key] = item
		if item.GetValue().GetSession() != nil {
			session.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{item.Key: item}
		}
		if item.GetValue().GetController() != nil {
			session.Indexes[pb.EntityKind_CONTROLLER] = map[string]*pb.EntitySnapshot{item.Key: item}
		}
	}
	airport := cluster.NewAggregate(airportRef)
	airport.Revision = 9
	return &fixtureProjection{session: session, airport: airport, global: global, updates: make(chan *pb.FrontendDelta, 8), observations: make(chan *pb.FrontendObservation, 8)}
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.DefaultDialer.Dial(strings.Replace(url, "http", "ws", 1), nil)
	if err == nil {
		conn.Close()
		t.Fatal("accepted socket without required subprotocol")
	}
	if response == nil || response.StatusCode != 403 {
		t.Fatalf("unexpected response: %v %v", response, err)
	}
	conn, _, err = (&websocket.Dialer{Subprotocols: []string{Subprotocol}}).Dial(strings.Replace(url, "http", "ws", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeTestFrame(t *testing.T, conn *websocket.Conn, frame *pb.FrontendFrame) {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatal(err)
	}
}
func readTestFrame(t *testing.T, conn *websocket.Conn) *pb.FrontendFrame {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	kind, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if kind != websocket.BinaryMessage {
		t.Fatalf("expected binary, got %d", kind)
	}
	frame := &pb.FrontendFrame{}
	if err := pb.UnmarshalStrict(data, frame); err != nil {
		t.Fatal(err)
	}
	return frame
}
func authFrame() *pb.FrontendFrame {
	return &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "valid"}}}
}

func TestReplicaReconnectAndGapFreeHandoff(t *testing.T) {
	first, second := frontendFixture(), frontendFixture()
	first.buffered = &pb.FrontendDelta{Aggregate: first.session.Ref, AggregateRevision: 4, StreamSequence: 31}
	second.buffered = proto.Clone(first.buffered).(*pb.FrontendDelta)
	router := &fixtureRouter{}
	servers := []*httptest.Server{httptest.NewServer(Handler{Projection: first, Router: router, Auth: fixtureAuth{}}), httptest.NewServer(Handler{Projection: second, Router: router, Auth: fixtureAuth{}})}
	defer servers[0].Close()
	defer servers[1].Close()
	var firstInitial *pb.FrontendInitial
	var firstDelta *pb.FrontendDelta
	for i, server := range servers {
		conn := dial(t, server.URL)
		writeTestFrame(t, conn, authFrame())
		initial := readTestFrame(t, conn).GetInitial()
		if initial == nil || initial.AggregateRevision != 3 || initial.AirportAggregateRevision != 9 || len(initial.Entities) != 2 {
			t.Fatalf("replica %d initial: %+v", i, initial)
		}
		if i == 0 {
			firstInitial = initial
		} else if !proto.Equal(firstInitial, initial) {
			t.Fatal("replicas sent different typed snapshots for the same checkpoint")
		}
		delta := readTestFrame(t, conn).GetDelta()
		if delta == nil || delta.AggregateRevision != 4 {
			t.Fatalf("replica %d lost a delta buffered during initial load: %+v", i, delta)
		}
		if i == 0 {
			firstDelta = delta
			id := uuid.NewString()
			writeTestFrame(t, conn, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id, Action: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: "SAS123", Change: &pb.StripAction_GenerateSquawk{GenerateSquawk: &pb.GenerateSquawk{}}}}}}}})
			if result := readTestFrame(t, conn).GetActionResult(); result == nil || result.RequestId != id || result.Status != pb.CommandOutcome_SUCCEEDED {
				t.Fatalf("action result: %+v", result)
			}
			router.mu.Lock()
			request := router.request
			router.mu.Unlock()
			if request == nil || request.CommandId != id || request.Actor.Id != "123456" || request.Aggregate.GetSession().Id != 7 {
				t.Fatalf("wrong owner route: %+v", request)
			}
		} else if !proto.Equal(firstDelta, delta) {
			t.Fatal("replicas sent different typed deltas for the same event")
		}
		conn.Close()
	}
	// The second node has replayed the buffered event before this reconnect.
	second.session.Revision, second.session.StreamSequence = 4, 31
	conn := dial(t, servers[1].URL)
	defer conn.Close()
	writeTestFrame(t, conn, authFrame())
	if initial := readTestFrame(t, conn).GetInitial(); initial == nil || initial.AggregateRevision != 4 || initial.StreamSequence != 31 {
		t.Fatalf("reconnect did not use the replayed checkpoint: %+v", initial)
	}
}

func TestObservationBufferedAcrossInitial(t *testing.T) {
	projection := frontendFixture()
	position := &pb.PositionValue{SessionId: 7, AircraftKey: "SAS123", OwnerEpoch: 2,
		Observation: &pb.PositionValue_Position{Position: &pb.AircraftPosition{AltitudeFeet: 300}}}
	projection.initialObservation = &pb.FrontendObservation{Value: &pb.FrontendObservation_Position{Position: position}, SourceRevision: 10, Stale: true}
	projection.bufferedObservation = &pb.FrontendObservation{Value: &pb.FrontendObservation_Position{Position: position}, SourceRevision: 11}
	server := httptest.NewServer(Handler{Projection: projection, Router: &fixtureRouter{}, Auth: fixtureAuth{}})
	defer server.Close()
	conn := dial(t, server.URL)
	defer conn.Close()
	writeTestFrame(t, conn, authFrame())
	initial := readTestFrame(t, conn).GetInitial()
	if initial == nil || len(initial.TaggedObservations) != 1 || initial.TaggedObservations[0].SourceRevision != 10 || !initial.TaggedObservations[0].Stale {
		t.Fatalf("missing tagged initial observation: %+v", initial)
	}
	observation := readTestFrame(t, conn).GetObservation()
	if observation == nil || observation.SourceRevision != 11 || observation.Stale || observation.GetPosition().GetPosition().AltitudeFeet != 300 {
		t.Fatalf("buffered live observation lost: %+v", observation)
	}
}

func TestRejectsTextMalformedAndRevision(t *testing.T) {
	for _, test := range []struct {
		name string
		kind int
		data []byte
		want int
	}{
		{"text", websocket.TextMessage, []byte("hello"), 1003},
		{"malformed", websocket.BinaryMessage, []byte{0xff}, 1002},
		{"oversized", websocket.BinaryMessage, make([]byte, maxFrame+1), 1009},
		{"revision", websocket.BinaryMessage, mustMarshal(&pb.FrontendFrame{ProtocolRevision: 3, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: "valid"}}}), 1008},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(Handler{Projection: frontendFixture(), Router: &fixtureRouter{}, Auth: fixtureAuth{}})
			defer server.Close()
			conn := dial(t, server.URL)
			defer conn.Close()
			if err := conn.WriteMessage(test.kind, test.data); err != nil {
				t.Fatal(err)
			}
			_, _, err := conn.ReadMessage()
			if closeErr, ok := err.(*websocket.CloseError); !ok || closeErr.Code != test.want {
				t.Fatalf("close: %v", err)
			}
		})
	}
}

func TestMalformedNestedActionGetsTypedProtocolError(t *testing.T) {
	server := httptest.NewServer(Handler{Projection: frontendFixture(), Router: &fixtureRouter{}, Auth: fixtureAuth{}})
	defer server.Close()
	conn := dial(t, server.URL)
	defer conn.Close()
	writeTestFrame(t, conn, authFrame())
	_ = readTestFrame(t, conn)
	writeTestFrame(t, conn, &pb.FrontendFrame{ProtocolRevision: 2,
		Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: uuid.NewString(),
			Action: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: "SAS123"}}}}}})
	response := readTestFrame(t, conn).GetError()
	if response == nil || response.Code != pb.FrontendError_INVALID_FRAME {
		t.Fatalf("expected typed invalid-frame error, got %+v", response)
	}
}
func mustMarshal(value proto.Message) []byte { data, _ := proto.Marshal(value); return data }
