package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"github.com/nats-io/nats.go"
	"strings"
	"time"
)

type positionCursor struct {
	sub                                                                 *nats.Subscription
	consumer                                                            string
	created                                                             time.Time
	lastProved                                                          time.Time
	appliedConsumer, appliedStream                                      uint64
	provedConsumer, provedStream                                        uint64
	provedName                                                          string
	retained                                                            map[string]uint64
	changed                                                             chan struct{}
	metadataAt                                                          time.Time
	metadataStreamCreated                                               time.Time
	metadataStreamFirst, metadataStreamHead, metadataStreamMessages     uint64
	metadataConsumer                                                    string
	metadataDeliveredConsumer, metadataDeliveredStream, metadataPending uint64
}

var errPositionConsumerReset = errors.New("position consumer generation changed")
var errPositionTransport = errors.New("position consumer transport unavailable")

func (p *Projection) wakePositionCursorLocked() {
	if p.positionCursor.changed != nil {
		close(p.positionCursor.changed)
		p.positionCursor.changed = nil
	}
}
func (p *Projection) watchPositions(ctx context.Context) {
	for ctx.Err() == nil {
		err := p.replayPositionConsumer(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errPositionConsumerReset) || errors.Is(err, errPositionTransport) {
			p.mu.Lock()
			p.positionReplayProblem = err.Error()
			p.positionReady = false
			p.wakeWaitersLocked()
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		p.failObservation(err)
		return
	}
}
func (p *Projection) replayPositionConsumer(ctx context.Context) error {
	stream := "KV_" + p.Config.Names.Positions
	info, err := p.JS.StreamInfo(stream, nats.Context(ctx))
	if err != nil {
		return fmt.Errorf("%w: initial stream metadata: %v", errPositionTransport, err)
	}
	sub, err := p.JS.SubscribeSync("$KV."+p.Config.Names.Positions+".>", nats.BindStream(stream), nats.DeliverAll(), nats.OrderedConsumer())
	if err != nil {
		return fmt.Errorf("%w: initial subscription: %v", errPositionTransport, err)
	}
	defer sub.Unsubscribe()
	ci, err := sub.ConsumerInfo()
	if err != nil {
		return fmt.Errorf("%w: initial consumer metadata: %v", errPositionTransport, err)
	}
	p.mu.Lock()
	p.positionReady = false
	p.resetAsyncPositionOverlayLocked()

	p.wakeWaitersLocked()
	for _, old := range p.positions {
		p.publishObservationLocked(old.Value.SessionId, positionObservation(old, false, true))
	}
	p.positionCursor = positionCursor{sub: sub, consumer: ci.Name, created: info.Created, retained: map[string]uint64{}}
	p.positions = map[string]KVPosition{}
	p.positionRevision = map[string]uint64{}
	p.wakeWaitersLocked()
	p.mu.Unlock()
	proofCtx, stopProof := context.WithCancel(ctx)
	proofErrors := make(chan error, 1)
	proofDone := make(chan struct{})
	go func() {
		defer close(proofDone)
		if err := p.monitorPositionCursor(proofCtx, sub); err != nil {
			proofErrors <- err
		}
	}()
	defer func() { stopProof(); <-proofDone }()
	for ctx.Err() == nil {
		select {
		case proofErr := <-proofErrors:
			return proofErr
		default:
		}
		nextCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		msg, readErr := sub.NextMsgWithContext(nextCtx)
		cancel()
		if readErr == nil {
			if err = p.applyPositionMessage(msg); err != nil {
				return err
			}
		} else if !errors.Is(readErr, context.DeadlineExceeded) && !errors.Is(readErr, nats.ErrTimeout) {
			return fmt.Errorf("%w: %v", errPositionTransport, readErr)
		}

	}
	return ctx.Err()
}
func (p *Projection) observationFailurePresent() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.observationErr != nil
}

