package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"sync/atomic"
	"time"
)

const localPositionBit uint64 = 1 << 63

var ErrPositionIntegrityStale = errors.New("position integrity proof is stale")

var localPositionCounter atomic.Uint64

func init() {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	localPositionCounter.Store(binary.LittleEndian.Uint64(seed[:]) & ((1 << 62) - 1))
}

type asyncPositionState struct {
	values    map[string]KVPosition
	durable   map[string]uint64
	tokens    map[uint64]uint64
	pending   map[uint64]*pb.PositionValue
	baselines map[string]bool
}

// Caller holds p.mu. This merged view is never the raw ordered consumer proof.
func (p *Projection) positionViewLocked() map[string]KVPosition {
	if p.asyncPositions == nil || len(p.asyncPositions.values) == 0 {
		return p.positions
	}
	view := make(map[string]KVPosition, len(p.positions)+len(p.asyncPositions.values))
	for key, item := range p.positions {
		view[key] = item
	}
	for key, item := range p.asyncPositions.values {
		view[key] = item
	}
	return view
}

// SetAsyncOwners proves the durable baseline once before exposing this generation.
func (w *PositionWriter) SetAsyncOwners(ctx context.Context, owners *AsyncSessionOwners, p *Projection, nodeID string) error {
	if w == nil || owners == nil || p == nil || nodeID == "" {
		return fmt.Errorf("async position owner unavailable")
	}
	baselineKey := fmt.Sprintf("%d.%d", w.SessionID, w.OwnerEpoch)
	p.mu.RLock()
	established := p.asyncPositions != nil && p.asyncPositions.baselines[baselineKey]
	p.mu.RUnlock()
	var err error
	if !established {
		caught, proofErr := p.provePositionCursor(ctx)
		if proofErr != nil {
			return proofErr
		}
		if !caught {
			return fmt.Errorf("position baseline replay incomplete")
		}
	}
	if err = w.Authority(ctx, w.SessionID, w.OwnerEpoch, w.Connection); err != nil {
		return err
	}
	w.barrierMu.Lock()
	defer w.barrierMu.Unlock()
	if w.closed || w.async != nil {
		return fmt.Errorf("position async owner already configured or closed")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err = p.healthLocked(); err != nil {
		return err
	}
	if !established && (p.positionCursor.provedName != p.positionCursor.consumer || p.positionCursor.provedConsumer != p.positionCursor.appliedConsumer) {
		return fmt.Errorf("position baseline generation changed")
	}
	if p.asyncPositions == nil {
		p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{}, durable: map[string]uint64{}, tokens: map[uint64]uint64{}, pending: map[uint64]*pb.PositionValue{}, baselines: map[string]bool{}}
	}
	w.revisionMu.Lock()
	for key, item := range p.positions {
		if item.Value.SessionId == w.SessionID && item.Value.OwnerEpoch == w.OwnerEpoch {
			w.revisions[key] = item.Revision
			if p.asyncPositions.durable[key] < item.Revision {
				p.asyncPositions.durable[key] = item.Revision
			}
		}
	}
	w.revisionMu.Unlock()
	p.asyncPositions.baselines[baselineKey] = true
	w.async, w.projection, w.nodeID, w.baselineReady = owners, p, nodeID, true
	return nil
}

