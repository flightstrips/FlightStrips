package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

func TestBlankStripSnapshotsPreservePhysicalHolding(t *testing.T) {
	base := time.Date(2026, time.October, 5, 19, 0, 0, 0, time.UTC)
	for _, fix := range []string{"ROSBI", "OLPIB", "LUGAS", "TIDVU", "ERNOV"} {
		t.Run(fix, func(t *testing.T) {
			hold := fix + "-HOLD"
			flight := operationalFlight("PHYSICAL", "ARRIVAL-22L", fix, "M", base.Add(20*time.Minute))
			flight.SelectedHolding = &hold
			flight.Prediction.HoldingFixETA = &base
			applyBlankStrip := func(at time.Time) {
				observation := aman.FlightObservation{Callsign: flight.Callsign, SourceStatus: aman.DataStale,
					HoldingClearance: normalizedStripHoldingClearance(&models.Strip{}, at)}
				var err error
				flight, err = (&Service{}).reconcileFlight(context.Background(), aman.AirportState{}, flight, observation, at)
				require.NoError(t, err)
			}
			projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: navdata.HoldingID(hold), Fix: navdata.FixID(fix)},
				HoldingCandidate: &trajectory.HoldingCandidate{HoldingID: navdata.HoldingID(hold)}}
			for i := 0; i < 3; i++ {
				positionAt := base.Add(time.Duration(i) * time.Minute)
				// The ordinary strip snapshot is newer than its surveillance track.
				applyBlankStrip(positionAt.Add(30 * time.Second))
				flight.Prediction.InputObservedAt = positionAt
				flight.HoldingStack = updateFlightHoldingStack(flight, projection, positionAt)
			}
			require.Nil(t, flight.HoldingClearanceCanceledAt)
			require.True(t, flight.HoldingStack.Confirmed)
			require.Equal(t, base, flight.HoldingStack.FirstObservedAt)
			require.Equal(t, hold, holdingStackID(flight))
			require.Equal(t, &base, holdingQueueTime(flight, []aman.AMANFlight{flight}))

			flight.ReplaceHoldingClearance(&aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ObservedAt: base.Add(3 * time.Minute)})
			canceledAt := base.Add(4 * time.Minute)
			applyBlankStrip(canceledAt)
			require.Equal(t, &canceledAt, flight.HoldingClearanceCanceledAt)
			require.Empty(t, holdingStackID(flight), "actual cancellation supersedes the old position")
			for i := 5; i < 7; i++ {
				positionAt := base.Add(time.Duration(i) * time.Minute)
				applyBlankStrip(positionAt.Add(30 * time.Second))
				flight.Prediction.InputObservedAt = positionAt
				flight.HoldingStack = updateFlightHoldingStack(flight, projection, positionAt)
			}
			require.Equal(t, &canceledAt, flight.HoldingClearanceCanceledAt, "blank snapshots retain the original cancellation time")
			require.True(t, flight.HoldingStack.Confirmed, "fresh positions can confirm a new physical holding episode")
			require.Equal(t, hold, holdingStackID(flight))
			require.Equal(t, base.Add(5*time.Minute), *holdingQueueTime(flight, []aman.AMANFlight{flight}))
		})
	}
}

func TestClearedHoldingKeepsQueueAdmissionAcrossRacetrackLaps(t *testing.T) {
	base := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	id := navdata.HoldingID("LUGAS-HOLD")
	flight := aman.AMANFlight{
		HoldingClearance: &aman.HoldingClearance{Hold: "LUGAS", HoldType: aman.HoldingClearanceEnroute},
		HoldingStack: &aman.HoldingStackState{HoldingID: string(id), FirstObservedAt: base,
			CandidateObservedAt: base, Confirmed: true, ConsecutiveObservations: 2},
	}
	projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: id, Fix: "LUGAS"}}
	retained := updateFlightHoldingStack(flight, projection, base.Add(4*time.Minute))
	require.Equal(t, flight.HoldingStack, retained)
	require.NotSame(t, flight.HoldingStack, retained)
	flight.HoldingStack = retained
	projection.HoldingCandidate = &trajectory.HoldingCandidate{HoldingID: id}
	resumed := updateFlightHoldingStack(flight, projection, base.Add(5*time.Minute))
	require.True(t, resumed.Confirmed)
	require.Equal(t, base, resumed.FirstObservedAt, "returning from a cleared outbound leg is not a new holding arrival")
	projection.HoldingCandidate = nil
	flight.HoldingStack = resumed
	flight.HoldingClearance = nil
	require.Nil(t, updateFlightHoldingStack(flight, projection, base.Add(6*time.Minute)))
	flight.HoldingClearance = &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute}
	require.Nil(t, updateFlightHoldingStack(flight, projection, base.Add(6*time.Minute)))
	flight.HoldingClearance.Hold = "LUGAS"
	flight.HoldingStack = nil
	require.Nil(t, updateFlightHoldingStack(flight, projection, base), "a clearance alone must not create confirmed physical stack traffic")
}

