package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"fmt"
	"strings"
)

// IdleForPositionReset is called after projection admission has been closed by
// positionReady=false. Execute rechecks its checkpoint after obtaining a turn,
// so an initial checkpoint captured before that boundary cannot accept work.
// This method never acquires projection/history locks.
func (m *AsyncSessionOwners) IdleForPositionReset() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failure != nil || m.turns != 0 {
		return false
	}
	for _, session := range m.sessions {
		if session.pending != 0 || len(session.tail) != 0 {
			return false
		}
	}
	return true
}

func (m *AsyncSessionOwners) positionTokenReferences() map[uint64]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	references := map[uint64]bool{}
	record := func(deadline *pb.SessionDeadline) {
		if deadline != nil && deadline.Kind == "aircraft-disconnect" && deadline.SourceRevision&localPositionBit != 0 {
			references[deadline.SourceRevision] = true
		}
	}
	for _, session := range m.sessions {
		if session.ram != nil {
			for _, entity := range session.ram.Indexes[pb.EntityKind_SESSION_DEADLINE] {
				record(entity.GetValue().GetSessionDeadline())
			}
		}
		for _, event := range session.tail {
			for _, change := range event.GetDomainChanged().GetChanges() {
				record(change.GetUpsert().GetSessionDeadline())
			}
		}
	}
	return references
}

// Prune only aliases whose last RAM/queued deadline and position references have
// disappeared. No wall-clock expiry may invalidate a still-pending source guard.
func (p *Projection) pruneAsyncPositionAliasesLocked() {
	if p.asyncPositions == nil {
		return
	}
	refs := map[uint64]bool{}
	if p.Async != nil {
		refs = p.Async.positionTokenReferences()
	}
	for key, item := range p.asyncPositions.values {
		if item.Value == nil {
			delete(p.asyncPositions.values, key)
			continue
		}
		state := p.acceptedStateLocked("fs.v1.state.session." + fmt.Sprint(item.Value.SessionId))
		if state != nil && state.Owner != nil && item.Value.OwnerEpoch < state.Owner.Epoch {
			delete(p.asyncPositions.values, key)
		} else {
			refs[item.Revision] = true
		}
	}
	pendingKeys := map[string]bool{}
	pendingBaselines := map[string]bool{}
	for token, value := range p.asyncPositions.pending {
		refs[token] = true
		if value != nil {
			pendingKeys[positionKey(value.SessionId, value.AircraftKey, value.OwnerEpoch)] = true
			pendingBaselines[fmt.Sprintf("%d.%d", value.SessionId, value.OwnerEpoch)] = true
		}
	}
	for token := range p.asyncPositions.tokens {
		if !refs[token] {
			delete(p.asyncPositions.tokens, token)
		}
	}
	// Old epochs cannot publish another accepted background job at this point:
	// pending tokens are explicitly retained above, and core owns generation fences.
	for key := range p.asyncPositions.durable {
		pieces := strings.Split(key, ".")
		if len(pieces) == 3 && !pendingKeys[key] && !p.currentPositionBaselineLocked(pieces[0]+"."+pieces[2]) {
			delete(p.asyncPositions.durable, key)
		}
	}
	for key := range p.asyncPositions.baselines {
		if !pendingBaselines[key] && !p.currentPositionBaselineLocked(key) {
			delete(p.asyncPositions.baselines, key)
		}
	}
}

func (p *Projection) currentPositionBaselineLocked(key string) bool {
	var session int32
	var epoch uint64
	if _, err := fmt.Sscanf(key, "%d.%d", &session, &epoch); err != nil {
		return false
	}
	state := p.acceptedStateLocked(fmt.Sprintf("fs.v1.state.session.%d", session))
	return state != nil && state.Owner != nil && state.Owner.Epoch == epoch
}

// Caller has already closed positionReady under p.mu. Existing turns/pending
// jobs prohibit reuse; a fully flushed process can recover through full replay.
func (p *Projection) resetAsyncPositionOverlayLocked() {
	if p.asyncPositions != nil {
		idle := p.Async == nil || p.Async.IdleForPositionReset()
		if !idle {
			err := fmt.Errorf("position replay reset during pending RAM owner generation")
			if p.Async != nil {
				p.Async.Invalidate(nil, err)
			}
			p.observationErr = err
		}
		// Bound writers keep the manager; replay repopulates its durable CAS map.
		p.asyncPositions.values = map[string]KVPosition{}
		p.asyncPositions.tokens = map[uint64]uint64{}
		p.asyncPositions.pending = map[uint64]*pb.PositionValue{}
		p.asyncPositions.baselines = map[string]bool{}
		p.asyncPositions.durable = map[string]uint64{}
	}

}
