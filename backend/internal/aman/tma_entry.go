package aman

import "time"

// ObserveTMAContainment advances the persisted TMA cursor after a caller has
// accepted a fresh surveillance observation. It deliberately does not inspect
// geometry, source freshness, lifecycle, slots, or operational freeze fields;
// those policies belong to the later integration slice.
//
// The first observation establishes a baseline only. A subsequent
// outside-to-inside edge triggers capture once for the current arrival episode.
// Exit updates the cursor but never clears the sticky trigger.
func ObserveTMAContainment(previous *TMAEntryState, contained bool, observedAt time.Time) (TMAEntryState, bool, error) {
	containment := TMAOutside
	if contained {
		containment = TMAInside
	}
	next := TMAEntryState{
		LastContainment: containment,
		LastObservedAt:  observedAt,
	}
	if err := next.Validate(); err != nil {
		return TMAEntryState{}, false, err
	}
	if previous == nil {
		return next, false, nil
	}
	if err := previous.Validate(); err != nil {
		return TMAEntryState{}, false, err
	}
	if observedAt.Before(previous.LastObservedAt) {
		return TMAEntryState{}, false, invalid("TMA containment observation precedes the persisted cursor")
	}
	if observedAt.Equal(previous.LastObservedAt) {
		if containment != previous.LastContainment {
			return TMAEntryState{}, false, invalid("TMA containment conflicts at the persisted observation time")
		}
		return *previous, false, nil
	}

	triggered := !previous.FreezeTriggered && previous.LastContainment == TMAOutside && containment == TMAInside
	next.FreezeTriggered = previous.FreezeTriggered || triggered
	return next, triggered, nil
}
