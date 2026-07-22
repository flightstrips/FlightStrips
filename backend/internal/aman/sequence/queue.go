package sequence

import (
	"fmt"
	"sort"
	"time"

	"FlightStrips/internal/aman"
)

// QueueOfferConfig owns the explicit lifetime of an earlier-slot opportunity.
// The effective expiry is capped at the candidate slot time.
type QueueOfferConfig struct {
	Validity time.Duration
}

// QueueOfferCalculation carries the pure sequence input through the existing
// coordinator seam until that coordinator binds slots and offers to the next
// committed revision.
type QueueOfferCalculation struct {
	Input  Input
	Config QueueOfferConfig
}

type queueEntry struct {
	flight preparedFlight
	slot   aman.Slot
}

type pendingOffer struct {
	offer            aman.QueueOffer
	assignedSequence int
	flight           preparedFlight
}

type offerKey struct {
	group    aman.RunwayGroupID
	sequence int
	at       time.Time
}

// CalculateQueueOffers calculates occupied earlier-slot opportunities from a
// committed sequence revision. It neither mutates input nor allocates a new
// revision.
func CalculateQueueOffers(input Input, config QueueOfferConfig, at time.Time) ([]aman.QueueOffer, error) {
	if input.Revision == 0 {
		return nil, fmt.Errorf("queue offers require a committed airport revision")
	}
	if config.Validity <= 0 {
		return nil, fmt.Errorf("queue offers require positive validity")
	}
	if !validUTC(at) {
		return nil, fmt.Errorf("queue offer calculation time must be UTC")
	}
	policies, err := preparePolicies(input.Policies)
	if err != nil {
		return nil, err
	}
	prepared, err := prepareFlights(input.Flights, policies)
	if err != nil {
		return nil, err
	}

	pending := make(map[offerKey][]pendingOffer)
	for group, flights := range prepared {
		entries, err := queueEntries(input.Revision, flights)
		if err != nil {
			return nil, err
		}
		policy := policies[group]
		for targetIndex, target := range entries {
			if !queueEligible(target.flight) {
				continue
			}
			for candidateIndex := 0; candidateIndex < targetIndex; candidateIndex++ {
				candidate := entries[candidateIndex]
				if candidate.flight.FreezeReason != aman.FreezeNone || candidate.flight.ManualOrder != nil {
					continue
				}
				if candidate.slot.Time.Before(target.flight.OperationalTETA.Add(-policy.EarlyTolerance)) {
					continue
				}
				if crossesProtectedOrder(entries, candidateIndex, targetIndex) {
					continue
				}
				expiresAt := at.Add(config.Validity)
				if candidate.slot.Time.Before(expiresAt) {
					expiresAt = candidate.slot.Time
				}
				if !expiresAt.After(at) {
					continue
				}
				remaining := make([]allocatedEntry, 0, len(entries)-2)
				for index, entry := range entries {
					if index == candidateIndex || index == targetIndex {
						continue
					}
					remaining = append(remaining, allocatedEntry{flight: entry.flight, time: entry.slot.Time, reason: CandidateReason(entry.slot.Reason)})
				}
				valid, _, _ := placement(policy, remaining, target.flight, candidate.slot.Time)
				if !valid {
					continue
				}
				key := offerKey{group: group, sequence: candidate.slot.Sequence, at: candidate.slot.Time}
				pending[key] = append(pending[key], pendingOffer{
					offer: aman.QueueOffer{
						FlightID: target.flight.ID, RunwayGroupID: group, CandidateSlot: candidate.slot,
						ExpiresAt: expiresAt, AirportRevision: input.Revision, Reason: aman.QueueOfferEarlierOccupiedSlot,
					},
					assignedSequence: target.slot.Sequence,
					flight:           target.flight,
				})
			}
		}
	}

	offers := make([]aman.QueueOffer, 0)
	for _, candidates := range pending {
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].assignedSequence != candidates[j].assignedSequence {
				return candidates[i].assignedSequence < candidates[j].assignedSequence
			}
			return flightLess(candidates[i].flight, candidates[j].flight)
		})
		for index := range candidates {
			candidates[index].offer.QueuePosition = index + 1
			offers = append(offers, candidates[index].offer)
		}
	}
	sort.Slice(offers, func(i, j int) bool {
		a, b := offers[i], offers[j]
		if a.RunwayGroupID != b.RunwayGroupID {
			return a.RunwayGroupID < b.RunwayGroupID
		}
		if a.CandidateSlot.Sequence != b.CandidateSlot.Sequence {
			return a.CandidateSlot.Sequence < b.CandidateSlot.Sequence
		}
		if !a.CandidateSlot.Time.Equal(b.CandidateSlot.Time) {
			return a.CandidateSlot.Time.Before(b.CandidateSlot.Time)
		}
		if a.QueuePosition != b.QueuePosition {
			return a.QueuePosition < b.QueuePosition
		}
		return a.FlightID < b.FlightID
	})
	return offers, nil
}

