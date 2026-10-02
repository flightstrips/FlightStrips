package euroscopebinary

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/faultgate"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const Subprotocol = "flightstrips.euroscope.pb.v2"
const maxFrame = 4 << 20
const positionAwaitWorkers = 128

// Handler is the opt-in NATS EuroScope endpoint. The current SQL hub does not
// construct it. Domain adapters supplied at cutover consume validated frames.
type Handler struct {
	Projection *cluster.Projection
	Fanout     *cluster.SessionFanout
	Sessions   interface {
		GetOrCreateSession(context.Context, string, string) (*pb.Session, error)
	}
	Auth         shared.AuthenticationService
	Sync         cluster.SessionObservations
	Controllers  cluster.ControllerSector
	Inbound      func(context.Context, int32, string, string, *euroscope.Envelope) error
	RenderDelta  func(*pb.FrontendDelta) []*euroscope.Envelope
	RenderEffect func(int32, *pb.EffectRecord) (*euroscope.Envelope, error)
	Effects      *cluster.Effects
	OnReconciled func(context.Context, int32, []string) error
	// Task 18c supplies this concrete candidate. The current SQL endpoint does
	// not construct it; Task 20 binds it together with Inbound/Planner/worker.
	Deadlines *DeadlineCandidate
}

var upgrade = websocket.Upgrader{Subprotocols: []string{Subprotocol}, EnableCompression: false,
	CheckOrigin: func(*http.Request) bool { return true }}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	if h.Projection == nil || h.Fanout == nil || h.Sessions == nil || h.Auth == nil || h.Inbound == nil ||
		h.Sync.Store == nil || h.Controllers.Store == nil ||
		h.Fanout.Projection != h.Projection || h.Fanout.NodeID == "" {
		http.Error(w, "EuroScope runtime unavailable", http.StatusServiceUnavailable)
		return
	}
	if !hasProtocol(websocket.Subprotocols(r), Subprotocol) {
		http.Error(w, "unsupported EuroScope subprotocol", http.StatusForbidden)
		return
	}
	conn, err := upgrade.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	stop := context.AfterFunc(r.Context(), func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	conn.SetReadLimit(maxFrame)
	if err := h.serve(r.Context(), conn); err != nil {
		if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			slog.WarnContext(r.Context(), "EuroScope session failed", "error_type", fmt.Sprintf("%T", err), "error_reason", inboundFailureReason(err))
		}
		code := websocket.CloseTryAgainLater
		if failure, ok := err.(socketFailure); ok {
			code = failure.code
		}
		_ = conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(code, "EuroScope session unavailable"), time.Now().Add(time.Second))
	}
}