func TestClearedHoldingRetainsFirstSightingBeforeConfirmation(t *testing.T) {
	base := time.Date(2026, time.October, 5, 19, 0, 0, 0, time.UTC)
	for _, fix := range []string{"ROSBI", "OLPIB", "LUGAS", "TIDVU", "ERNOV"} {
		t.Run(fix, func(t *testing.T) {
			id := navdata.HoldingID(fix + "-HOLD")
			flight := aman.AMANFlight{HoldingClearance: &aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute}}
			projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: id, Fix: navdata.FixID(fix)},
				HoldingCandidate: &trajectory.HoldingCandidate{HoldingID: id}}
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base)
			require.False(t, flight.HoldingStack.Confirmed)
			projection.HoldingCandidate = nil
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(time.Minute))
			require.NotNil(t, flight.HoldingStack, "an outbound leg must not erase a cleared aircraft's queue admission")
			require.False(t, flight.HoldingStack.Confirmed, "retaining admission must not fabricate confirmed physical holding")
			projection.HoldingCandidate = &trajectory.HoldingCandidate{HoldingID: id}
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(2*time.Minute))
			require.False(t, flight.HoldingStack.Confirmed, "a sighting after leaving the footprint starts a new confirmation streak")
			projection.HoldingCandidate = nil
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(3*time.Minute))
			projection.HoldingCandidate = &trajectory.HoldingCandidate{HoldingID: id}
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(5*time.Minute))
			require.Equal(t, base, flight.HoldingStack.FirstObservedAt)
			require.False(t, flight.HoldingStack.Confirmed, "widely separated sightings still require consecutive confirmation")
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(6*time.Minute))
			require.True(t, flight.HoldingStack.Confirmed)
			require.Equal(t, base, flight.HoldingStack.FirstObservedAt)
			flight.ReplaceHoldingClearance(&aman.HoldingClearance{ObservedAt: base.Add(7 * time.Minute)})
			flight.HoldingStack = updateFlightHoldingStack(flight, projection, base.Add(8*time.Minute))
			require.Equal(t, base.Add(8*time.Minute), flight.HoldingStack.FirstObservedAt, "cancellation ends the old admission")
		})
	}
}

func TestCanceledHoldingWithStalePositionDoesNotBlockOtherReleases(t *testing.T) {
	base := time.Date(2026, time.October, 5, 19, 0, 0, 0, time.UTC)
	for _, fix := range []string{"ROSBI", "OLPIB", "LUGAS", "TIDVU", "ERNOV"} {
		t.Run(fix, func(t *testing.T) {
			group, hold := aman.RunwayGroupID("ARRIVAL-22L"), fix+"-HOLD"
			old := operationalFlight("RELEASED", group, fix, "M", base.Add(20*time.Minute))
			old.SelectedHolding = &hold
			old.DataStatus = aman.DataStale
			old.HoldingStack = &aman.HoldingStackState{HoldingID: hold, Confirmed: true, FirstObservedAt: base.Add(-10 * time.Minute)}
			old.HoldingClearance = &aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute}
			old.ReplaceHoldingClearance(&aman.HoldingClearance{ObservedAt: base.Add(time.Minute)})
			entry := base.Add(-10 * time.Minute)
			old.Prediction.HoldingFixETA, old.Prediction.InputObservedAt, old.Prediction.GeneratedAt = &entry, base, base.Add(2*time.Minute)
			old.Prediction.RawTETA = base.Add(20 * time.Minute)
			state := aman.AirportState{Flights: []aman.AMANFlight{old}}
			for i, callsign := range []string{"FIRST", "SECOND"} {
				flight := operationalFlight(callsign, group, fix, "M", base.Add(15*time.Minute))
				flight.SelectedHolding = &hold
				flight.HoldingClearance = &aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute}
				entered := base.Add(time.Duration(i) * time.Minute)
				flight.HoldingStack = &aman.HoldingStackState{HoldingID: hold, Confirmed: true, FirstObservedAt: entered}
				flight.Prediction.HoldingFixETA = &entered
				flight.Prediction.RawTETA = base.Add(15 * time.Minute)
				flight.Slot = &aman.Slot{Time: base.Add(time.Duration(25+i*3) * time.Minute), RunwayGroupID: group}
				state.Flights = append(state.Flights, flight)
			}
			(&Service{}).refreshHoldingPlans(&state)
			require.Nil(t, holdingQueueTime(state.Flights[0], state.Flights))
			require.Empty(t, holdingStackID(state.Flights[0]))
			require.Nil(t, activeHoldingSince(state.Flights[0]))
			for i := 1; i < len(state.Flights); i++ {
				require.NotNil(t, state.Flights[i].Prediction.HoldingPlan, "released stale traffic must not cascade unavailable EATs")
				require.Nil(t, state.Flights[i].Prediction.HoldingPlanBlockedBy)
				require.Equal(t, base.Add(time.Duration(25+(i-1)*3)*time.Minute), state.Flights[i].Slot.Time)
			}
			// A fresh position after cancellation may establish physical holding
			// again. Data loss alone must never be treated as cancellation.
			state.Flights[0].Prediction.InputObservedAt = base.Add(2 * time.Minute)
			require.NotNil(t, holdingQueueTime(state.Flights[0], state.Flights))
			state.Flights[0].ReplaceHoldingClearance(&aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ObservedAt: base.Add(time.Minute)})
			state.Flights[0].Prediction.InputObservedAt = base
			(&Service{}).refreshHoldingPlans(&state)
			require.Nil(t, state.Flights[1].Prediction.HoldingPlan, "a genuinely held older aircraft still blocks an unsafe release")
		})
	}
}
