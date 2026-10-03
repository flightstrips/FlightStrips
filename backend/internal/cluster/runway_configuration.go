package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"fmt"
	"google.golang.org/protobuf/proto"
)

// ReadRunwayConfiguration copies the session and its fencing/sync terms from
// one accepted publication, without copying strips or historical commands.
func (p *Projection) ReadRunwayConfiguration(id int32) (*Aggregate, error) {
	if err := p.sessionReadHealth(sessionRef(id)); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	state := p.acceptedStateLocked(fmt.Sprintf("fs.v1.state.session.%d", id))
	if state == nil || state.Owner == nil {
		return nil, fmt.Errorf("session terms unavailable")
	}
	out := NewAggregate(sessionRef(id))
	out.Owner = proto.Clone(state.Owner).(*pb.OwnerTerm)
	if state.Master != nil {
		out.Master = proto.Clone(state.Master).(*pb.MasterTerm)
	}
	if state.Sync != nil {
		out.Sync = proto.Clone(state.Sync).(*pb.SessionSync)
	}
	for _, entity := range state.Indexes[pb.EntityKind_SESSION] {
		out.Entities[entitySlot(out.Entities, pb.EntityKind_SESSION, entity.Key)] = proto.Clone(entity).(*pb.EntitySnapshot)
	}
	out.rebuildIndexes()
	return out, nil
}
