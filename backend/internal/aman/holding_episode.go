package aman

import (
	"fmt"
	"time"
)

// HoldingEpisode is observation evidence, not a clearance or predicted ETA.
// EndedAt remains durable so reinstating a clearance cannot resurrect its clock.
type HoldingEpisode struct {
	ID              string
	HoldingID       string
	Fix             string
	StartedAt       *time.Time
	Source          string
	PendingAt       *time.Time
	PendingSource   string
	LastObservation *SurveillanceFact
	EndedAt         *time.Time
}

type HoldingReleaseReservation struct {
	CommandID string
	EpisodeID string
	HoldingID string
	Level     int
	Rank      int
}

type RebuildDiagnostic struct {
	Code     string    `json:"code"`
	Callsign Callsign  `json:"callsign"`
	Blocker  *Callsign `json:"blocker,omitempty"`
	Message  string    `json:"message"`
}

type ArrivalSequenceRebuildOutcome struct {
	ReleaseOrder []Callsign          `json:"release_order"`
	Diagnostics  []RebuildDiagnostic `json:"diagnostics"`
}

func (e HoldingEpisode) Validate() error {
	if e.HoldingID == "" || e.Fix == "" {
		return invalid("holding episode requires identity and fix")
	}
	for _, at := range []*time.Time{e.StartedAt, e.PendingAt, e.EndedAt} {
		if at != nil {
			if err := requireUTCTime("holding episode time", *at); err != nil {
				return err
			}
		}
	}
	if e.StartedAt != nil && (e.ID == "" || (e.Source != "fix_crossing" && e.Source != "near_fix" && e.Source != "estimated_detection")) {
		return invalid("holding episode start requires identity and provenance")
	}
	if e.StartedAt != nil && e.EndedAt != nil && e.EndedAt.Before(*e.StartedAt) {
		return invalid("holding episode ends before start")
	}
	return nil
}

func HoldingEpisodeID(callsign Callsign, hold string, at time.Time) string {
	return fmt.Sprintf("%s/%s/%s", callsign, hold, at.UTC().Format(time.RFC3339Nano))
}

func EndHoldingEpisode(flight *AMANFlight, at time.Time) {
	// Accepted clearance and lifecycle termination may predate the latest radar
	// report, but must not end an episode that started after the termination.
	if e := flight.HoldingEpisode; e != nil && e.StartedAt != nil && at.Before(*e.StartedAt) {
		return
	}
	if flight.HoldingEpisode != nil && flight.HoldingEpisode.EndedAt == nil {
		episode := *flight.HoldingEpisode
		episode.EndedAt = &at
		flight.HoldingEpisode = &episode
	}
	flight.HoldingReleaseReservation = nil
}

// EndHoldingEpisodeFromObservation ignores delayed radar evidence without
// preventing an independently accepted clearance from terminating the episode.
func EndHoldingEpisodeFromObservation(flight *AMANFlight, at time.Time) {
	if e := flight.HoldingEpisode; e != nil && e.LastObservation != nil && e.LastObservation.ObservedAt != nil && at.Before(*e.LastObservation.ObservedAt) {
		return
	}
	EndHoldingEpisode(flight, at)
}

func ActiveHoldingEpisode(flight AMANFlight) *HoldingEpisode {
	if flight.State == StateLanded || flight.State == StateRemoved || flight.State == StatePlanned {
		return nil
	}
	e := flight.HoldingEpisode
	if e == nil || e.StartedAt == nil || e.EndedAt != nil || flight.SelectedHolding == nil || e.HoldingID != *flight.SelectedHolding {
		return nil
	}
	return e
}

func ValidHoldingReleaseReservation(flight AMANFlight) *HoldingReleaseReservation {
	r := flight.HoldingReleaseReservation
	e := ActiveHoldingEpisode(flight)
	if r == nil || e == nil || r.EpisodeID != e.ID || r.HoldingID != e.HoldingID || r.Rank < 1 ||
		flight.HoldingClearance == nil || flight.HoldingClearance.ClearedAltitude == nil ||
		int(*flight.HoldingClearance.ClearedAltitude) != r.Level {
		return nil
	}
	return r
}

// RebuiltHoldingPrecedes is shared by sequencing and publication. It describes
// release precedence, which need not be the order of landing slots.
func RebuiltHoldingPrecedes(older, later AMANFlight) (bool, bool) {
	// Keep a changed cleared level as a physical constraint until another
	// explicit rebuild or sequence decision replaces the reservation.
	ea, eb := ActiveHoldingEpisode(older), ActiveHoldingEpisode(later)
	ra, rb := older.HoldingReleaseReservation, later.HoldingReleaseReservation
	if ea != nil && eb != nil && ea.HoldingID == eb.HoldingID && ((ra != nil && ra.EpisodeID == ea.ID) || (rb != nil && rb.EpisodeID == eb.ID)) {
		if older.HoldingClearance == nil || older.HoldingClearance.ClearedAltitude == nil {
			return true, true
		}
		if later.HoldingClearance == nil || later.HoldingClearance.ClearedAltitude == nil {
			return false, true
		}
		if *older.HoldingClearance.ClearedAltitude != *later.HoldingClearance.ClearedAltitude {
			return *older.HoldingClearance.ClearedAltitude < *later.HoldingClearance.ClearedAltitude, true
		}
	}
	a, b := ValidHoldingReleaseReservation(older), ValidHoldingReleaseReservation(later)
	if a == nil || b == nil || a.CommandID != b.CommandID {
		return false, false
	}
	return a.Rank < b.Rank, true
}