func (w *PositionWriter) enqueueAsync(ctx context.Context, aircraft string, value *pb.PositionValue, disconnect bool) (<-chan PositionWriteResult, error) {
	result := make(chan PositionWriteResult, 1)
	detached := proto.Clone(value).(*pb.PositionValue)
	key := positionKey(w.SessionID, aircraft, w.OwnerEpoch)
	var token uint64
	ref := sessionRef(w.SessionID)
	err := w.async.Append(ctx, ref, func(runCtx context.Context) error {
		if err := w.Authority(runCtx, w.SessionID, w.OwnerEpoch, w.Connection); err != nil {
			return err
		}
		p := w.projection
		p.mu.Lock()
		defer p.mu.Unlock()
		if err := p.asyncPositionHealthLocked(); err != nil {
			return err
		}
		token = localPositionBit | localPositionCounter.Add(1)
		item := KVPosition{Value: detached, Revision: token, Observed: value.ObservedAt.AsTime()}
		p.asyncPositions.values[key] = item
		p.asyncPositions.pending[token] = detached
		p.publishObservationLocked(w.SessionID, positionObservation(item, false, false))
		p.wakePositionWaitersLocked(key)
		return nil
	}, func(runCtx context.Context) error {
		revision, err := w.writeAsyncDurable(runCtx, key, detached)
		if err != nil {
			return err
		}
		p := w.projection
		p.mu.Lock()
		if p.asyncPositions == nil {
			p.mu.Unlock()
			return fmt.Errorf("position RAM generation was reset")
		}
		if current := p.positions[key]; current.Revision > revision {
			p.mu.Unlock()
			return fmt.Errorf("position was superseded by foreign publisher")
		}
		p.asyncPositions.tokens[token] = revision
		p.asyncPositions.durable[key] = revision
		delete(p.asyncPositions.pending, token)
		p.mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	if disconnect {
		w.disconnected[aircraft] = true
	}
	result <- PositionWriteResult{Revision: token}
	close(result)
	return result, nil
}

// TranslatePositionSources changes only position-backed deadline source tags on
// a detached persistent event; unrelated provider/entity revisions are preserved.
func (p *Projection) TranslatePositionSources(event *pb.StateEvent) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.asyncPositions == nil {
		return nil
	}
	return translatePositionSources(event.ProtoReflect(), p.asyncPositions.tokens)
}

func (w *PositionWriter) executeAsyncObservation(ctx context.Context, aircraft string, token uint64, disconnected bool, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	var reply *pb.CommandReply
	err := w.async.Execute(ctx, sessionRef(w.SessionID), func(runCtx context.Context) error {
		if err := w.Authority(runCtx, w.SessionID, w.OwnerEpoch, w.Connection); err != nil {
			return err
		}
		p := w.projection
		p.mu.RLock()
		item, ok := p.positionAtLocked(positionKey(w.SessionID, aircraft, w.OwnerEpoch))
		err := p.asyncPositionHealthLocked()
		if err == nil && (!ok || !p.positionSourceMatchesLocked(item.Revision, token) || item.Value.SourceConnectionId != w.Connection || (disconnected && item.Value.GetTombstone() == nil) || (!disconnected && item.Value.GetPosition() == nil)) {
			err = fmt.Errorf("RAM source position changed or disconnected")
		}
		p.mu.RUnlock()
		if err != nil {
			return err
		}
		reply, err = run(runCtx)
		return err
	})
	return reply, err
}

func (w *PositionWriter) executeAsyncLifecycle(ctx context.Context, observations []KVPosition, run func(context.Context) (*pb.CommandReply, error)) (*pb.CommandReply, error) {
	var reply *pb.CommandReply
	err := w.async.Execute(ctx, sessionRef(w.SessionID), func(runCtx context.Context) error {
		if err := w.Authority(runCtx, w.SessionID, w.OwnerEpoch, w.Connection); err != nil {
			return err
		}
		p := w.projection
		// Ordered watcher proves retention/generation health in the background.
		// A local assignment compares the complete RAM tail under this turn.
		p.mu.RLock()
		err := p.asyncPositionHealthLocked()
		if err == nil {
			err = p.compareLifecyclePositionsLocked(w.SessionID, w.OwnerEpoch, w.Connection, observations)
		}
		p.mu.RUnlock()
		if err != nil {
			return err
		}
		reply, err = run(runCtx)
		return err
	})
	return reply, err
}

func translatePositionSources(message protoreflect.Message, tokens map[uint64]uint64) error {
	if deadline, ok := message.Interface().(*pb.SessionDeadline); ok && deadline.Kind == "aircraft-disconnect" && deadline.SourceRevision&localPositionBit != 0 {
		revision, exists := tokens[deadline.SourceRevision]
		if !exists || revision == 0 {
			return fmt.Errorf("position source has not been synchronized")
		}
		deadline.SourceRevision = revision
	}
	var result error
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind {
			value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
				result = translatePositionSources(v.Message(), tokens)
				return result == nil
			})
		} else if field.IsList() && field.Kind() == protoreflect.MessageKind {
			for i := 0; i < value.List().Len() && result == nil; i++ {
				result = translatePositionSources(value.List().Get(i).Message(), tokens)
			}
		} else if field.Kind() == protoreflect.MessageKind {
			result = translatePositionSources(value.Message(), tokens)
		}
		return result == nil
	})
	return result
}

