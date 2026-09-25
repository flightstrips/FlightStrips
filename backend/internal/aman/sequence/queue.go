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

// VacancyPromotion records an automatic move from a flight's committed slot
// into an earlier queue opportunity that became vacant.
type VacancyPromotion struct {
	FlightID aman.FlightID
	From     aman.Slot
	To       aman.Slot
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

// GenerateWithVacancyPromotions consumes revision-bound queue offers whose
// candidate slots are no longer occupied and also compacts Stable flights into
// any earlier legal runway-grid opportunity. Offers remain evidence of queue
// order for Unstable traffic; a Stable flight no longer depends on a prior
// offer for a slot which is already empty. Every promotion is checked again
// for lifecycle, TETA, runway, manual/freeze boundaries, Stable order, WTC,
// and same-STAR spacing before it is applied.
func GenerateWithVacancyPromotions(input Input, offers []aman.QueueOffer, at time.Time) (Result, []VacancyPromotion, error) {
	if input.Revision == 0 {
		return Result{}, nil, fmt.Errorf("vacancy promotion requires a committed airport revision")
	}
	if !validUTC(at) {
		return Result{}, nil, fmt.Errorf("vacancy promotion time must be UTC")
	}
	policies, err := preparePolicies(input.Policies)
	if err != nil {
		return Result{}, nil, err
	}
	prepared, err := prepareFlights(input.Flights, policies)
	if err != nil {
		return Result{}, nil, err
	}
	baseline, err := generate(input, nil)
	if err != nil || baseline.HasConflicts() {
		return baseline, nil, err
	}

	canonical := append([]aman.QueueOffer(nil), offers...)
	sort.Slice(canonical, func(i, j int) bool {
		a, b := canonical[i], canonical[j]
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

	promotionSlots := make(map[aman.FlightID]aman.Slot)
	promotions := make([]VacancyPromotion, 0)
	groupIDs := make([]aman.RunwayGroupID, 0, len(prepared))
	for group := range prepared {
		groupIDs = append(groupIDs, group)
	}
	sort.Slice(groupIDs, func(i, j int) bool { return groupIDs[i] < groupIDs[j] })
	for _, group := range groupIDs {
		flights := prepared[group]
		entries, entryErr := baselineQueueEntries(input.Revision, group, flights, baseline)
		if entryErr != nil {
			return Result{}, nil, entryErr
		}
		policy := policies[group]
		for _, offer := range canonical {
			if offer.RunwayGroupID != group || offer.AirportRevision != input.Revision || offer.CandidateSlot.Revision != input.Revision ||
				offer.CandidateSlot.RunwayGroupID != group || !offer.ExpiresAt.After(at) {
				continue
			}
			if _, alreadyPromoted := promotionSlots[offer.FlightID]; alreadyPromoted {
				continue
			}
			if queueSlotOccupied(entries, offer.CandidateSlot) {
				continue
			}
			targetIndex := queueEntryIndex(entries, offer.FlightID)
			if targetIndex < 0 {
				continue
			}
			target := entries[targetIndex]
			if !queueEligible(target.flight) || !offer.CandidateSlot.Time.Before(target.slot.Time) || !isGridSlot(policy, offer.CandidateSlot.Time) ||
				offer.CandidateSlot.Time.Before(promotionLowerBound(policy, target.flight, at)) || crossesProtectedTime(entries, offer.CandidateSlot.Time, target.slot.Time) ||
				crossesStableOrder(entries, target.flight, offer.CandidateSlot.Time) {
				continue
			}
			remaining := make([]allocatedEntry, 0, len(entries)-1)
			for index, entry := range entries {
				if index == targetIndex {
					continue
				}
				remaining = append(remaining, allocatedEntry{flight: entry.flight, time: entry.slot.Time, reason: CandidateReason(entry.slot.Reason)})
			}
			valid, _, _ := placement(policy, remaining, target.flight, offer.CandidateSlot.Time)
			if !valid {
				continue
			}

			if target.flight.CurrentSlot == nil {
				continue
			}
			from, to := *target.flight.CurrentSlot, offer.CandidateSlot
			promotionSlots[target.flight.ID] = to
			promotions = append(promotions, VacancyPromotion{FlightID: target.flight.ID, From: from, To: to})
			entries[targetIndex].slot = to
			sort.Slice(entries, func(i, j int) bool {
				if !entries[i].slot.Time.Equal(entries[j].slot.Time) {
					return entries[i].slot.Time.Before(entries[j].slot.Time)
				}
				if entries[i].slot.Sequence != entries[j].slot.Sequence {
					return entries[i].slot.Sequence < entries[j].slot.Sequence
				}
				return entries[i].flight.ID < entries[j].flight.ID
			})
		}
		promotions = compactStableFlights(policy, entries, promotionSlots, promotions, at)
	}

	result, err := generate(input, promotionSlots)
	if err != nil {
		return Result{}, nil, err
	}
	if result.HasConflicts() {
		fallback, fallbackErr := Generate(input)
		return fallback, nil, fallbackErr
	}
	for index := range promotions {
		for _, entry := range result.Entries {
			if entry.FlightID != promotions[index].FlightID {
				continue
			}
			promotions[index].To = aman.Slot{
				Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence,
				Revision: input.Revision, Reason: string(entry.Reason),
			}
			break
		}
	}
	return result, promotions, nil
}

// compactStableFlights moves Stable flights monotonically earlier without
// changing their committed relative order. A Superstable flight may accept an
// earlier vacancy, but it cannot cross another freeze or manual-order boundary.
// TMA and manual freezes remain immovable. The current slot is always retained
// when no earlier opportunity satisfies the complete placement policy.
func compactStableFlights(policy preparedPolicy, entries []queueEntry, promotionSlots map[aman.FlightID]aman.Slot, promotions []VacancyPromotion, at time.Time) []VacancyPromotion {
	stableIDs := make([]aman.FlightID, 0, len(entries))
	for _, entry := range entries {
		if stablePromotionEligible(entry.flight) {
			stableIDs = append(stableIDs, entry.flight.ID)
		}
	}

	promotionIndexes := make(map[aman.FlightID]int, len(promotions))
	for index := range promotions {
		promotionIndexes[promotions[index].FlightID] = index
	}
	for _, flightID := range stableIDs {
		targetIndex := queueEntryIndex(entries, flightID)
		if targetIndex < 0 {
			continue
		}
		target := entries[targetIndex]
		remaining := make([]allocatedEntry, 0, len(entries)-1)
		for index, entry := range entries {
			if index == targetIndex {
				continue
			}
			remaining = append(remaining, allocatedEntry{flight: entry.flight, time: entry.slot.Time, reason: CandidateReason(entry.slot.Reason)})
		}

		lower := promotionLowerBound(policy, target.flight, at)
		candidate, ok := nextGridAtOrAfter(policy, lower)
		for ok && candidate.Before(target.slot.Time) {
			if !crossesProtectedTime(entries, candidate, target.slot.Time) && !crossesStableOrder(entries, target.flight, candidate) {
				valid, _, later := placement(policy, remaining, target.flight, candidate)
				if valid {
					to := aman.Slot{
						Time: candidate, RunwayGroupID: target.slot.RunwayGroupID,
						Sequence: target.slot.Sequence, Revision: target.slot.Revision,
						Reason: string(ReasonQueuePromotion),
					}
					promotionSlots[flightID] = to
					if promotionIndex, promoted := promotionIndexes[flightID]; promoted {
						promotions[promotionIndex].To = to
					} else {
						promotionIndexes[flightID] = len(promotions)
						promotions = append(promotions, VacancyPromotion{FlightID: flightID, From: *target.flight.CurrentSlot, To: to})
					}
					entries[targetIndex].slot = to
					sort.Slice(entries, func(i, j int) bool {
						if !entries[i].slot.Time.Equal(entries[j].slot.Time) {
							return entries[i].slot.Time.Before(entries[j].slot.Time)
						}
						if entries[i].slot.Sequence != entries[j].slot.Sequence {
							return entries[i].slot.Sequence < entries[j].slot.Sequence
						}
						return entries[i].flight.ID < entries[j].flight.ID
					})
					break
				}
				if later.After(candidate) {
					candidate, ok = nextGridAtOrAfter(policy, later)
					continue
				}
			}
			candidate, ok = nextGridAtOrAfter(policy, candidate.Add(time.Nanosecond))
		}
	}
	return promotions
}

func stablePromotionEligible(flight preparedFlight) bool {
	if flight.State != aman.StateStable || flight.ManualOrder != nil || flight.CurrentSlot == nil {
		return false
	}
	switch flight.FreezeReason {
	case aman.FreezeNone:
		return flight.ProtectCurrentSlot
	case aman.FreezeSuperstable:
		return flight.CapturedSlot != nil
	default:
		return false
	}
}

func promotionLowerBound(policy preparedPolicy, flight preparedFlight, at time.Time) time.Time {
	lower := flight.OperationalTETA.Add(-policy.EarlyTolerance)
	if !lower.After(at) {
		lower = at.Add(time.Nanosecond)
	}
	if flight.PromotionNotBefore != nil && lower.Before(*flight.PromotionNotBefore) {
		lower = *flight.PromotionNotBefore
	}
	return lower
}

// Stable order is committed by sequence number. The nearest time neighbor may
// be Unstable, so checking only placement's adjacent neighbors is insufficient.
func crossesStableOrder(entries []queueEntry, target preparedFlight, candidate time.Time) bool {
	if target.stableOrder == nil {
		return false
	}
	for _, entry := range entries {
		if entry.flight.ID == target.ID || entry.flight.stableOrder == nil {
			continue
		}
		if *entry.flight.stableOrder < *target.stableOrder && !candidate.After(entry.slot.Time) {
			return true
		}
		if *entry.flight.stableOrder > *target.stableOrder && !candidate.Before(entry.slot.Time) {
			return true
		}
	}
	return false
}

func baselineQueueEntries(revision aman.SequenceRevision, group aman.RunwayGroupID, flights []preparedFlight, baseline Result) ([]queueEntry, error) {
	byID := make(map[aman.FlightID]preparedFlight, len(flights))
	for _, flight := range flights {
		byID[flight.ID] = flight
	}
	entries := make([]queueEntry, 0, len(flights))
	for _, candidate := range baseline.Entries {
		if candidate.RunwayGroupID != group {
			continue
		}
		flight, exists := byID[candidate.FlightID]
		if !exists {
			return nil, fmt.Errorf("baseline flight %q is missing from runway group %q", candidate.FlightID, group)
		}
		entries = append(entries, queueEntry{flight: flight, slot: aman.Slot{
			Time: candidate.Time, RunwayGroupID: group, Sequence: candidate.Sequence,
			Revision: revision, Reason: string(candidate.Reason),
		}})
	}
	return entries, nil
}

func queueEntryIndex(entries []queueEntry, flightID aman.FlightID) int {
	for index := range entries {
		if entries[index].flight.ID == flightID {
			return index
		}
	}
	return -1
}

func queueSlotOccupied(entries []queueEntry, slot aman.Slot) bool {
	for _, entry := range entries {
		if entry.slot.Time.Equal(slot.Time) {
			return true
		}
	}
	return false
}

func crossesProtectedTime(entries []queueEntry, candidateTime, targetTime time.Time) bool {
	for _, entry := range entries {
		if !entry.slot.Time.After(candidateTime) || !entry.slot.Time.Before(targetTime) {
			continue
		}
		if entry.flight.FreezeReason != aman.FreezeNone || entry.flight.ManualOrder != nil {
			return true
		}
	}
	return false
}

func isGridSlot(policy preparedPolicy, candidate time.Time) bool {
	grid, ok := previousGridAtOrBefore(policy, candidate)
	return ok && grid.Equal(candidate)
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
				valid, _, _ := queuePlacement(policy, remaining, target.flight, candidate.slot.Time)
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
		if flight.SequenceDisposition.Participates() && flight.Slot != nil && (!exists || !slotsEqual(flight.Slot, inputFlight.CurrentSlot)) {
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
