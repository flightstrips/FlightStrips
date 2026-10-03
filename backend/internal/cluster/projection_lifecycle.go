package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// ReadLifecyclePlanning returns a detached, coherent SAT planning snapshot.
// Lifecycle preflight needs every entity, term, and workflow, including archived
// workflow lookup. Command outcomes and delivery effects are not planning inputs.
// Authoritative publication still uses Writer.Execute's complete snapshot.
func (p *Projection) ReadLifecyclePlanning(ref *pb.AggregateRef) (*Aggregate, error) {
	return p.ReadLifecyclePlanningCached(ref, nil)
}

// ReadLifecyclePlanningCached reuses an immutable detached snapshot only when
// its accepted checkpoint is unchanged. The caller keeps it within one lifecycle
// invocation and never modifies its protobuf inputs. Position, presence, source,
// and clock inputs are still read fresh by the lifecycle planner on every use.
func (p *Projection) ReadLifecyclePlanningCached(ref *pb.AggregateRef, previous *Aggregate) (*Aggregate, error) {
	if memory, err := p.memoryControl(ref); err != nil || memory != nil {
		if err != nil {
			return nil, err
		}
		if previous != nil && proto.Equal(previous.Ref, ref) && previous.Revision == memory.Revision &&
			previous.StreamSequence == memory.StreamSequence && previous.SubjectSequence == memory.SubjectSequence && previous.history == memory.history {
			return previous, nil
		}
		out := NewAggregate(ref)
		out.history = memory.history
		out.Revision, out.StreamSequence, out.SubjectSequence = memory.Revision, memory.StreamSequence, memory.SubjectSequence
		if memory.Owner != nil {
			out.Owner = proto.Clone(memory.Owner).(*pb.OwnerTerm)
		}
		if memory.Master != nil {
			out.Master = proto.Clone(memory.Master).(*pb.MasterTerm)
		}
		if memory.Sync != nil {
			out.Sync = proto.Clone(memory.Sync).(*pb.SessionSync)
		}
		for key, entity := range memory.Entities {
			out.Entities[key] = proto.Clone(entity).(*pb.EntitySnapshot)
		}
		for key, workflow := range memory.Workflows {
			out.Workflows[key] = proto.Clone(workflow).(*pb.WorkflowRecord)
		}
		out.rebuildIndexes()
		return out, nil
	}
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	if err = p.readyForRead(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.healthLocked(); err != nil {
		return nil, err
	}
	state := p.states[subject]
	if state != nil && previous != nil && proto.Equal(previous.Ref, ref) && previous.Revision == state.Revision && previous.StreamSequence == state.StreamSequence && previous.SubjectSequence == state.SubjectSequence && previous.history == state.history {
		return previous, nil
	}
	out := NewAggregate(ref)
	if state != nil {
		out.history = state.history
		out.Revision, out.StreamSequence, out.SubjectSequence = state.Revision, state.StreamSequence, state.SubjectSequence
		if state.Owner != nil {
			out.Owner = proto.Clone(state.Owner).(*pb.OwnerTerm)
		}
		if state.Master != nil {
			out.Master = proto.Clone(state.Master).(*pb.MasterTerm)
		}
		if state.Sync != nil {
			out.Sync = proto.Clone(state.Sync).(*pb.SessionSync)
		}
		for key, value := range state.Entities {
			out.Entities[key] = proto.Clone(value).(*pb.EntitySnapshot)
		}
		for key, value := range state.Workflows {
			out.Workflows[key] = proto.Clone(value).(*pb.WorkflowRecord)
		}
		out.rebuildIndexes()
	}
	return out, nil
}

// PresenceSnapshot reads the same accepted presence view as ObservationSnapshot
// without cloning unrelated aircraft positions when selecting message recipients.
func (p *Projection) PresenceSnapshot(sessionID int32) ([]KVPresence, error) {
	if err := p.readyForRead(); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.presenceSnapshotLocked(sessionID), nil
}
