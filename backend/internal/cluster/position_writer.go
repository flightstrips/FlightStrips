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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PositionAuthority fences a queued write at execution time, after it has
// waited behind earlier reports for the same aircraft.
type PositionAuthority func(context.Context, int32, uint64, string) error

type PositionWriteResult struct {
	Revision uint64
	Err      error
}

// PositionWriter is constructed only by the candidate NATS runtime. A single
// writer accepts reports from one master connection in one owner epoch.
type PositionWriter struct {
	KV           nats.KeyValue
	SessionID    int32
	OwnerEpoch   uint64
	Connection   string
	Authority    PositionAuthority
	dispatcher   *shared.PositionDispatcher
	mu           sync.Mutex
	disconnected map[string]bool
}

func NewPositionWriter(kv nats.KeyValue, sessionID int32, epoch uint64, connection string, authority PositionAuthority, workers, pending int) (*PositionWriter, error) {
	if kv == nil || sessionID < 1 || epoch == 0 || connection == "" || authority == nil || workers < 1 || pending < 1 {
		return nil, fmt.Errorf("invalid position writer")
	}
	return &PositionWriter{KV: kv, SessionID: sessionID, OwnerEpoch: epoch, Connection: connection, Authority: authority,
		dispatcher: shared.NewPositionDispatcher(workers, pending, nil), disconnected: map[string]bool{}}, nil
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
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.disconnected[aircraft] {
		return nil, fmt.Errorf("aircraft already disconnected in owner epoch")
	}
	result := make(chan PositionWriteResult, 1)
	key := positionKey(w.SessionID, aircraft, w.OwnerEpoch)
	err := w.dispatcher.Submit(ctx, key, func(runCtx context.Context) {
		revision, err := w.write(runCtx, key, value)
		result <- PositionWriteResult{Revision: revision, Err: err}
		close(result)
	})
	if err != nil {
		return nil, err
	}
	if disconnect {
		w.disconnected[aircraft] = true
	}
	return result, nil
}

func (w *PositionWriter) write(ctx context.Context, key string, value *pb.PositionValue) (uint64, error) {
	if err := w.Authority(ctx, w.SessionID, w.OwnerEpoch, w.Connection); err != nil {
		return 0, err
	}
	data, err := proto.Marshal(value)
	if err != nil {
		return 0, err
	}
	current, err := w.KV.Get(key)
	if errors.Is(err, nats.ErrKeyNotFound) {
		return w.KV.Create(key, data)
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
	return w.KV.Update(key, data, current.Revision())
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

func (w *PositionWriter) Depth() int                      { return w.dispatcher.Depth() }
func (w *PositionWriter) Close(ctx context.Context) error { return w.dispatcher.Close(ctx) }

// ExecuteDerived drains accepted position work and pauses later reports while
// the owner rechecks the source KV revision and commits a derived domain
// command. The command must target this writer's session and be submitted by
// the same session owner; the position itself never writes a strip version.
func (w *PositionWriter) ExecuteDerived(ctx context.Context, aircraft string, revision uint64, run func() (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	if w == nil || w.dispatcher == nil || !canonicalAircraft(aircraft) || revision == 0 || run == nil {
		return nil, fmt.Errorf("invalid position-derived command")
	}
	var reply *pb.CommandReply
	var commandErr error
	err := w.dispatcher.RunBarrier(ctx, func() {
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
		if value.SessionId != w.SessionID || value.AircraftKey != aircraft || value.OwnerEpoch != w.OwnerEpoch || value.SourceConnectionId != w.Connection || value.GetPosition() == nil {
			commandErr = fmt.Errorf("source position is stale or disconnected")
			return
		}
		reply, commandErr = run()
	})
	if err != nil {
		return nil, err
	}
	return reply, commandErr
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
