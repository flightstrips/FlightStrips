package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/proto"
	"time"
)

// ApplyCommittedPosition materializes only this owner's exact durable CAS result.
// It does not advance the ordered consumer or its complete-fleet proof.
func (p *Projection) ApplyCommittedPosition(nodeID string) func(context.Context, *pb.PositionValue, uint64) error {
	return func(ctx context.Context, value *pb.PositionValue, revision uint64) error {
		ctx, span := otel.Tracer("cluster").Start(ctx, "euroscope.position.local_apply")
		defer span.End()
		if value == nil || revision == 0 || !canonicalAircraft(value.AircraftKey) {
			return fmt.Errorf("invalid committed position")
		}
		key := positionKey(value.SessionId, value.AircraftKey, value.OwnerEpoch)
		p.mu.RLock()
		created := p.positionCursor.created
		sub, consumer := p.positionCursor.sub, p.positionCursor.consumer
		ready := p.positionReady
		p.mu.RUnlock()
		if !ready || created.IsZero() || sub == nil || consumer == "" {
			return fmt.Errorf("position cursor generation unavailable")
		}

		msg, err := p.JS.GetMsg("KV_"+p.Config.Names.Positions, revision, nats.Context(ctx))
		if err != nil {
			return err
		}
		info, err := p.JS.StreamInfo("KV_"+p.Config.Names.Positions, nats.Context(ctx))
		if err != nil {
			return err
		}
		if info == nil || !info.Created.Equal(created) {
			return fmt.Errorf("committed position stream generation mismatch")
		}
		accepted := &pb.PositionValue{}
		if msg == nil || msg.Sequence != revision || msg.Subject != "$KV."+p.Config.Names.Positions+"."+key || msg.Header.Get("KV-Operation") != "" || msg.Time.IsZero() || msg.Time.Before(created) {
			return fmt.Errorf("committed position metadata mismatch")
		}
		if err = pb.UnmarshalStrict(msg.Data, accepted); err != nil {
			return err
		}
		if !proto.Equal(accepted, value) {
			return fmt.Errorf("committed position value mismatch")
		}
		if accepted.SchemaVersion != 1 || accepted.GetObservation() == nil {
			return fmt.Errorf("committed position schema mismatch")
		}
		if err = validateTyped(accepted.ProtoReflect()); err != nil {
			return err
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		if !p.positionCursor.created.Equal(created) || p.positionCursor.sub != sub || p.positionCursor.consumer != consumer {
			return fmt.Errorf("position stream generation changed")
		}
		if err = p.healthLocked(); err != nil {
			return err
		}
		state := p.states[fmt.Sprintf("fs.v1.state.session.%d", value.SessionId)]
		if state == nil || state.Owner == nil || state.Master == nil || state.Owner.NodeId != nodeID || state.Owner.Epoch != value.OwnerEpoch || state.Master.OwnerEpoch != value.OwnerEpoch || state.Master.ConnectionId != value.SourceConnectionId || state.Owner.LeaseUntil == nil || !time.Now().Before(state.Owner.LeaseUntil.AsTime()) || !msg.Time.Before(state.Owner.LeaseUntil.AsTime()) || p.NC == nil || p.NC.Status() != nats.CONNECTED || p.operationalSyncLocked(state, value.SessionId) == nil {
			return fmt.Errorf("committed position owner/master changed")
		}
		if current := p.positionRevisionLocked(key); current > revision {
			return fmt.Errorf("committed position was superseded")
		}
		return p.materializePositionLocked(key, accepted, revision, msg.Time)
	}
}

func (p *Projection) positionRevisionLocked(key string) uint64 {
	revision := p.positionRevision[key]
	if item := p.positions[key]; item.Revision > revision {
		revision = item.Revision
	}
	return revision
}

// Raw replay calls this after advancing its own cursor. Older replay therefore
// cannot overwrite an owner-local commit, even when the older record is a delete.
func (p *Projection) materializePositionLocked(key string, value *pb.PositionValue, revision uint64, observed time.Time) error {
	current := p.positionRevisionLocked(key)
	if revision < current {
		return nil
	}
	if revision == current && current != 0 {
		old, present := p.positions[key]
		if (value == nil && present) || (value != nil && (!present || !proto.Equal(old.Value, value))) {
			return fmt.Errorf("position revision content mismatch")
		}
		return nil
	}
	if p.positionRevision == nil {
		p.positionRevision = map[string]uint64{}
	}
	p.positionRevision[key] = revision
	if value == nil {
		if old, ok := p.positions[key]; ok && (p.asyncPositions == nil || p.asyncPositions.values[key].Value == nil) {
			p.publishObservationLocked(old.Value.SessionId, positionObservation(old, false, true))
		}
		delete(p.positions, key)
	} else {
		if p.positions == nil {
			p.positions = map[string]KVPosition{}
		}
		p.positions[key] = KVPosition{Value: value, Revision: revision, Observed: observed}
		if selected, ok := p.selectedPositionLocked(value.SessionId, value.AircraftKey); ok && selected.Revision == revision && (p.asyncPositions == nil || p.asyncPositions.values[key].Value == nil) {
			p.publishObservationLocked(value.SessionId, positionObservation(selected, selected.Stale, false))
		}
	}
	p.wakePositionWaitersLocked(key)
	return nil
}
