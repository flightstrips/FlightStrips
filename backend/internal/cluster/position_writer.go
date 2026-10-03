package cluster

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PositionAuthority fences a queued write at execution time, after it has
// waited behind earlier reports for the same aircraft.
type PositionAuthority func(context.Context, int32, uint64, string) error

// PositionLifecycleFence verifies the complete ordered position projection
// against a freshly captured authoritative stream checkpoint. It is configured
// once before this owner/master writer is exposed to operational admission.
type PositionLifecycleFence func(context.Context, int32, uint64, string, []KVPosition) error

// ErrAircraftDisconnected marks an observation superseded by an accepted disconnect.
var ErrAircraftDisconnected = errors.New("aircraft already disconnected in owner epoch")

type PositionWriteResult struct {
	Revision uint64
	Err      error
}

// PositionWriter is constructed only by the candidate NATS runtime. A single
// writer accepts reports from one master connection in one owner epoch.
type PositionWriter struct {
	KV             nats.KeyValue
	SessionID      int32
	OwnerEpoch     uint64
	Connection     string
	Authority      PositionAuthority
	lifecycleFence PositionLifecycleFence
	committedApply func(context.Context, *pb.PositionValue, uint64) error
	async          *AsyncSessionOwners
	projection     *Projection
	nodeID         string
	baselineReady  bool
	dispatcher     *shared.PositionDispatcher
	mu             sync.Mutex
	revisionMu     sync.Mutex
	revisions      map[string]uint64
	barrierMu      sync.Mutex
	closed         bool // guarded by barrierMu
	disconnected   map[string]bool
}

func NewPositionWriter(kv nats.KeyValue, sessionID int32, epoch uint64, connection string, authority PositionAuthority, workers, pending int) (*PositionWriter, error) {
	if kv == nil || sessionID < 1 || epoch == 0 || connection == "" || authority == nil || workers < 1 || pending < 1 {
		return nil, fmt.Errorf("invalid position writer")
	}
	return &PositionWriter{KV: kv, SessionID: sessionID, OwnerEpoch: epoch, Connection: connection, Authority: authority,
		dispatcher: shared.NewPositionDispatcher(workers, pending, nil), disconnected: map[string]bool{}, revisions: map[string]uint64{}}, nil
}

func positionKey(sessionID int32, aircraft string, epoch uint64) string {
	return fmt.Sprintf("%d.%s.%d", sessionID, aircraft, epoch)
}

