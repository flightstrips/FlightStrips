package aman

import "strings"

// ReplaceHoldingClearance records the actual end of a holding clearance episode.
// Repeated blank strip snapshots must not supersede fresh physical holding evidence.
// Callers only supply accepted, newer clearance facts.
func (flight *AMANFlight) ReplaceHoldingClearance(clearance *HoldingClearance) {
	active := func(value *HoldingClearance) bool {
		return value != nil && value.HoldType == HoldingClearanceEnroute && strings.TrimSpace(value.Hold) != ""
	}
	if active(clearance) {
		flight.HoldingClearanceCanceledAt = nil
	} else if active(flight.HoldingClearance) && clearance != nil && !clearance.ObservedAt.IsZero() {
		at := clearance.ObservedAt
		flight.HoldingClearanceCanceledAt = &at
	}
	flight.HoldingClearance = clearance
}
