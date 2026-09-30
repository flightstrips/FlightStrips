package frontendbinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const Subprotocol = "flightstrips.frontend.pb.v2"
const maxFrame = 4 << 20

// Projection is deliberately small so the same endpoint can be used by every
// NATS-backed node. SubscribeInitial registers the listener before it returns
// its checkpoint, including while the initial frame is being sent.
type Projection interface {
	Ready() error
	Read(*pb.AggregateRef) (*cluster.Aggregate, error)
	SubscribeObservedInitial(int32) (*pb.FrontendInitial, <-chan *pb.FrontendDelta, <-chan *pb.FrontendObservation, func(), error)
	SubscribeInitial(*pb.AggregateRef) (*cluster.Aggregate, <-chan *pb.FrontendDelta, func(), error)
}

type Router interface {
	Route(context.Context, *pb.CommandRequest) *pb.CommandReply
}

type Handler struct {
	Projection Projection
	Router     Router
	Auth       shared.AuthenticationService
	// NodeID enables per-socket FS_PRESENCE in the opt-in NATS runtime.
	NodeID string
}

var upgrade = websocket.Upgrader{Subprotocols: []string{Subprotocol}, CheckOrigin: func(*http.Request) bool { return true }}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	if h.Projection == nil || h.Router == nil || h.Auth == nil {
		http.Error(w, "frontend unavailable", http.StatusServiceUnavailable)
		return
	}
	if !contains(websocket.Subprotocols(r), Subprotocol) {
		http.Error(w, "unsupported websocket subprotocol", http.StatusForbidden)
		return
	}
	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
	defer stop()
	conn.SetReadLimit(maxFrame)
	_ = h.serve(r.Context(), conn)
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type closeFailure struct {
	code   int
	reason string
}

func (e closeFailure) Error() string { return e.reason }

func readFrame(conn *websocket.Conn) (*pb.FrontendFrame, error) {
	kind, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if kind != websocket.BinaryMessage {
		return nil, closeFailure{websocket.CloseUnsupportedData, "binary frames required"}
	}
	if len(data) == 0 || len(data) > maxFrame {
		return nil, closeFailure{websocket.CloseProtocolError, "invalid frame size"}
	}
	frame := &pb.FrontendFrame{}
	if err := pb.UnmarshalStrict(data, frame); err != nil || frame.GetFrame() == nil {
		return nil, closeFailure{websocket.CloseProtocolError, "invalid protobuf frame"}
	}
	if frame.ProtocolRevision != 2 {
		return nil, closeFailure{websocket.ClosePolicyViolation, "unsupported revision"}
	}
	return frame, nil
}