func hasProtocol(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type socketFailure struct{ code int }

func (f socketFailure) Error() string { return "invalid EuroScope socket frame" }

func read(conn *websocket.Conn) (*euroscope.Envelope, error) {
	kind, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	if kind != websocket.BinaryMessage {
		return nil, socketFailure{websocket.CloseUnsupportedData}
	}
	if len(data) == 0 || len(data) > maxFrame {
		return nil, socketFailure{websocket.CloseProtocolError}
	}
	frame := &euroscope.Envelope{}
	if err := pb.UnmarshalStrict(data, frame); err != nil || frame.GetEvent() == nil {
		return nil, socketFailure{websocket.CloseProtocolError}
	}
	return frame, nil
}

type socketWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *socketWriter) send(frame *euroscope.Envelope) error {
	if frame == nil || frame.GetEvent() == nil {
		return fmt.Errorf("empty EuroScope frame")
	}
	data, err := proto.Marshal(frame)
	if err != nil || len(data) > maxFrame {
		return fmt.Errorf("invalid or oversized EuroScope frame")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	faultgate.Reach("before-socket-write", frame.CommandId, 0)
	err = w.conn.WriteMessage(websocket.BinaryMessage, data)
	if err == nil {
		faultgate.Reach("after-socket-write", frame.CommandId, 0)
	}
	return err
}

func (h Handler) serve(ctx context.Context, conn *websocket.Conn) error {
	token, err := read(conn)
	if err != nil {
		return err
	}
	if token.GetToken() == nil || token.GetToken().ProtocolRevision != 2 || token.GetToken().Token == "" {
		return socketFailure{websocket.ClosePolicyViolation}
	}
	user, err := h.Auth.Validate(token.GetToken().Token)
	if err != nil || !user.IsValid() || user.GetCid() == "" {
		return socketFailure{websocket.ClosePolicyViolation}
	}
	login, err := read(conn)
	if err != nil {
		return err
	}
	_ = conn.SetReadDeadline(time.Time{})
	identity := login.GetLogin()
	if identity == nil || identity.Airport == "" || identity.Callsign == "" {
		return socketFailure{websocket.ClosePolicyViolation}
	}
	airport := strings.ToUpper(strings.TrimSpace(identity.Airport))
	name := strings.ToUpper(strings.TrimSpace(identity.Connection))
	if name == "" {
		name = "LIVE"
	}
	session, err := h.Sessions.GetOrCreateSession(ctx, airport, name)
	if err != nil {
		return fmt.Errorf("open EuroScope session: %w", err)
	}
	if session == nil || session.Tombstoned {
		return socketFailure{websocket.ClosePolicyViolation}
	}
	if err := h.putController(ctx, session.Id, user.GetCid(), identity); err != nil {
		return err
	}
	lease, err := cluster.NewSocketPresenceLease(h.Projection.Presence, h.Fanout.NodeID, session.Id,
		user.GetCid(), strings.ToUpper(strings.TrimSpace(identity.Callsign)),
		identity.Position, identity.Observer, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		return socketFailure{websocket.ClosePolicyViolation}
	}
	writer := &socketWriter{conn: conn}
	callbacks := cluster.LocalSessionSocket{
		OnInitial: func(state, _ *cluster.Aggregate) error {
			return writer.send(backendSync(state))
		},
		OnDelta: func(delta *pb.FrontendDelta) error {
			if h.RenderDelta != nil {
				for _, frame := range h.RenderDelta(delta) {
					if err := writer.send(frame); err != nil {
						return err
					}
				}
				return nil
			}
			frames := stripDeltas(delta)
			if delta.Aggregate.GetSession() != nil {
				for _, change := range delta.Changes {
					if change.GetUpsert().GetStrip() != nil || change.GetUpsert().GetCdmState() != nil || change.GetUpsert().GetEcfmpState() != nil {
						state, err := h.Projection.ReadEntityKinds(delta.Aggregate, pb.EntityKind_STRIP, pb.EntityKind_CDM_STATE, pb.EntityKind_ECFMP_STATE)
						if err != nil {
							return err
						}
						frames = []*euroscope.Envelope{backendSync(state)}
						break
					}
				}
			}
			for _, frame := range frames {
				if err := writer.send(frame); err != nil {
					return err
				}
			}
			return nil
		},
		OnRole: func(role string, masterEpoch, ownerEpoch uint64) error {
			return writer.send(&euroscope.Envelope{SessionId: session.Id, OwnerEpoch: ownerEpoch,
				MasterEpoch: masterEpoch, Event: &euroscope.Envelope_SessionInfo{SessionInfo: &euroscope.SessionInfoEvent{
					Role: role, MasterEpoch: masterEpoch, OwnerEpoch: ownerEpoch}}})
		},
		OnEffect: func(effect *pb.EffectRecord) error {
			if h.RenderEffect == nil {
				return fmt.Errorf("effect renderer unavailable")
			}
			frame, err := h.RenderEffect(session.Id, effect)
			if err != nil {
				return err
			}
			if frame == nil || frame.CommandId != effect.CommandId || frame.SessionId != session.Id ||
				frame.OwnerEpoch != effect.OwnerEpoch || frame.MasterEpoch != effect.MasterEpoch {
				return fmt.Errorf("effect envelope terms do not match dispatch claim")
			}
			return writer.send(frame)
		},
		Close: func() { _ = conn.Close() },
	}
	closeSocket, err := h.Fanout.Attach(ctx, lease, callbacks)
	if err != nil {
		return err
	}
	defer closeSocket()
	if h.Deadlines != nil {
		// Liveness is already shared. Recovery is advisory here; the owner
		// worker repairs it even if login races an election or this node dies.
		_ = h.Deadlines.Recover(ctx, session.Id)
		defer func() {
			closeSocket()
			// Advisory fast recovery only. SessionWork's shared-state sweep is
			// authoritative if the socket node dies or this call is unavailable.
			recoverCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = h.Deadlines.Recover(recoverCtx, session.Id)
		}()
	}
	// Report callbacks hold a slot through PubAck and projection application.
	// A bounded pool overlaps independent aircraft's I/O waits; the owner
	// position writer keeps its separate 32-worker publication budget.
	positions := shared.NewPositionDispatcher(positionAwaitWorkers, 1024, nil)
	defer func() {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = positions.Close(drain)
	}()
	var failureMu sync.Mutex
	var positionFailure error
	for {
		frame, err := read(conn)
		if err != nil {
			failureMu.Lock()
			failure := positionFailure
			failureMu.Unlock()
			if failure != nil {
				return failure
			}
			return err
		}
		received := time.Now()
		independent := independentInboundKey(frame)
		if independent == "" {
			if err := positions.Barrier(ctx); err != nil {
				return err
			}
			failureMu.Lock()
			err = positionFailure
			failureMu.Unlock()
			if err != nil {
				return err
			}
		}
		var admissionSpan trace.Span
		if frame.GetAircraftPositionUpdate() != nil {
			_, admissionSpan = otel.Tracer("euroscopebinary").Start(ctx, "euroscope.position.admission", trace.WithTimestamp(received))
		}
		validationStarted := time.Now()
		validationErr := h.Projection.ValidateEuroScopeInbound(session.Id, lease.Client.ConnectionId, user.GetCid(), frame)
		if admissionSpan != nil {
			admissionSpan.SetAttributes(attribute.Float64("position.reader_validation_ms", float64(time.Since(validationStarted))/float64(time.Millisecond)))
			admissionSpan.End()
		}
		if err := validationErr; err != nil {
			slog.WarnContext(ctx, "candidate EuroScope inbound rejected", "session", session.Id, "error", err)
			return socketFailure{websocket.ClosePolicyViolation}
		}
		if frame.GetCommandResult() != nil {
			if h.Effects == nil || h.Effects.RecordResult(ctx, session.Id, lease.Client.ConnectionId, user.GetCid(), frame) != nil {
				return socketFailure{websocket.ClosePolicyViolation}
			}
			if err := writer.send(&euroscope.Envelope{CommandId: frame.CommandId, SessionId: session.Id,
				OwnerEpoch: frame.OwnerEpoch, MasterEpoch: frame.MasterEpoch,
				Event: &euroscope.Envelope_ResultRecorded{ResultRecorded: &euroscope.ResultRecordedEvent{CommandId: frame.CommandId}}}); err != nil {
				return err
			}
			continue
		}
		if independent != "" {
			if err := positions.Submit(ctx, independent, func(run context.Context) {
				if err := h.tracedInbound(run, received, session.Id, lease.Client.ConnectionId, user.GetCid(), frame); err != nil {
					failureMu.Lock()
					if positionFailure == nil {
						positionFailure = err
					}
					failureMu.Unlock()
					positions.Cancel()
					_ = conn.Close()
				}
			}); err != nil {
				return err
			}
			continue
		}
		err = h.tracedInbound(ctx, received, session.Id, lease.Client.ConnectionId, user.GetCid(), frame)
		if err != nil {
			return err
		}
		if frame.GetSync() != nil {
			if h.OnReconciled != nil {
				state, err := h.Projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: session.Id}}})
				if err != nil {
					return err
				}
				if err := h.OnReconciled(ctx, session.Id, cluster.ReconciledUnknownEffects(state, frame.GetSync())); err != nil {
					return err
				}
			}
			_, err := h.Sync.RecordSync(ctx, session.Id, uuid.NewString(), &pb.SessionSync{
				ConnectionId: lease.Client.ConnectionId, MasterEpoch: frame.MasterEpoch,
				CompletedAt: timestamppb.Now()})
			if err != nil {
				return err
			}
			if h.Deadlines != nil {
				_ = h.Deadlines.Recover(ctx, session.Id)
			}
		}
	}
}

