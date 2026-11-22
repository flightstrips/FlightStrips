package aman

import (
	"cmp"
	"strings"
	"time"
)

// StripRevision identifies a persisted strip and its version. The row ID
// distinguishes a replacement strip whose version starts over.
type StripRevision struct {
	ID      int32
	Version int32
}

func (r StripRevision) Known() bool {
	return r.ID > 0 && r.Version > 0
}

func (r StripRevision) compare(other StripRevision) int {
	if r.ID != other.ID {
		return cmp.Compare(r.ID, other.ID)
	}
	return cmp.Compare(r.Version, other.Version)
}

func (r StripRevision) IsOlderThan(other StripRevision) bool {
	return r.Known() && other.Known() && r.compare(other) < 0
}

// LatestStripClearanceTime is the newest timestamp of the retained strip fact.
func (clearance HoldingClearance) LatestStripClearanceTime() time.Time {
	if clearance.ArrivalClearedAltitudeAt.After(clearance.ObservedAt) {
		return clearance.ArrivalClearedAltitudeAt
	}
	return clearance.ObservedAt
}

// PredatesStripSnapshot uses database order when both snapshots are versioned.
func (clearance *HoldingClearance) PredatesStripSnapshot(revision StripRevision, observedAt time.Time) bool {
	if clearance == nil {
		return true
	}
	if revision.Known() {
		if clearance.StripRevision.Known() {
			return revision.compare(clearance.StripRevision) > 0
		}
		if clearance.PreviousStripRevision.Known() && revision.compare(clearance.PreviousStripRevision) <= 0 {
			return false
		}
	}
	return observedAt.After(clearance.LatestStripClearanceTime())
}

// ConflictsWithNewerCFL rejects older holding data when a newer strip shows a
// different CFL. A matching position report can confirm the level before the
// holding plan has been updated.
func (clearance *HoldingClearance) ConflictsWithNewerCFL(observation *FlightObservation) bool {
	if observation == nil || !clearance.StripRevision.IsOlderThan(observation.ClearedAltitudeStripRevision) {
		return false
	}
	if clearance.StripRevision.ID != observation.ClearedAltitudeStripRevision.ID {
		return true
	}
	altitude := clearance.ArrivalClearedAltitude
	if altitude == nil {
		altitude = clearance.ClearedAltitude
	}
	clearedAltitude := 0
	if altitude != nil {
		clearedAltitude = int(*altitude)
	}
	observedAltitude := 0
	if observation.ClearedAltitude != nil {
		observedAltitude = *observation.ClearedAltitude
	}
	return clearedAltitude != observedAltitude
}

// ReplaceHoldingClearance records the actual end of a holding clearance episode.
// Repeated blank strip snapshots must not supersede fresh physical holding evidence.
// Callers only supply accepted, newer clearance facts.
func (flight *AMANFlight) ReplaceHoldingClearance(clearance *HoldingClearance) {
	active := func(value *HoldingClearance) bool {
		return value != nil && value.HoldType == HoldingClearanceEnroute && strings.TrimSpace(value.Hold) != ""
	}
	if active(flight.HoldingClearance) && clearance != nil && (!active(clearance) || clearance.Hold != flight.HoldingClearance.Hold) {
		EndHoldingEpisode(flight, clearance.ObservedAt)
	}
	if active(clearance) {
		flight.HoldingClearanceCanceledAt = nil
	} else if active(flight.HoldingClearance) && clearance != nil && !clearance.ObservedAt.IsZero() {
		at := clearance.ObservedAt
		flight.HoldingClearanceCanceledAt = &at
	}
	// Some full-sync snapshots do not carry a persisted version. Retain the
	// last known revision so a delayed dedicated event cannot replay it.
	if clearance != nil && !clearance.StripRevision.Known() && flight.HoldingClearance != nil {
		previousRevision := flight.HoldingClearance.StripRevision
		if !previousRevision.Known() {
			previousRevision = flight.HoldingClearance.PreviousStripRevision
		}
		if previousRevision.Known() {
			replacement := *clearance
			replacement.PreviousStripRevision = previousRevision
			clearance = &replacement
		}
	}
	flight.HoldingClearance = clearance
}
