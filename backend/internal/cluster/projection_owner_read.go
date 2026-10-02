package cluster

import (
	"fmt"

	pb "FlightStrips/pkg/events/cluster"
)

// ReadOwned is the domain writer's detached planning state. A successful
// subject-CAS append proves this subject prefix was current at commit, even
// when the independent global consumer is processing unrelated aggregates.
// Callers must also check CanCommitLocal; this is not an external-effect or
// takeover admission gate. Cross-aggregate dependencies retain ordinary Read.
func (p *Projection) ReadOwned(ref *pb.AggregateRef, nodeID string) (*Aggregate, error) {
	if memory, err := p.readMemory(ref); err != nil || memory != nil {
		return memory, err
	}
	return p.readOwnedDurable(ref, nodeID)
}

func (p *Projection) readOwnedDurable(ref *pb.AggregateRef, nodeID string) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.healthLocked(); err != nil {
		return nil, err
	}
	if p.history != nil {
		if err := p.history.check(); err != nil {
			return nil, err
		}
	}
	state := p.states[subject]
	if nodeID == "" || state == nil || state.Owner == nil || state.Owner.NodeId != nodeID {
		return nil, fmt.Errorf("local planning owner changed")
	}
	return cloneAggregate(state)
}