func (h Handler) serve(ctx context.Context, conn *websocket.Conn) (result error) {
	ctx, cancel := context.WithCancel(ctx)
	var jobs sync.WaitGroup
	defer func() {
		if result != nil {
			var failure closeFailure
			code, reason := websocket.CloseTryAgainLater, "projection unavailable"
			if errors.As(result, &failure) {
				code, reason = failure.code, failure.reason
			}
			_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), time.Now().Add(time.Second))
		}
		cancel()
		_ = conn.Close()
		jobs.Wait()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	first, err := readFrame(conn)
	if err != nil {
		return err
	}
	auth := first.GetAuthenticate()
	if auth == nil || auth.BearerToken == "" {
		return closeFailure{websocket.ClosePolicyViolation, "authentication required"}
	}
	user, err := h.Auth.Validate(auth.BearerToken)
	if err != nil || !user.IsValid() {
		return closeFailure{websocket.ClosePolicyViolation, "authentication failed"}
	}
	identity, err := h.resolve(&user, auth)
	if err != nil {
		return closeFailure{websocket.ClosePolicyViolation, "session unavailable"}
	}
	_ = conn.SetReadDeadline(time.Time{})
	var presenceErrors <-chan error
	if h.NodeID != "" {
		projection, ok := h.Projection.(*cluster.Projection)
		if !ok || projection.Presence == nil {
			return fmt.Errorf("frontend presence unavailable")
		}
		lease, err := cluster.NewSocketPresenceLease(projection.Presence, h.NodeID, identity.id,
			identity.me.Cid, identity.me.Callsign, identity.me.Position, identity.me.Observer, pb.ClientPresence_FRONTEND)
		if err != nil {
			return err
		}
		presenceCtx, stopPresence := context.WithCancel(ctx)
		defer stopPresence()
		if _, err := lease.Renew(presenceCtx); err != nil {
			return err
		}
		errs := make(chan error, 1)
		presenceErrors = errs
		jobs.Add(1)
		go func() { defer jobs.Done(); errs <- lease.Run(presenceCtx) }()
	}
	session, sessionUpdates, observations, stopSession, err := h.Projection.SubscribeObservedInitial(identity.id)
	if err != nil {
		return err
	}
	defer stopSession()
	airport, airportUpdates, stopAirport, err := h.Projection.SubscribeInitial(identity.airportRef)
	if err != nil {
		return err
	}
	defer stopAirport()
	initial := buildInitial(identity, session, airport)
	if err := send(conn, &pb.FrontendFrame_Initial{Initial: initial}); err != nil {
		return err
	}
	// The projection listeners were installed before building initial. Their
	// buffered events are read after the frame is written, with per-aggregate
	// revisions checked before delivery.
	read := make(chan readResult, 1)
	jobs.Add(1)
	go func() {
		defer jobs.Done()
		for {
			frame, err := readFrame(conn)
			select {
			case read <- readResult{frame, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	lastSession, lastAirport := session.AggregateRevision, airport.Revision
	pending := map[string]*pb.AggregateRef{}
	for {
		if err := h.Projection.Ready(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-presenceErrors:
			return fmt.Errorf("frontend presence renewal: %w", err)
		case item := <-read:
			if item.err != nil {
				return item.err
			}
			if err := h.handleFrame(ctx, conn, identity, item.frame, pending); err != nil {
				return err
			}
		case delta, ok := <-sessionUpdates:
			if !ok {
				return fmt.Errorf("session delivery overflow")
			}
			if err := sendDelta(conn, delta, &lastSession); err != nil {
				return err
			}
			if err := h.sendTerminalResults(conn, identity, pending, identity.sessionRef); err != nil {
				return err
			}
		case delta, ok := <-airportUpdates:
			if !ok {
				return fmt.Errorf("airport delivery overflow")
			}
			if err := sendDelta(conn, delta, &lastAirport); err != nil {
				return err
			}
			if err := h.sendTerminalResults(conn, identity, pending, identity.airportRef); err != nil {
				return err
			}
		case observation, ok := <-observations:
			if !ok {
				return fmt.Errorf("observation delivery overflow")
			}
			if err := send(conn, &pb.FrontendFrame_Observation{Observation: observation}); err != nil {
				return err
			}
		}
	}
}

type readResult struct {
	frame *pb.FrontendFrame
	err   error
}

func send(conn *websocket.Conn, value any) error {
	frame := &pb.FrontendFrame{ProtocolRevision: 2}
	switch v := value.(type) {
	case *pb.FrontendFrame_Initial:
		frame.Frame = v
	case *pb.FrontendFrame_Delta:
		frame.Frame = v
	case *pb.FrontendFrame_Observation:
		frame.Frame = v
	case *pb.FrontendFrame_ActionResult:
		frame.Frame = v
	case *pb.FrontendFrame_StatusMissing:
		frame.Frame = v
	case *pb.FrontendFrame_Error:
		frame.Frame = v
	default:
		return fmt.Errorf("unsupported outgoing frame")
	}
	data, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	if len(data) > maxFrame {
		return closeFailure{websocket.CloseMessageTooBig, "frontend frame too large"}
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func sendDelta(conn *websocket.Conn, delta *pb.FrontendDelta, revision *uint64) error {
	if delta == nil {
		return fmt.Errorf("empty projection delta")
	}
	if delta.AggregateRevision <= *revision {
		return nil
	}
	if delta.AggregateRevision != *revision+1 {
		return fmt.Errorf("projection revision gap")
	}
	if err := send(conn, &pb.FrontendFrame_Delta{Delta: delta}); err != nil {
		return err
	}
	*revision = delta.AggregateRevision
	return nil
}

type identity struct {
	id                     int32
	airport, name          string
	me                     *pb.Controller
	sessionRef, airportRef *pb.AggregateRef
}

func (h Handler) resolve(user *shared.AuthenticatedUser, auth *pb.FrontendAuthenticate) (identity, error) {
	global, err := h.Projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}})
	if err != nil {
		return identity{}, err
	}
	ids := make([]int, 0)
	for _, item := range global.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
		entry := item.GetValue().GetSessionRegistry()
		if entry != nil && entry.State == pb.SessionRegistry_ACTIVE && (auth.Airport == "" || strings.EqualFold(auth.Airport, entry.Airport)) && (auth.SessionName == "" || auth.SessionName == entry.Name) {
			ids = append(ids, int(entry.Id))
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		ref := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: int32(id)}}}
		state, err := h.Projection.Read(ref)
		if err != nil {
			return identity{}, err
		}
		controller := state.EntitiesByKind(pb.EntityKind_CONTROLLER)
		for _, record := range controller {
			me := record.GetValue().GetController()
			if me == nil || me.Cid != user.GetCid() {
				continue
			}
			sessionRecord := state.EntitiesByKind(pb.EntityKind_SESSION)
			if len(sessionRecord) != 1 {
				return identity{}, fmt.Errorf("missing session")
			}
			s := sessionRecord[0].GetValue().GetSession()
			if s == nil || s.Tombstoned {
				return identity{}, fmt.Errorf("inactive session")
			}
			return identity{id: int32(id), airport: s.Airport, name: s.Name, me: me, sessionRef: ref, airportRef: &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: s.Airport}}}}, nil
		}
	}
	return identity{}, fmt.Errorf("controller has no active session")
}

