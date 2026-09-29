package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

func (p *Projection) watchPositions(ctx context.Context) {
	watcher, err := p.Positions.WatchAll(nats.Context(ctx))
	if err != nil {
		p.failObservation(err)
		return
	}
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				p.failObservation(fmt.Errorf("position watcher stopped"))
				return
			}
			p.mu.Lock()
			if entry == nil {
				p.positionReady = true
				p.mu.Unlock()
				continue
			}
			if entry.Operation() != nats.KeyValuePut {
				delete(p.positions, entry.Key())
				p.mu.Unlock()
				continue
			}
			value := &pb.PositionValue{}
			err := pb.UnmarshalStrict(entry.Value(), value)
			if err == nil {
				err = validateTyped(value.ProtoReflect())
			}
			if err == nil && (value.SchemaVersion != 1 || value.GetObservation() == nil || entry.Key() != fmt.Sprintf("%d.%s.%d", value.SessionId, value.AircraftKey, value.OwnerEpoch) || strings.Contains(value.AircraftKey, ".")) {
				err = fmt.Errorf("position key or schema mismatch")
			}
			if err != nil {
				p.observationErr = err
				p.mu.Unlock()
				return
			}
			p.positions[entry.Key()] = KVPosition{Value: value, Revision: entry.Revision(), Observed: entry.Created()}
			p.mu.Unlock()
		}
	}
}

func (p *Projection) watchPresence(ctx context.Context) {
	watcher, err := p.Presence.WatchAll(nats.Context(ctx))
	if err != nil {
		p.failObservation(err)
		return
	}
	defer watcher.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case entry, ok := <-watcher.Updates():
			if !ok {
				p.failObservation(fmt.Errorf("presence watcher stopped"))
				return
			}
			p.mu.Lock()
			if entry == nil {
				p.presenceReady = true
				p.mu.Unlock()
				continue
			}
			if entry.Operation() != nats.KeyValuePut {
				delete(p.presence, entry.Key())
				p.mu.Unlock()
				continue
			}
			value := &pb.PresenceValue{}
			err := pb.UnmarshalStrict(entry.Value(), value)
			if err == nil {
				err = validateTyped(value.ProtoReflect())
			}
			key := ""
			if node := value.GetNode(); node != nil {
				key = "node." + node.NodeId
			}
			if client := value.GetClient(); client != nil {
				key = "client." + client.ConnectionId
			}
			if err == nil && (value.SchemaVersion != 1 || key == "" || entry.Key() != key || strings.Count(key, ".") != 1) {
				err = fmt.Errorf("presence key or schema mismatch")
			}
			if err != nil {
				p.observationErr = err
				p.mu.Unlock()
				return
			}
			p.presence[entry.Key()] = KVPresence{Value: value, Revision: entry.Revision(), Observed: entry.Created()}
			p.mu.Unlock()
		}
	}
}

func (p *Projection) failObservation(err error) { p.mu.Lock(); p.observationErr = err; p.mu.Unlock() }

// ObservationSnapshot keeps KV revisions separate from the FS_STATE stream
// revision. Expired presence is filtered even if no delete notification arrived.
func (p *Projection) ObservationSnapshot(sessionID int32) ([]KVPosition, []KVPresence, error) {
	if err := p.Ready(); err != nil {
		return nil, nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	positions := make([]KVPosition, 0)
	presence := make([]KVPresence, 0)
	for _, item := range p.positions {
		if item.Value.SessionId == sessionID {
			positions = append(positions, KVPosition{Value: proto.Clone(item.Value).(*pb.PositionValue), Revision: item.Revision, Observed: item.Observed})
		}
	}
	for _, item := range p.presence {
		if time.Since(item.Observed) >= 10*time.Second {
			continue
		}
		if client := item.Value.GetClient(); client != nil && client.SessionId != sessionID {
			continue
		}
		presence = append(presence, KVPresence{Value: proto.Clone(item.Value).(*pb.PresenceValue), Revision: item.Revision, Observed: item.Observed})
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i].Value.AircraftKey < positions[j].Value.AircraftKey })
	sort.Slice(presence, func(i, j int) bool { return presenceKey(presence[i].Value) < presenceKey(presence[j].Value) })
	return positions, presence, nil
}

func presenceKey(v *pb.PresenceValue) string {
	if n := v.GetNode(); n != nil {
		return "node." + n.NodeId
	}
	if c := v.GetClient(); c != nil {
		return "client." + c.ConnectionId
	}
	return ""
}

// OperationalSync accepts a durable sync marker only for a currently live
// master connection and node. Both presence entries must have been renewed
// since this replica started, so a full cluster restart cannot briefly reuse
// TTL entries left by the previous process incarnations.
func (p *Projection) OperationalSync(ref *pb.AggregateRef) (*pb.SessionSync, error) {
	if ref == nil || ref.GetSession() == nil {
		return nil, fmt.Errorf("operational sync requires a session")
	}
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err := p.Ready(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	state := p.states[subject]
	if state == nil || state.Sync == nil || state.Master == nil {
		return nil, nil
	}
	master, sync := state.Master, state.Sync
	if sync.ConnectionId != master.ConnectionId || sync.MasterEpoch != master.Epoch {
		return nil, nil
	}
	clientEntry := p.presence["client."+master.ConnectionId]
	client := clientEntry.Value.GetClient()
	if client == nil || client.ConnectionId != master.ConnectionId || client.Cid != master.Cid || client.SessionId != ref.GetSession().GetId() || client.Kind != pb.ClientPresence_EUROSCOPE || clientEntry.Observed.Before(p.startedAt) || time.Since(clientEntry.Observed) >= 10*time.Second {
		return nil, nil
	}
	nodeEntry := p.presence["node."+client.NodeId]
	node := nodeEntry.Value.GetNode()
	if node == nil || !node.Ready || node.NodeId != client.NodeId || nodeEntry.Observed.Before(p.startedAt) || time.Since(nodeEntry.Observed) >= 10*time.Second {
		return nil, nil
	}
	return proto.Clone(sync).(*pb.SessionSync), nil
}
