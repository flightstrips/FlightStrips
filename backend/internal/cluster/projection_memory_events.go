package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"google.golang.org/protobuf/proto"
)

// PublishMemoryEvent delivers execution on the owner immediately. Independent
// replay still confirms durable state, but must not redeliver the same changes.
func (p *Projection) PublishMemoryEvent(event *pb.StateEvent) {
	p.memoryEvents.Store(event.EventId, struct{}{})
	subject, _ := Subject(event.Aggregate)
	delta := &pb.FrontendDelta{Aggregate: proto.Clone(event.Aggregate).(*pb.AggregateRef), AggregateRevision: event.AggregateRevision}
	for _, change := range event.GetDomainChanged().GetChanges() {
		delta.Changes = append(delta.Changes, proto.Clone(change).(*pb.EntityChange))
	}
	for _, workflow := range event.GetDomainChanged().GetWorkflows() {
		delta.Workflows = append(delta.Workflows, proto.Clone(workflow).(*pb.WorkflowRecord))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for id, listener := range p.listeners {
		if listener.subject != subject {
			continue
		}
		select {
		case listener.updates <- delta:
		default:
			p.closeListenerLocked(id)
		}
	}
}

// FlushSession is used within an accepted session turn before an external
// workflow claim. The worker never acquires that turn, so it can finish the tail.
func (m *AsyncSessionOwners) FlushSession(ctx context.Context, ref *pb.AggregateRef) error {
	key, err := Subject(ref)
	if err != nil {
		return err
	}
	for {
		m.mu.Lock()
		err = m.failure
		s := m.sessions[key]
		pending := s != nil && s.pending > 0
		changed := m.changed
		m.mu.Unlock()
		if err != nil || !pending {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