func buildInitial(who identity, session *pb.FrontendInitial, airport *cluster.Aggregate) *pb.FrontendInitial {
	initial := proto.Clone(session).(*pb.FrontendInitial)
	initial.AirportAggregateRevision = airport.Revision
	initial.Me = who.me
	initial.ReadOnly = who.me.Observer
	initial.Writable = initial.Writable && !who.me.Observer
	if who.me.Observer {
		initial.PositionAvailable = false
		for _, client := range initial.Clients {
			if client.Kind == pb.ClientPresence_EUROSCOPE && client.Position == who.me.Position {
				initial.PositionAvailable = true
				break
			}
		}
	}
	initial.AmanFmp = strings.HasSuffix(strings.ToUpper(who.me.Callsign), "_FMP")
	if who.me.LayoutId != "" {
		initial.LayoutId = who.me.LayoutId
	}
	for _, entity := range airport.Entities {
		initial.Entities = append(initial.Entities, proto.Clone(entity).(*pb.EntitySnapshot))
	}
	sort.Slice(initial.Entities, func(i, j int) bool { return initial.Entities[i].Key < initial.Entities[j].Key })
	return initial
}

func (h Handler) handleFrame(ctx context.Context, conn *websocket.Conn, who identity, frame *pb.FrontendFrame, pending map[string]*pb.AggregateRef) error {
	switch value := frame.GetFrame().(type) {
	case *pb.FrontendFrame_Command:
		command := value.Command
		if command == nil || uuid.Validate(command.RequestId) != nil || uuid.MustParse(command.RequestId).String() != command.RequestId || command.Action == nil || command.Action.GetAction() == nil {
			return send(conn, &pb.FrontendFrame_Error{Error: &pb.FrontendError{Code: pb.FrontendError_INVALID_FRAME, Detail: "invalid command"}})
		}
		if err := requireOneofs(command.Action.ProtoReflect()); err != nil {
			return send(conn, &pb.FrontendFrame_Error{Error: &pb.FrontendError{Code: pb.FrontendError_INVALID_FRAME, Detail: err.Error()}})
		}
		if who.me.Observer && command.Action.GetAman() == nil {
			return h.reject(conn, command.RequestId, who.sessionRef, "UNAUTHORIZED", "observer is read-only")
		}
		if command.Action.GetAman() != nil && !strings.HasSuffix(strings.ToUpper(who.me.Callsign), "_FMP") {
			return h.reject(conn, command.RequestId, who.airportRef, "UNAUTHORIZED", "AMAN controller role required")
		}
		ref := who.sessionRef
		if command.Action.GetAman() != nil {
			ref = who.airportRef
		}
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command.RequestId, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: who.me.Cid, SessionId: &who.id}, ExpectedEntityRevision: command.ExpectedEntityRevision, Command: &pb.CommandRequest_Client{Client: command.Action}}
		if _, err := cluster.RequestHash(request); err != nil {
			return h.reject(conn, command.RequestId, ref, "INVALID_ARGUMENT", err.Error())
		}
		reply := h.Router.Route(ctx, request)
		result := &pb.FrontendActionResult{RequestId: command.RequestId, Aggregate: ref}
		if reply == nil {
			result.Status, result.ReasonCode = pb.CommandOutcome_UNKNOWN, "UNAVAILABLE"
		} else if reply.Outcome != nil {
			result.Status, result.ReasonCode, result.Detail, result.AggregateRevision = reply.Outcome.Status, reply.Outcome.ReasonCode, reply.Outcome.Detail, reply.Outcome.AggregateRevision
		} else {
			result.Status, result.ReasonCode, result.Detail = pb.CommandOutcome_FAILED, reply.Status.String(), reply.Detail
			// A transport reply without a projected outcome cannot prove that an
			// effect was accepted or that a backend change committed.
			if reply.Status == pb.CommandReply_COMMITTED || reply.Status == pb.CommandReply_PENDING || reply.Status == pb.CommandReply_UNAVAILABLE || reply.Status == pb.CommandReply_NOT_OWNER {
				result.Status = pb.CommandOutcome_UNKNOWN
			}
		}
		if result.Status == pb.CommandOutcome_ACCEPTED {
			pending[command.RequestId] = ref
		}
		return send(conn, &pb.FrontendFrame_ActionResult{ActionResult: result})
	case *pb.FrontendFrame_StatusQuery:
		if value.StatusQuery == nil || len(value.StatusQuery.RequestIds) > 100 {
			return closeFailure{websocket.CloseProtocolError, "invalid status query"}
		}
		for _, id := range value.StatusQuery.RequestIds {
			if uuid.Validate(id) != nil || uuid.MustParse(id).String() != id {
				return closeFailure{websocket.CloseProtocolError, "invalid status ID"}
			}
			found := false
			for _, ref := range []*pb.AggregateRef{who.sessionRef, who.airportRef} {
				state, err := h.Projection.Read(ref)
				if err != nil {
					return err
				}
				outcome := state.Ledger[id]
				if outcome == nil || !proto.Equal(outcome.Actor, &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: who.me.Cid, SessionId: &who.id}) {
					continue
				}
				found = true
				if outcome.Status == pb.CommandOutcome_ACCEPTED {
					pending[id] = ref
				} else {
					delete(pending, id)
				}
				if err := send(conn, &pb.FrontendFrame_ActionResult{ActionResult: &pb.FrontendActionResult{RequestId: id, Status: outcome.Status, Aggregate: ref, AggregateRevision: outcome.AggregateRevision, ReasonCode: outcome.ReasonCode, Detail: outcome.Detail}}); err != nil {
					return err
				}
				break
			}
			if !found {
				if err := send(conn, &pb.FrontendFrame_StatusMissing{StatusMissing: &pb.ActionStatusMissing{RequestId: id}}); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return closeFailure{websocket.CloseProtocolError, "client frame not allowed"}
	}
}