// Positions overlap across aircraft. Heading changes never derive position
// state and share one serial commit lane to avoid competing subject CAS writes.
// Every other operational frame drains both lanes before admission.
func independentInboundKey(frame *euroscope.Envelope) string {
	if v := frame.GetAircraftPositionUpdate(); v != nil {
		return v.Callsign
	}
	if frame.GetHeading() != nil {
		return "$heading"
	}
	return ""
}

func (h Handler) tracedInbound(ctx context.Context, received time.Time, session int32, connection, cid string, frame *euroscope.Envelope) error {
	processingStarted := time.Now()
	ctx, span := otel.Tracer("euroscopebinary").Start(ctx, "euroscope.receipt_to_completion", trace.WithTimestamp(received))
	span.SetAttributes(attribute.String("command_id", frame.CommandId), attribute.Bool("position", frame.GetAircraftPositionUpdate() != nil), attribute.Float64("dispatch_wait_ms", float64(processingStarted.Sub(received))/float64(time.Millisecond)))
	err := h.Inbound(ctx, session, connection, cid, frame)
	span.SetAttributes(attribute.Float64("processing_ms", float64(time.Since(processingStarted))/float64(time.Millisecond)))
	if err != nil {
		span.SetStatus(codes.Error, inboundFailureReason(err))
		span.SetAttributes(attribute.String("error_type", fmt.Sprintf("%T", err)), attribute.String("error_reason", inboundFailureReason(err)))
	}
	span.End()
	return err
}