// ProjectQueueOffers replaces every flight's offers from one calculation and
// rejects a stale or slot-inconsistent projection.
func ProjectQueueOffers(state aman.AirportState, input Input, config QueueOfferConfig, at time.Time) (aman.AirportState, error) {
	if state.Revision != input.Revision {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: fmt.Sprintf("queue offer revision %d does not match airport revision %d", input.Revision, state.Revision)}
	}
	offers, err := CalculateQueueOffers(input, config, at)
	if err != nil {
		return aman.AirportState{}, err
	}
	projected := state
	projected.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	indexes := make(map[aman.FlightID]int, len(projected.Flights))
	for index := range projected.Flights {
		projected.Flights[index].QueueOffers = nil
		indexes[projected.Flights[index].ID] = index
	}
	inputFlights := make(map[aman.FlightID]Flight, len(input.Flights))
	for _, flight := range input.Flights {
		inputFlights[flight.ID] = flight
		index, exists := indexes[flight.ID]
		if !exists || !slotsEqual(projected.Flights[index].Slot, flight.CurrentSlot) {
			return aman.AirportState{}, fmt.Errorf("queue offer input flight %q does not match airport slot state", flight.ID)
		}
	}
	for _, flight := range projected.Flights {
		inputFlight, exists := inputFlights[flight.ID]
		if flight.Slot != nil && (!exists || !slotsEqual(flight.Slot, inputFlight.CurrentSlot)) {
			return aman.AirportState{}, fmt.Errorf("airport slot flight %q is missing from queue offer input", flight.ID)
		}
	}
	for _, offer := range offers {
		index, exists := indexes[offer.FlightID]
		if !exists {
			return aman.AirportState{}, fmt.Errorf("queue offer flight %q does not match airport slot state", offer.FlightID)
		}
		projected.Flights[index].QueueOffers = append(projected.Flights[index].QueueOffers, offer)
	}
	for index := range projected.Flights {
		sort.Slice(projected.Flights[index].QueueOffers, func(i, j int) bool {
			a, b := projected.Flights[index].QueueOffers[i], projected.Flights[index].QueueOffers[j]
			if a.CandidateSlot.Sequence != b.CandidateSlot.Sequence {
				return a.CandidateSlot.Sequence < b.CandidateSlot.Sequence
			}
			return a.QueuePosition < b.QueuePosition
		})
	}
	if err := projected.Validate(); err != nil {
		return aman.AirportState{}, err
	}
	return projected, nil
}

func (c QueueOfferCalculation) project(state aman.AirportState, revision aman.SequenceRevision, at time.Time) (aman.AirportState, error) {
	input := cloneInput(c.Input)
	input.Revision = revision
	for index := range input.Flights {
		if input.Flights[index].CurrentSlot != nil {
			input.Flights[index].CurrentSlot.Revision = revision
		}
	}
	return ProjectQueueOffers(state, input, c.Config, at)
}

func queueEntries(revision aman.SequenceRevision, flights []preparedFlight) ([]queueEntry, error) {
	entries := make([]queueEntry, 0, len(flights))
	for _, flight := range flights {
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved || flight.CurrentSlot == nil {
			continue
		}
		slot := *flight.CurrentSlot
		if slot.Revision != revision || slot.Reason == "" {
			return nil, fmt.Errorf("flight %q has a stale or incomplete committed slot", flight.ID)
		}
		entries = append(entries, queueEntry{flight: flight, slot: slot})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].slot.Sequence != entries[j].slot.Sequence {
			return entries[i].slot.Sequence < entries[j].slot.Sequence
		}
		if !entries[i].slot.Time.Equal(entries[j].slot.Time) {
			return entries[i].slot.Time.Before(entries[j].slot.Time)
		}
		return entries[i].flight.ID < entries[j].flight.ID
	})
	for index := range entries {
		if index > 0 && (entries[index].slot.Sequence == entries[index-1].slot.Sequence || !entries[index].slot.Time.After(entries[index-1].slot.Time)) {
			return nil, fmt.Errorf("runway group %q has duplicate or unordered committed slots", entries[index].slot.RunwayGroupID)
		}
	}
	return entries, nil
}

func queueEligible(flight preparedFlight) bool {
	return (flight.State == aman.StateUnstable || flight.State == aman.StateStable) && flight.FreezeReason == aman.FreezeNone && flight.ManualOrder == nil
}

func crossesProtectedOrder(entries []queueEntry, candidateIndex, targetIndex int) bool {
	for index := candidateIndex + 1; index < targetIndex; index++ {
		flight := entries[index].flight
		if flight.FreezeReason != aman.FreezeNone || flight.ManualOrder != nil {
			return true
		}
	}
	return false
}

func slotsEqual(left, right *aman.Slot) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Time.Equal(right.Time) && left.RunwayGroupID == right.RunwayGroupID && left.Sequence == right.Sequence && left.Revision == right.Revision && left.Reason == right.Reason
}
