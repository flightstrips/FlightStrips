package aman

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRebuildReleaseConstraintUsesNewClearedLevelsWithoutResettingClock(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	hold := "H1"
	makeFlight := func(id Callsign, level int32, rank int) AMANFlight {
		return AMANFlight{Callsign: id, State: StateStable, SelectedHolding: &hold, HoldingEpisode: &HoldingEpisode{ID: string(id), HoldingID: hold, Fix: "FIX", StartedAt: &at, Source: "near_fix"},
			HoldingClearance:          &HoldingClearance{Hold: "FIX", HoldType: HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: at},
			HoldingReleaseReservation: &HoldingReleaseReservation{CommandID: "reset", EpisodeID: string(id), HoldingID: hold, Level: int(level), Rank: rank}}
	}
	lower, upper := makeFlight("LOW", 10000, 1), makeFlight("UP", 12000, 2)
	precedes, applicable := RebuiltHoldingPrecedes(lower, upper)
	require.True(t, applicable)
	require.True(t, precedes)
	newLevel := int32(8000)
	upper.ReplaceHoldingClearance(&HoldingClearance{Hold: "FIX", HoldType: HoldingClearanceEnroute, ClearedAltitude: &newLevel, ObservedAt: at.Add(time.Minute)})
	precedes, applicable = RebuiltHoldingPrecedes(upper, lower)
	require.True(t, applicable)
	require.True(t, precedes, "a changed CFL cannot publish the former upper aircraft behind its new higher-level neighbor")
	require.Equal(t, at, *upper.HoldingEpisode.StartedAt)
}

func TestHoldingEpisodeIgnoresOlderEndEvidence(t *testing.T) {
	at := time.Date(2026, 7, 20, 18, 15, 0, 500000000, time.UTC)
	flight := AMANFlight{HoldingEpisode: &HoldingEpisode{ID: "episode", HoldingID: "H1", Fix: "FIX", StartedAt: &at, Source: "near_fix", LastObservation: &SurveillanceFact{ObservedAt: &at}}}
	EndHoldingEpisode(&flight, at.Truncate(time.Second))
	require.Nil(t, flight.HoldingEpisode.EndedAt)
	require.NoError(t, flight.HoldingEpisode.Validate())
	EndHoldingEpisode(&flight, at)
	require.Equal(t, &at, flight.HoldingEpisode.EndedAt)
	require.NoError(t, flight.HoldingEpisode.Validate())
}

func TestHoldingEpisodeDistinguishesAcceptedTerminationFromDelayedRadar(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	latestRadar := at.Add(2 * time.Minute)
	flight := AMANFlight{HoldingEpisode: &HoldingEpisode{ID: "episode", HoldingID: "H1", Fix: "FIX", StartedAt: &at, Source: "near_fix", LastObservation: &SurveillanceFact{ObservedAt: &latestRadar}},
		HoldingReleaseReservation: &HoldingReleaseReservation{CommandID: "rebuild", EpisodeID: "episode", HoldingID: "H1", Rank: 1}}
	endedAt := at.Add(time.Minute)
	EndHoldingEpisodeFromObservation(&flight, endedAt)
	require.Nil(t, flight.HoldingEpisode.EndedAt)
	require.NotNil(t, flight.HoldingReleaseReservation)
	EndHoldingEpisode(&flight, endedAt)
	require.Equal(t, &endedAt, flight.HoldingEpisode.EndedAt)
	require.Nil(t, flight.HoldingReleaseReservation)
	require.NoError(t, flight.HoldingEpisode.Validate())
}