func (h Handler) sendTerminalResults(conn *websocket.Conn, who identity, pending map[string]*pb.AggregateRef, ref *pb.AggregateRef) error {
	if len(pending) == 0 {
		return nil
	}
	state, err := h.Projection.Read(ref)
	if err != nil {
		return err
	}
	actor := &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: who.me.Cid, SessionId: &who.id}
	for id, aggregate := range pending {
		if !proto.Equal(aggregate, ref) {
			continue
		}
		outcome := state.Ledger[id]
		if outcome == nil || outcome.Status == pb.CommandOutcome_ACCEPTED || !proto.Equal(outcome.Actor, actor) {
			continue
		}
		result := &pb.FrontendActionResult{RequestId: id, Status: outcome.Status, Aggregate: ref,
			AggregateRevision: outcome.AggregateRevision, ReasonCode: outcome.ReasonCode, Detail: outcome.Detail}
		if err := send(conn, &pb.FrontendFrame_ActionResult{ActionResult: result}); err != nil {
			return err
		}
		delete(pending, id)
	}
	return nil
}

// Each active command family is a closed oneof. A generated but empty nested
// action is malformed even though proto3 can decode it successfully.
func requireOneofs(message protoreflect.Message) error {
	if !message.IsValid() {
		return fmt.Errorf("missing command action")
	}
	for i := 0; i < message.Descriptor().Oneofs().Len(); i++ {
		oneof := message.Descriptor().Oneofs().Get(i)
		if !oneof.IsSynthetic() && message.WhichOneof(oneof) == nil {
			return fmt.Errorf("missing %s case", oneof.Name())
		}
	}
	var result error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind || field.IsList() || field.IsMap() {
			return true
		}
		result = requireOneofs(value.Message())
		return result == nil
	})
	return result
}

func (h Handler) reject(conn *websocket.Conn, id string, ref *pb.AggregateRef, reason, detail string) error {
	return send(conn, &pb.FrontendFrame_ActionResult{ActionResult: &pb.FrontendActionResult{RequestId: id, Status: pb.CommandOutcome_FAILED, Aggregate: ref, ReasonCode: reason, Detail: detail}})
}
