package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/trajectory"
	"time"
)

func observeHoldingEpisode(flight *aman.AMANFlight, projection trajectory.Result, observation aman.FlightObservation, now time.Time) {
	if observation.SourceStatus != aman.DataFresh || observation.Surveillance == nil || observation.Surveillance.ObservedAt == nil {
		return
	}
	fact := observation.Surveillance
	at := *fact.ObservedAt
	if at.After(now) || now.Sub(at) > tmaSurveillanceFresh {
		return
	}
	if projection.SelectedHolding == nil || projection.HoldingFixPosition == nil {
		if operationalHoldingClearanceFix(*flight) == "" {
			aman.EndHoldingEpisodeFromObservation(flight, at)
		}
		return
	}
	hold := string(projection.SelectedHolding.ID)
	fix := string(projection.SelectedHolding.Fix)
	old := flight.HoldingEpisode
	if old != nil && old.EndedAt != nil && !at.After(*old.EndedAt) {
		return
	}
	first := old == nil
	episode := aman.HoldingEpisode{HoldingID: hold, Fix: fix}
	if old != nil && old.HoldingID == hold && old.Fix == fix && old.EndedAt == nil {
		episode = *old
	}
	if episode.LastObservation != nil && episode.LastObservation.ObservedAt != nil && !at.After(*episode.LastObservation.ObservedAt) {
		return
	}
	entry, source := trajectory.HoldingEntry(episode.LastObservation, *fact, *projection.HoldingFixPosition)
	if episode.PendingAt == nil && entry != nil {
		episode.PendingAt = entry
		episode.PendingSource = source
	}
	physical := flight.HoldingStack != nil && flight.HoldingStack.Confirmed && flight.HoldingStack.HoldingID == hold
	cleared := string(operationalHoldingClearanceFix(*flight)) == fix
	if episode.StartedAt == nil && episode.PendingAt != nil && (cleared || physical && projection.HoldingCandidate != nil) {
		episode.StartedAt = episode.PendingAt
		episode.Source = episode.PendingSource
	}
	// Only restored, already confirmed holding may seed a missed-entry estimate.
	// Watching an approaching aircraft become a footprint candidate is not entry.
	if episode.StartedAt == nil && first && physical {
		episode.StartedAt = &at
		episode.Source = "estimated_detection"
	}
	if episode.StartedAt != nil {
		episode.ID = aman.HoldingEpisodeID(flight.Callsign, hold, *episode.StartedAt)
	}
	copy := *fact
	episode.LastObservation = &copy
	flight.HoldingEpisode = &episode
	if r := flight.HoldingReleaseReservation; r != nil && (episode.ID != r.EpisodeID || episode.HoldingID != r.HoldingID) {
		flight.HoldingReleaseReservation = nil
	}
}

func holdingReleaseRank(flight aman.AMANFlight) *int {
	if r := aman.ValidHoldingReleaseReservation(flight); r != nil {
		rank := r.Rank
		return &rank
	}
	return nil
}