func (p *Projection) applyPositionMessage(msg *nats.Msg) error {
	meta, err := msg.Metadata()
	if err != nil {
		return err
	}
	prefix := "$KV." + p.Config.Names.Positions + "."
	if !strings.HasPrefix(msg.Subject, prefix) {
		return fmt.Errorf("position subject mismatch")
	}
	key := strings.TrimPrefix(msg.Subject, prefix)
	var value *pb.PositionValue
	op := msg.Header.Get("KV-Operation")
	if op == "" {
		value = &pb.PositionValue{}
		err = pb.UnmarshalStrict(msg.Data, value)
		if err == nil {
			err = validateTyped(value.ProtoReflect())
		}
		if err == nil && (value.SchemaVersion != 1 || value.GetObservation() == nil || key != fmt.Sprintf("%d.%s.%d", value.SessionId, value.AircraftKey, value.OwnerEpoch) || strings.Contains(value.AircraftKey, ".")) {
			err = fmt.Errorf("position key or schema mismatch")
		}
		if err != nil {
			return err
		}
	} else if op != "DEL" && op != "PURGE" {
		return fmt.Errorf("unknown position KV operation")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	c := &p.positionCursor
	if meta.Consumer != c.consumer {
		return errPositionConsumerReset
	}
	if meta.Sequence.Consumer != c.appliedConsumer+1 {
		return fmt.Errorf("position consumer application gap")
	}
	c.appliedConsumer = meta.Sequence.Consumer
	c.appliedStream = meta.Sequence.Stream
	c.retained[key] = meta.Sequence.Stream
	if err := p.checkAsyncPositionReplayLocked(key, value, meta.Sequence.Stream); err != nil {
		return err
	}
	if err := p.materializePositionLocked(key, value, meta.Sequence.Stream, meta.Timestamp); err != nil {
		return err
	}
	p.wakePositionCursorLocked()
	return nil
}

// Delivered.Stream is the server scan cursor, including retention holes. The
// contiguous consumer sequence proves all delivered records were applied.
// Retained keys include deletion markers, detecting silent administrative purge.
func (p *Projection) provePositionCursor(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	p.mu.RLock()
	sub := p.positionCursor.sub
	p.mu.RUnlock()
	if sub == nil {
		return false, fmt.Errorf("position cursor unavailable")
	}
	info, err := p.JS.StreamInfo("KV_"+p.Config.Names.Positions, nats.Context(ctx))
	if err != nil {
		return false, fmt.Errorf("position stream metadata: %w", err)
	}
	ci, err := sub.ConsumerInfo()
	if errors.Is(err, nats.ErrConsumerInfoOnOrderedReset) {
		return false, errPositionConsumerReset
	}
	if err != nil {
		return false, fmt.Errorf("position consumer metadata: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return false, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.verifyPositionCursorLocked(sub, info, ci)
}

func (p *Projection) verifyPositionCursorLocked(sub *nats.Subscription, info *nats.StreamInfo, ci *nats.ConsumerInfo) (bool, error) {
	c := &p.positionCursor
	c.metadataAt = time.Now()
	c.metadataStreamCreated = info.Created
	c.metadataStreamFirst, c.metadataStreamHead, c.metadataStreamMessages = info.State.FirstSeq, info.State.LastSeq, info.State.Msgs
	c.metadataConsumer = ci.Name
	c.metadataDeliveredConsumer, c.metadataDeliveredStream, c.metadataPending = ci.Delivered.Consumer, ci.Delivered.Stream, ci.NumPending
	if c.sub != sub || c.consumer != ci.Name {
		return false, errPositionConsumerReset
	}
	if !c.created.Equal(info.Created) || info.Config.MaxAge != 0 || info.Config.MaxMsgsPerSubject != 1 || info.Config.MaxMsgs != -1 || info.Config.MaxBytes != -1 || info.Config.AllowMsgTTL {
		return false, p.positionIntegrityFailureLocked("position stream identity or retention changed")
	}
	if ci.Delivered.Stream < info.State.LastSeq || c.appliedConsumer < ci.Delivered.Consumer || c.appliedStream > info.State.LastSeq {
		return false, nil
	}
	if uint64(len(c.retained)) != info.State.Msgs {
		return false, p.positionIntegrityFailureLocked("position retained view changed without ordered delivery")
	}
	c.lastProved = time.Now()
	c.provedConsumer = c.appliedConsumer
	c.provedStream = c.appliedStream
	c.provedName = c.consumer
	return true, nil
}
func (p *Projection) positionIntegrityFailureLocked(reason string) error {
	err := errors.New(reason)
	p.observationErr = err
	if p.Async != nil {
		p.Async.Invalidate(nil, err)
	}
	p.positionReady = false
	p.wakeWaitersLocked()
	return err
}

// Called only with the owner writer drained and paused. No fleet Keys/Get RPCs.
func (p *Projection) FenceLifecyclePositions(ctx context.Context, session int32, epoch uint64, connection string, observations []KVPosition) error {
	for {
		caught, err := p.provePositionCursor(ctx)
		if err != nil {
			return err
		}
		p.mu.Lock()
		if err = p.healthLocked(); err != nil {
			p.mu.Unlock()
			return err
		}
		state := p.states[fmt.Sprintf("fs.v1.state.session.%d", session)]
		if state == nil || state.Owner == nil || state.Owner.Epoch != epoch || state.Master == nil || state.Master.ConnectionId != connection || p.operationalSyncLocked(state, session) == nil {
			p.mu.Unlock()
			return fmt.Errorf("position lifecycle authority changed")
		}
		if caught && p.positionCursor.provedName == p.positionCursor.consumer && p.positionCursor.provedConsumer == p.positionCursor.appliedConsumer && p.materializedPositionsCaughtLocked() {
			err = p.compareLifecyclePositionsLocked(session, epoch, connection, observations)
			p.mu.Unlock()
			return err
		}
		if p.positionCursor.changed == nil {
			p.positionCursor.changed = make(chan struct{})
		}
		changed := p.positionCursor.changed
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func (p *Projection) compareLifecyclePositionsLocked(session int32, epoch uint64, connection string, observations []KVPosition) error {
	view := p.positionViewLocked()
	expected := make(map[string]uint64, len(observations))
	for _, item := range observations {
		if item.Stale {
			continue
		}
		v := item.Value
		if v == nil || v.SessionId != session || v.OwnerEpoch != epoch || v.SourceConnectionId != connection || item.Revision == 0 {
			return fmt.Errorf("invalid lifecycle position observation")
		}
		key := fmt.Sprintf("%d.%s.%d", session, v.AircraftKey, epoch)
		current, ok := view[key]
		if !ok || current.Revision != item.Revision || current.Value.SourceConnectionId != connection || current.Value.SessionId != session || current.Value.OwnerEpoch != epoch || current.Value.AircraftKey != v.AircraftKey {
			return fmt.Errorf("lifecycle position observation changed")
		}
		if old, duplicate := expected[key]; duplicate && old != item.Revision {
			return fmt.Errorf("conflicting lifecycle position observation")
		}
		expected[key] = item.Revision
	}
	for key, item := range view {
		v := item.Value
		if v.SessionId == session && v.OwnerEpoch == epoch && v.SourceConnectionId == connection {
			if expected[key] != item.Revision {
				return fmt.Errorf("lifecycle position set changed")
			}
		}
	}
	return nil
}

// A local PubAck may materialize ahead of the last leader checkpoint. Fleet
// acceptance still requires raw replay through every such revision, including
// deletion watermarks, before using the complete materialized neighbor set.
func (p *Projection) materializedPositionsCaughtLocked() bool {
	for _, revision := range p.positionRevision {
		if revision > p.positionCursor.appliedStream {
			return false
		}
	}
	for _, position := range p.positions {
		if position.Revision > p.positionCursor.appliedStream {
			return false
		}
	}
	return true
}

// Metadata requests must never run on the ordered application goroutine: the
// server may deliver more records while ConsumerInfo is in flight. Application
// remains free to advance the exact frontier captured by that metadata reply.
func (p *Projection) monitorPositionCursor(ctx context.Context, sub *nats.Subscription) error {
	return p.monitorPositionCursorWithProof(ctx, sub, p.provePositionCursor)
}

func (p *Projection) monitorPositionCursorWithProof(ctx context.Context, sub *nats.Subscription, prove func(context.Context) (bool, error)) error {
	for ctx.Err() == nil {
		caught, err := prove(ctx)
		if err != nil {
			if errors.Is(err, errPositionConsumerReset) || p.observationFailurePresent() {
				return err
			}
			return fmt.Errorf("%w: %v", errPositionTransport, err)
		}
		if caught {
			p.mu.Lock()
			if p.positionCursor.sub != sub {
				p.mu.Unlock()
				return errPositionConsumerReset
			}
			currentProof := p.positionCursor.provedName == p.positionCursor.consumer && p.positionCursor.provedConsumer == p.positionCursor.appliedConsumer
			if currentProof {
				p.positionReady = true
				p.positionReplayProblem = ""
				p.pruneAsyncPositionAliasesLocked()
			}
			caught = currentProof
			p.wakeWaitersLocked()
			p.mu.Unlock()
		}
		delay := time.Second
		if !caught {
			delay = 10 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return ctx.Err()
}
