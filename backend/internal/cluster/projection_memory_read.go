package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"fmt"
	"time"
)

func (p *Projection) memoryControl(ref *pb.AggregateRef) (*Aggregate, error) {
	if p.Async == nil || ref == nil || ref.GetSession() == nil {
		return nil, nil
	}
	view := p.Async.Control(ref)
	if view == nil {
		if err := p.Async.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if !p.Async.Active(ref) {
		return nil, fmt.Errorf("session memory owner unavailable")
	}
	return view, nil
}

func (p *Projection) readMemory(ref *pb.AggregateRef) (*Aggregate, error) {
	if p.Async == nil || ref == nil || ref.GetSession() == nil {
		return nil, nil
	}
	return p.Async.Read(ref)
}

// sessionReadHealth permits the active owner's RAM view after the complete
// baseline has been established. A standby retains its replay barrier.
func (p *Projection) sessionReadHealth(ref *pb.AggregateRef) error {
	if p.Async != nil && ref != nil && ref.GetSession() != nil && p.Async.Active(ref) {
		return p.commandHealth()
	}
	return p.readyForRead()
}

// acceptedStateLocked is safe under p.mu: Control returns an immutable published
// view without acquiring the session executor or projection locks.
func (p *Projection) acceptedStateLocked(subject string) *Aggregate {
	raw := p.states[subject]
	if p.Async != nil {
		if ref, err := refFromSubject(subject); err == nil && ref.GetSession() != nil {
			if memory := p.Async.Control(ref); memory != nil && memory.Owner != nil && raw != nil && raw.Owner != nil &&
				memory.Owner.NodeId == raw.Owner.NodeId && memory.Owner.Epoch == raw.Owner.Epoch &&
				raw.Owner.LeaseUntil != nil && time.Now().Before(raw.Owner.LeaseUntil.AsTime()) {
				return memory
			}
		}
	}
	return raw
}