func canonicalAircraft(key string) bool {
	if key == "" || key != strings.ToUpper(strings.TrimSpace(key)) {
		return false
	}
	for _, r := range key {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// QueuePosition returns after bounded admission. Its result channel reports
// the KV revision or an error; cancelling admission does not cancel accepted
// work. A disconnect closes the aircraft to later reports in this epoch.
func (w *PositionWriter) QueuePosition(ctx context.Context, aircraft string, value *pb.AircraftPosition, observed time.Time) (<-chan PositionWriteResult, error) {
	if !canonicalAircraft(aircraft) || value == nil || !validAircraftPosition(value) || observed.IsZero() || timestamppb.New(observed).CheckValid() != nil {
		return nil, fmt.Errorf("invalid aircraft position")
	}
	return w.enqueue(ctx, aircraft, &pb.PositionValue{SchemaVersion: 1, SessionId: w.SessionID, AircraftKey: aircraft,
		OwnerEpoch: w.OwnerEpoch, SourceConnectionId: w.Connection, ObservedAt: timestamppb.New(observed.UTC()),
		Observation: &pb.PositionValue_Position{Position: proto.Clone(value).(*pb.AircraftPosition)}}, false)
}

// QueueDisconnect is ordered after every accepted report for the aircraft.
// Once accepted, a late report cannot recreate the aircraft in this epoch.
func (w *PositionWriter) QueueDisconnect(ctx context.Context, aircraft string, observed time.Time) (<-chan PositionWriteResult, error) {
	if !canonicalAircraft(aircraft) || observed.IsZero() || timestamppb.New(observed).CheckValid() != nil {
		return nil, fmt.Errorf("invalid aircraft disconnect")
	}
	return w.enqueue(ctx, aircraft, &pb.PositionValue{SchemaVersion: 1, SessionId: w.SessionID, AircraftKey: aircraft,
		OwnerEpoch: w.OwnerEpoch, SourceConnectionId: w.Connection, ObservedAt: timestamppb.New(observed.UTC()),
		Observation: &pb.PositionValue_Tombstone{Tombstone: &pb.PositionTombstone{}}}, true)
}

func (w *PositionWriter) enqueue(ctx context.Context, aircraft string, value *pb.PositionValue, disconnect bool) (<-chan PositionWriteResult, error) {
	if w == nil || w.dispatcher == nil || w.KV == nil || w.Authority == nil {
		return nil, fmt.Errorf("position writer unavailable")
	}
	stage := time.Now()
	w.mu.Lock()
	// The admission span belongs to the caller, not the dispatcher lifetime.
	admissionSpan := trace.SpanFromContext(ctx)
	admissionSpan.SetAttributes(attribute.Float64("position.owner_enqueue_lock_ms", float64(time.Since(stage))/float64(time.Millisecond)))
	defer w.mu.Unlock()
	if w.disconnected[aircraft] {
		return nil, ErrAircraftDisconnected
	}
	if w.async != nil {
		w.barrierMu.Lock()
		closed := w.closed
		w.barrierMu.Unlock()
		if closed {
			return nil, fmt.Errorf("position generation is closed")
		}
		return w.enqueueAsync(ctx, aircraft, value, disconnect)
	}
	result := make(chan PositionWriteResult, 1)
	key := positionKey(w.SessionID, aircraft, w.OwnerEpoch)
	queued := time.Now()
	err := w.dispatcher.Submit(ctx, key, func(runCtx context.Context) {
		// Preserve the dispatcher cancellation context for accepted work while
		// associating diagnostics with the original admission trace.
		runCtx = trace.ContextWithSpan(runCtx, admissionSpan)
		runCtx, span := otel.Tracer("cluster").Start(runCtx, "euroscope.position.owner_write")
		span.SetAttributes(attribute.Float64("position.owner_queue_to_worker_ms", float64(time.Since(queued))/float64(time.Millisecond)))
		defer span.End()
		revision, err := w.write(runCtx, key, value)
		result <- PositionWriteResult{Revision: revision, Err: err}
		close(result)
	})
	admissionSpan.SetAttributes(attribute.Float64("position.owner_submit_ms", float64(time.Since(queued))/float64(time.Millisecond)))
	if err != nil {
		return nil, err
	}
	if disconnect {
		w.disconnected[aircraft] = true
	}
	return result, nil
}

func (w *PositionWriter) write(ctx context.Context, key string, value *pb.PositionValue) (uint64, error) {
	stage := time.Now()
	authorityErr := w.Authority(ctx, w.SessionID, w.OwnerEpoch, w.Connection)
	trace.SpanFromContext(ctx).SetAttributes(attribute.Float64("position.owner_authority_ms", float64(time.Since(stage))/float64(time.Millisecond)))
	if authorityErr != nil {
		return 0, authorityErr
	}
	data, err := proto.Marshal(value)
	if err != nil {
		return 0, err
	}
	w.revisionMu.Lock()
	expected := w.revisions[key]
	w.revisionMu.Unlock()
	remember := func(revision uint64, err error) (uint64, error) {
		if err == nil {
			w.revisionMu.Lock()
			w.revisions[key] = revision
			w.revisionMu.Unlock()
		}
		return revision, err
	}
	publish := func(run func() (uint64, error)) (uint64, error) {
		_, span := otel.Tracer("cluster").Start(ctx, "nats.positions.puback")
		revision, err := run()
		if err != nil {
			span.SetStatus(codes.Error, "publish failed")
		}
		span.End()
		revision, err = remember(revision, err)
		if err == nil && w.committedApply != nil && w.async == nil {
			err = w.committedApply(ctx, value, revision)
		}
		return revision, err
	}
	if expected > 0 {
		// Per-aircraft FIFO makes the previous PubAck the exact CAS precondition.
		// A foreign publisher still causes a revision conflict; never refresh
		// and overwrite it. On a new writer the first value is read/validated.
		return publish(func() (uint64, error) { return w.KV.Update(key, data, expected) })
	}
	if w.baselineReady {
		return publish(func() (uint64, error) { return w.KV.Create(key, data) })
	}
	current, err := w.KV.Get(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return publish(func() (uint64, error) { return w.KV.Create(key, data) })
	}
	if err != nil {
		return 0, err
	}
	old := &pb.PositionValue{}
	if err := pb.UnmarshalStrict(current.Value(), old); err != nil {
		return 0, err
	}
	if old.SessionId != value.SessionId || old.AircraftKey != value.AircraftKey || old.OwnerEpoch != value.OwnerEpoch {
		return 0, fmt.Errorf("position epoch or identity conflict")
	}
	// A later master sync may observe an aircraft again in this same owner
	// epoch. Admission and the per-aircraft queue keep the old master's
	// accepted reports ahead of its tombstone; Authority fences its later work.
	// No retry on a revision conflict: another publisher may have violated the
	// single-owner rule, and overwriting its observation would hide that fault.
	return publish(func() (uint64, error) { return w.KV.Update(key, data, current.Revision()) })
}

func validAircraftPosition(v *pb.AircraftPosition) bool {
	if v.VerticalSpeedFpm != nil && (math.IsNaN(*v.VerticalSpeedFpm) || math.IsInf(*v.VerticalSpeedFpm, 0)) {
		return false
	}
	return !math.IsNaN(v.Latitude) && !math.IsInf(v.Latitude, 0) && math.Abs(v.Latitude) <= 90 &&
		!math.IsNaN(v.Longitude) && !math.IsInf(v.Longitude, 0) && math.Abs(v.Longitude) <= 180 &&
		!math.IsNaN(v.GroundSpeedKnots) && !math.IsInf(v.GroundSpeedKnots, 0) && v.GroundSpeedKnots >= 0 &&
		!math.IsNaN(v.TrackDegrees) && !math.IsInf(v.TrackDegrees, 0) && v.TrackDegrees >= 0 && v.TrackDegrees < 360
}

func (w *PositionWriter) Depth() int { return w.dispatcher.Depth() }
func (w *PositionWriter) Close(ctx context.Context) error {
	w.barrierMu.Lock()
	defer w.barrierMu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.dispatcher.Close(ctx)
}

// ExecuteLifecycle drains all admitted reports and fences every tagged current
// observation used for occupancy, including neighbours and disconnects.
func (w *PositionWriter) ExecuteLifecycle(ctx context.Context, observations []KVPosition, run func() (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	if run == nil {
		return nil, fmt.Errorf("lifecycle callback required")
	}
	return w.ExecuteLifecycleContext(ctx, observations, func(context.Context) (*pb.CommandReply, error) { return run() })
}
func (w *PositionWriter) ExecuteLifecycleContext(ctx context.Context, observations []KVPosition, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {

	if w == nil || w.dispatcher == nil || run == nil {
		return nil, fmt.Errorf("lifecycle position barrier unavailable")
	}
	if w.async != nil {
		return w.executeAsyncLifecycle(ctx, observations, run)
	}
	ctx, span := otel.Tracer("cluster").Start(ctx, "euroscope.position.lifecycle_barrier")
	defer span.End()
	var reply *pb.CommandReply
	var failure error
	err := w.runBarrier(ctx, func() {
		if failure = w.Authority(ctx, w.SessionID, w.OwnerEpoch, w.Connection); failure != nil {
			return
		}
		stage := time.Now()
		if w.lifecycleFence == nil {
			failure = fmt.Errorf("verified position projection fence is unavailable")
		} else {
			failure = w.lifecycleFence(ctx, w.SessionID, w.OwnerEpoch, w.Connection, observations)
		}
		span.SetAttributes(attribute.Float64("position.validation_ms", float64(time.Since(stage))/float64(time.Millisecond)))
		if failure != nil {
			return
		}
		stage = time.Now()
		reply, failure = run(ctx)
		span.SetAttributes(attribute.Float64("position.lifecycle_commit_ms", float64(time.Since(stage))/float64(time.Millisecond)))
	})
	if err != nil {
		return nil, err
	}
	return reply, failure
}

// SetLifecycleFence must complete before exposing a writer to admissions.
// Rebinding an active generation is rejected, and the barrier mutex prevents
// configuration from racing either lifecycle execution or generation closure.
func (w *PositionWriter) SetLifecycleFence(fence PositionLifecycleFence) error {
	if w == nil || fence == nil {
		return fmt.Errorf("verified position projection fence is required")
	}
	w.barrierMu.Lock()
	defer w.barrierMu.Unlock()
	if w.closed || w.lifecycleFence != nil {
		return fmt.Errorf("position lifecycle fence is already configured or writer is closed")
	}
	w.lifecycleFence = fence
	return nil
}

// SetCommittedApply binds the owner-local materialization before admission.
// The callback runs after durable CAS acknowledgment, before reporting success.
func (w *PositionWriter) SetCommittedApply(apply func(context.Context, *pb.PositionValue, uint64) error) error {
	if w == nil || apply == nil {
		return fmt.Errorf("committed position apply is required")
	}
	w.barrierMu.Lock()
	defer w.barrierMu.Unlock()
	if w.closed || w.committedApply != nil {
		return fmt.Errorf("position committed apply already configured or writer closed")
	}
	w.committedApply = apply
	return nil
}

// ExecuteDerived drains accepted position work and pauses later reports while
// the owner rechecks the source KV revision and commits a derived domain
// command. The command must target this writer's session and be submitted by
// the same session owner; the position itself never writes a strip version.
func (w *PositionWriter) ExecuteDerived(ctx context.Context, aircraft string, revision uint64, run func() (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	return w.ExecuteDerivedContext(ctx, aircraft, revision, func(context.Context) (*pb.CommandReply, error) { return run() })
}

// ExecuteDisconnect holds the same owner dispatcher barrier for a tombstone
// derived deletion/retention decision. A newer report invalidates its revision.
func (w *PositionWriter) ExecuteDisconnect(ctx context.Context, aircraft string, revision uint64, run func() (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	return w.ExecuteDisconnectContext(ctx, aircraft, revision, func(context.Context) (*pb.CommandReply, error) { return run() })
}

func (w *PositionWriter) ExecuteDerivedContext(ctx context.Context, aircraft string, revision uint64, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	return w.executeObservation(ctx, aircraft, revision, false, run)
}
func (w *PositionWriter) ExecuteDisconnectContext(ctx context.Context, aircraft string, revision uint64, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	return w.executeObservation(ctx, aircraft, revision, true, run)
}

func (w *PositionWriter) executeObservation(ctx context.Context, aircraft string, revision uint64, disconnected bool, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	if w == nil || w.dispatcher == nil || !canonicalAircraft(aircraft) || revision == 0 || run == nil {
		return nil, fmt.Errorf("invalid position-derived command")
	}
	if w.async != nil {
		return w.executeAsyncObservation(ctx, aircraft, revision, disconnected, run)
	}
	var reply *pb.CommandReply
	var commandErr error
	err := w.runBarrier(ctx, func() {
		if commandErr = w.Authority(ctx, w.SessionID, w.OwnerEpoch, w.Connection); commandErr != nil {
			return
		}
		key := positionKey(w.SessionID, aircraft, w.OwnerEpoch)
		var entry nats.KeyValueEntry
		entry, commandErr = w.KV.Get(key)
		if commandErr != nil {
			return
		}
		if entry.Revision() != revision {
			commandErr = fmt.Errorf("source position observation changed")
			return
		}
		value := &pb.PositionValue{}
		if commandErr = pb.UnmarshalStrict(entry.Value(), value); commandErr != nil {
			return
		}
		if value.SessionId != w.SessionID || value.AircraftKey != aircraft || value.OwnerEpoch != w.OwnerEpoch || value.SourceConnectionId != w.Connection || disconnected && value.GetTombstone() == nil || !disconnected && value.GetPosition() == nil {
			commandErr = fmt.Errorf("source position is stale or disconnected")
			return
		}
		reply, commandErr = run(ctx)
	})
	if err != nil {
		return nil, err
	}
	return reply, commandErr
}

// The shared dispatcher expects one operational reader. The candidate also
// admits worker expiry, so serialize its barriers and keep reopening admission
// outside a paused queue to prevent a full queue/mutex deadlock.
func (w *PositionWriter) runBarrier(ctx context.Context, run func()) error {
	w.barrierMu.Lock()
	defer w.barrierMu.Unlock()
	if w.closed {
		return fmt.Errorf("position generation is closed")
	}
	return w.dispatcher.RunBarrier(ctx, run)
}

// ReopenAircraft is used only for a newer authenticated strip/sync observation
// in the current generation, after prior reports and the disconnect drain.
func (w *PositionWriter) ReopenAircraft(ctx context.Context, aircraft string) error {
	if !canonicalAircraft(aircraft) {
		return fmt.Errorf("invalid reappearing aircraft")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.runBarrier(ctx, func() { delete(w.disconnected, aircraft) })
}

// PositionAuthority rejects work left in the queue after owner or master
// handoff. A fresh synced master is required even when old KV values survive.
func (p *Projection) PositionAuthority(nodeID string) PositionAuthority {
	return func(_ context.Context, sessionID int32, epoch uint64, connection string) error {
		ref := sessionRef(sessionID)
		sync, err := p.OperationalSync(ref)
		if err != nil {
			return err
		}
		if sync == nil || sync.ConnectionId != connection {
			return fmt.Errorf("master sync is unavailable")
		}
		subject, _ := Subject(ref)
		p.mu.RLock()
		defer p.mu.RUnlock()
		state := p.states[subject]
		if state == nil || state.Owner == nil || state.Master == nil || state.Owner.NodeId != nodeID ||
			state.Owner.Epoch != epoch || state.Master.OwnerEpoch != epoch || state.Master.ConnectionId != connection ||
			state.Owner.LeaseUntil == nil || !time.Now().Before(state.Owner.LeaseUntil.AsTime()) || p.NC.Status() != nats.CONNECTED {
			return fmt.Errorf("position owner epoch is no longer current")
		}
		return nil
	}
}
