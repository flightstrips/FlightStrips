package cluster

import (
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
)

const retryWindow = 4096
const terminalWindow = 1024

func (a *Aggregate) LookupOutcome(id string) (*pb.CommandOutcome, error) { return a.Ledger[id], nil }
func (a *Aggregate) LookupWorkflow(id string) (*pb.WorkflowRecord, error) {
	return a.Workflows[id], nil
}
func (a *Aggregate) LookupEffect(id string) (*pb.EffectRecord, error) { return a.Effects[id], nil }

func terminalEffect(e *pb.EffectRecord) bool {
	return e.Status == pb.EffectRecord_EXECUTED || e.Status == pb.EffectRecord_FAILED || e.Status == pb.EffectRecord_EXPIRED || e.Status == pb.EffectRecord_UNKNOWN
}

// trimRecords bounds retry materialization. It only deletes from replacement
// maps, so earlier published views and concurrent snapshot jobs remain valid.
func (a *Aggregate) trimRecords() {
	if len(a.Ledger) > retryWindow {
		pinned := make(map[string]bool)
		for id, effect := range a.Effects {
			if !terminalEffect(effect) {
				pinned[id] = true
			}
		}
		for _, workflow := range a.Workflows {
			if workflow.Status == pb.WorkflowRecord_PENDING {
				pinned[workflow.DerivedCommandId] = true
			}
		}
		trimMap(a.Ledger, retryWindow, func(id string) bool { return pinned[id] || a.Ledger[id].CommittedStreamSequence == 0 }, func(id string) uint64 { return a.Ledger[id].CommittedStreamSequence })
	}
	if len(a.Workflows) > terminalWindow {
		var checkpoint *pb.WorkflowRecord
		for _, w := range a.Workflows {
			if w.Status == pb.WorkflowRecord_COMPLETED && strings.HasPrefix(w.Step, transceiverSectorStep) && (checkpoint == nil || w.GetSourceRevision() > checkpoint.GetSourceRevision() || w.GetSourceRevision() == checkpoint.GetSourceRevision() && w.WorkflowId < checkpoint.WorkflowId) {
				checkpoint = w
			}
		}
		trimMap(a.Workflows, terminalWindow, func(id string) bool {
			w := a.Workflows[id]
			return w.Status == pb.WorkflowRecord_PENDING || w == checkpoint
		}, func(id string) uint64 {
			w := a.Workflows[id]
			if outcome := a.Ledger[w.DerivedCommandId]; outcome != nil {
				return outcome.CommittedStreamSequence
			}
			return w.GetDestinationStreamSequence()
		})
	}
	if len(a.Effects) > terminalWindow {
		trimMap(a.Effects, terminalWindow, func(id string) bool { return !terminalEffect(a.Effects[id]) }, func(id string) uint64 {
			if outcome := a.Ledger[id]; outcome != nil {
				return outcome.CommittedStreamSequence
			}
			return 0
		})
	}
	// Observation identities are cursors, not an append-only event archive.
	if entries := a.Indexes[pb.EntityKind_VATSIM_OBSERVATION]; len(entries) > 0 {
		latest := make(map[string]*pb.EntitySnapshot)
		for _, e := range entries {
			o := e.Value.GetVatsimObservation()
			previous := latest[o.Callsign]
			if previous == nil || o.ProviderId > previous.Value.GetVatsimObservation().ProviderId {
				latest[o.Callsign] = e
			}
		}
		changed := false
		for _, e := range entries {
			if latest[e.Value.GetVatsimObservation().Callsign] != e {
				delete(a.Entities, entitySlot(a.Entities, pb.EntityKind_VATSIM_OBSERVATION, e.Key))
				changed = true
			}
		}
		if changed {
			a.rebuildIndexes()
		}
	}
}

// Pins are outside the terminal/retry budget: unfinished work cannot be lost
// just because unrelated updates filled the recent receipt window.
func trimMap[T any](values map[string]T, limit int, pinned func(string) bool, sequence func(string) uint64) {
	keys := make([]string, 0, len(values))
	for id := range values {
		if !pinned(id) {
			keys = append(keys, id)
		}
	}
	if len(keys) <= limit {
		return
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := sequence(keys[i]), sequence(keys[j])
		if left == right {
			return keys[i] < keys[j]
		}
		return left < right
	})
	for _, id := range keys[:len(keys)-limit] {
		delete(values, id)
	}
}