func (w *PositionWriter) writeAsyncDurable(ctx context.Context, key string, value *pb.PositionValue) (uint64, error) {
	if value == nil || value.SchemaVersion != 1 || value.SessionId != w.SessionID || value.OwnerEpoch != w.OwnerEpoch || value.SourceConnectionId != w.Connection || key != positionKey(w.SessionID, value.AircraftKey, w.OwnerEpoch) {
		return 0, fmt.Errorf("invalid async position identity")
	}
	data, err := proto.Marshal(value)
	if err != nil {
		return 0, err
	}
	w.projection.mu.RLock()
	if w.projection.asyncPositions == nil {
		w.projection.mu.RUnlock()
		return 0, fmt.Errorf("position RAM generation was reset")
	}
	expected := w.projection.asyncPositions.durable[key]
	w.projection.mu.RUnlock()
	var revision uint64
	ambiguous := false
	for {
		if err = ctx.Err(); err != nil {
			return 0, err
		}
		if ambiguous {
			// Read only after an unknown acknowledgement. An unchanged baseline
			// permits the same CAS again; a different advanced value never does.
			entry, getErr := w.KV.Get(key)
			if getErr == nil {
				if entry == nil || entry.Revision() < expected {
					return 0, fmt.Errorf("position durable revision regressed")
				}
				if entry.Revision() > expected {
					if !bytes.Equal(entry.Value(), data) {
						return 0, fmt.Errorf("position ambiguous publication was superseded")
					}
					revision = entry.Revision()
					break
				}
			} else if errors.Is(getErr, nats.ErrKeyNotFound) {
				if expected != 0 {
					return 0, fmt.Errorf("position ambiguous publication was deleted: %w", getErr)
				}
			} else if asyncTransportRetryable(getErr) {
				select {
				case <-ctx.Done():
					return 0, ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
				continue
			} else {
				return 0, getErr
			}
			if err = w.async.waitPersistenceRetry(ctx, sessionRef(w.SessionID), w.OwnerEpoch); err != nil {
				return 0, err
			}
			a, checkErr := w.async.checkpoint(sessionRef(w.SessionID))
			if checkErr != nil {
				continue
			} // next retry validates integrity and the live lease
			if a.Owner.Epoch != w.OwnerEpoch {
				return 0, fmt.Errorf("async position owner generation changed")
			}
		}
		_, publishSpan := otel.Tracer("cluster").Start(ctx, "nats.positions.puback")
		if expected == 0 {
			revision, err = w.KV.Create(key, data)
		} else {
			revision, err = w.KV.Update(key, data, expected)
		}
		publishSpan.End()
		if err == nil {
			break
		}
		if asyncTransportRetryable(err) || (ambiguous && errors.Is(err, nats.ErrKeyExists)) {
			ambiguous = true
			continue
		}
		return 0, err
	}
	if revision == 0 {
		return 0, fmt.Errorf("invalid position acknowledgement")
	}
	w.revisionMu.Lock()
	w.revisions[key] = revision
	w.revisionMu.Unlock()
	return revision, nil
}

func (p *Projection) checkAsyncPositionReplayLocked(key string, value *pb.PositionValue, revision uint64) error {
	if p.asyncPositions == nil {
		return nil
	}
	item, active := p.asyncPositions.values[key]
	if !active {
		return nil
	}
	if revision <= p.asyncPositions.durable[key] {
		return nil
	}
	if value != nil {
		for _, accepted := range p.asyncPositions.pending {
			if accepted.SessionId == item.Value.SessionId && accepted.OwnerEpoch == item.Value.OwnerEpoch && accepted.AircraftKey == item.Value.AircraftKey && proto.Equal(accepted, value) {
				return nil
			}
		}
	}
	err := fmt.Errorf("foreign position mutation in active RAM owner generation")
	p.observationErr = err
	p.wakeWaitersLocked()
	if p.Async != nil {
		p.Async.Invalidate(sessionRef(item.Value.SessionId), err)
	}
	return err
}

func (p *Projection) asyncPositionHealthLocked() error {
	if err := p.healthLocked(); err != nil {
		return err
	}
	if p.positionCursor.lastProved.IsZero() || time.Since(p.positionCursor.lastProved) > 2*time.Second {
		return ErrPositionIntegrityStale
	}
	return nil
}

func (p *Projection) PositionSourceMatches(a, b uint64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.positionSourceMatchesLocked(a, b)
}
func (p *Projection) positionSourceMatchesLocked(a, b uint64) bool {
	if a == b {
		return true
	}
	if p.asyncPositions == nil {
		return false
	}
	if a&localPositionBit != 0 {
		if durable := p.asyncPositions.tokens[a]; durable > 0 {
			a = durable
		}
	}
	if b&localPositionBit != 0 {
		if durable := p.asyncPositions.tokens[b]; durable > 0 {
			b = durable
		}
	}
	return a == b
}

func (p *Projection) positionAtLocked(key string) (KVPosition, bool) {
	if p.asyncPositions != nil {
		if item, ok := p.asyncPositions.values[key]; ok {
			return item, true
		}
	}
	item, ok := p.positions[key]
	return item, ok
}