func (h Handler) putController(ctx context.Context, sessionID int32, cid string, login *euroscope.LoginEvent) error {
	for attempt := 0; attempt < 100; attempt++ {
		controllers, err := h.Controllers.Controllers(ctx, sessionID)
		if err != nil {
			return err
		}
		record := &pb.Controller{Cid: cid, Callsign: strings.ToUpper(strings.TrimSpace(login.Callsign)),
			Position: login.Position, Observer: login.Observer}
		var revision uint64
		for _, previous := range controllers {
			if previous.Cid == cid {
				revision = previous.Revision
				record = proto.Clone(previous).(*pb.Controller)
				record.Callsign = strings.ToUpper(strings.TrimSpace(login.Callsign))
				record.Position = login.Position
				record.Observer = login.Observer
				break
			}
		}
		if reply, err := h.Controllers.PutController(ctx, sessionID, record, revision); err == nil &&
			reply != nil && reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
			return nil
		} else if reply == nil || (reply.Status != pb.CommandReply_REVISION_CONFLICT && reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" && reply.Status != pb.CommandReply_UNAVAILABLE && reply.Status != pb.CommandReply_NOT_OWNER) {
			return fmt.Errorf("controller identity could not be committed: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	return fmt.Errorf("controller identity changed during login")
}

func backendSync(state *cluster.Aggregate) *euroscope.Envelope {
	lat, lon := config.GetAirportCoordinates()
	sync := &euroscope.BackendSyncEvent{Latitude: lat, Longitude: lon}
	for _, entity := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		if strip := entity.GetValue().GetStrip(); operationalStrip(strip) {
			value := syncStrip(strip)
			value.Cdm = syncCdm(strip, state.Indexes[pb.EntityKind_CDM_STATE][strip.Callsign].GetValue().GetCdmState(), state.Indexes[pb.EntityKind_ECFMP_STATE][strip.Callsign].GetValue().GetEcfmpState())
			sync.Strips = append(sync.Strips, value)
		}
	}
	return &euroscope.Envelope{Event: &euroscope.Envelope_BackendSync{BackendSync: sync}}
}

func syncStrip(strip *pb.Strip) *euroscope.BackendSyncStrip {
	return &euroscope.BackendSyncStrip{Callsign: strip.Callsign, AssignedSquawk: strip.AssignedSquawk,
		Cleared: strip.Bay != "NOT_CLEARED" && strip.Bay != "UNKNOWN", GroundState: shared.GetGroundState(strip.Bay),
		Stand: strip.Stand, PdcState: strip.PdcState, PdcRequestRemarks: strip.PdcRequestRemarks, Hold: strip.Hold, HoldType: strip.HoldType, HoldEat: strip.HoldEat}
}

func syncCdm(s *pb.Strip, c *pb.CdmState, flow *pb.EcfmpState) *euroscope.BackendSyncCdmData {
	clock := func(v *timestamppb.Timestamp) string {
		if v == nil {
			return ""
		}
		return v.AsTime().UTC().Format("1504")
	}
	value := &euroscope.BackendSyncCdmData{Eobt: clock(s.Eobt), Tobt: clock(s.Tobt), TobtSetBy: s.GetTobtSetBy(), TobtConfirmedBy: c.GetTobtConfirmedBy(), Tsat: clock(s.Tsat), Ttot: clock(s.Ttot), Ctot: clock(s.Ctot), CtotSource: s.GetCtotSource(), Asat: clock(s.Asat), Asrt: clock(s.Asrt), Tsac: clock(s.Tsac), Status: s.GetOperationalStatus(), DeiceType: c.GetDeice(), EcfmpId: s.GetEcfmpId(), Phase: s.GetPhase()}
	for _, v := range flow.GetRestrictions() {
		levels := make([]int32, len(v.ExactLevels))
		for i, n := range v.ExactLevels {
			levels[i] = n
		}
		value.EcfmpRestrictions = append(value.EcfmpRestrictions, &euroscope.EcfmpRestriction{MeasureId: int64(v.MeasureId), Ident: v.Ident, Type: v.Kind, Reason: v.Reason, Routes: v.Routes, Destination: v.Destination, MaxLevel: v.MaxLevel, MinLevel: v.MinLevel, ExactLevels: levels, HasCtot: v.HasCtot})
	}
	return value
}

func operationalStrip(strip *pb.Strip) bool {
	return strip != nil && strip.HasFlightPlan && strip.Bay != shared.BAY_DEP_HIDDEN &&
		strip.Bay != shared.BAY_ARR_HIDDEN
}

func stripDeltas(delta *pb.FrontendDelta) []*euroscope.Envelope {
	if delta == nil {
		return nil
	}
	var frames []*euroscope.Envelope
	lat, lon := config.GetAirportCoordinates()
	for _, change := range delta.Changes {
		if strip := change.GetUpsert().GetStrip(); operationalStrip(strip) {
			frames = append(frames, &euroscope.Envelope{Event: &euroscope.Envelope_BackendSync{
				BackendSync: &euroscope.BackendSyncEvent{Latitude: lat, Longitude: lon,
					Strips: []*euroscope.BackendSyncStrip{syncStrip(strip)}}}})
		}
	}
	return frames
}

// Only allowlisted classifications enter logs/telemetry; error strings may
// contain callsigns, provider data, or authentication input.
func inboundFailureReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "context_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "context_deadline"
	}
	var api *nats.APIError
	if errors.As(err, &api) {
		return fmt.Sprintf("nats_api_%d", api.ErrorCode)
	}
	if errors.Is(err, cluster.ErrCAS) {
		return "revision_conflict"
	}
	if err == nil {
		return "none"
	}
	text := err.Error()
	for _, reason := range []struct{ match, name string }{
		{"REVISION_CONFLICT", "revision_conflict"},
		{"stale strip revision", "revision_conflict"},
		{"snapshot", "snapshot_unavailable"},
		{"projection", "projection_unavailable"},
		{"master", "master_authority"},
		{"owner", "owner_authority"},
		{"socket", "socket_authority"},
	} {
		if strings.Contains(text, reason.match) {
			return reason.name
		}
	}
	return "inbound_failure"
}
