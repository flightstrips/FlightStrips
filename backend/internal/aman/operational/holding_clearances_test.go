package operational

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/holdingclearance"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestHoldingClearancePublishesEATWithoutWaitingForReconciliation(t *testing.T) {
	service, state, now := holdingClearanceRefreshFixture(t)
	flight := state.Flights[0]
	slot := *flight.Slot
	positionAt := *flight.LatestObservation.Surveillance.ObservedAt
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	clearances, err := holdingclearance.New(holdingclearance.Dependencies{
		Repository: repository, Publisher: publisher, Planner: service,
	})
	require.NoError(t, err)
	cfl := int32(10000)
	require.NoError(t, clearances.ObserveHoldingClearance(context.Background(), aman.HoldingClearanceFact{
		Callsign: flight.Callsign, Destination: state.Airport, Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute,
		ClearedAltitude: &cfl, ObservedAt: now,
	}))
	require.Len(t, publisher.states, 1, "the clearance and its EAT must be published together")
	model := holdingclearance.BuildReadModel(publisher.states[0])
	require.Len(t, model.Entries, 1)
	require.NotNil(t, model.Entries[0].EAT, "the first clearance publication must already contain the EAT: warnings=%+v", model.Warnings)
	require.Empty(t, model.Warnings)
	updated := repository.state.Flights[0]
	require.NoError(t, updated.Prediction.HoldingPlan.Validate())
	require.Equal(t, slot.Time, updated.Slot.Time)
	require.Equal(t, state.Revision+1, repository.state.Revision)
	require.Equal(t, positionAt, *updated.LatestObservation.Surveillance.ObservedAt, "replanning must not invent a newer position")
}

func TestHoldingClearanceSurvivesUnavailableImmediateCalculation(t *testing.T) {
	for _, scenario := range []string{"stale position", "missing observation", "navigation unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			service, state, now := holdingClearanceRefreshFixture(t)
			switch scenario {
			case "stale position":
				stale := now.Add(-3 * time.Minute)
				state.Flights[0].LatestObservation.Surveillance.ObservedAt = &stale
			case "missing observation":
				state.Flights[0].LatestObservation = nil
			case "navigation unavailable":
				service.deps.Geometry = unavailableHoldingGeometry{service.deps.Geometry}
			}
			repository := &memoryRepository{state: state, has: true}
			publisher := &recordingPublisher{}
			clearances, err := holdingclearance.New(holdingclearance.Dependencies{
				Repository: repository, Publisher: publisher, Planner: service,
			})
			require.NoError(t, err)
			require.NoError(t, clearances.ObserveHoldingClearance(context.Background(), aman.HoldingClearanceFact{
				Callsign: state.Flights[0].Callsign, Destination: state.Airport, Hold: "TESPI",
				HoldType: aman.HoldingClearanceEnroute, ObservedAt: now,
			}))
			require.Len(t, publisher.states, 1)
			require.Equal(t, "TESPI", repository.state.Flights[0].HoldingClearance.Hold)
			require.Nil(t, repository.state.Flights[0].Prediction.HoldingPlan, "unavailable inputs must not invent an EAT")
			require.Equal(t, state.Flights[0].Slot.Time, repository.state.Flights[0].Slot.Time)
		})
	}
}

type unavailableHoldingGeometry struct{ GeometryCache }

func (g unavailableHoldingGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return navdata.ActiveGeometrySnapshot{}, errors.New("navigation unavailable")
}

func holdingClearanceRefreshFixture(t *testing.T) (*Service, aman.AirportState, time.Time) {
	t.Helper()
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	hold := navdata.HoldingID("TESPI-HOLD")
	geometry.path.HoldingIDs = []navdata.HoldingID{hold}
	service.deps.Geometry = holdingReleaseGeometry{terminalIdentityGeometry: geometry,
		holding: navdata.HoldingPattern{ID: hold, Fix: "TESPI"}}
	service.deps.Terminal.Paths[0].SelectedHolding = hold
	service.deps.Now = func() time.Time { return now }

	flight := state.Flights[0]
	flight.Slot.Time = now.Add(30 * time.Minute)
	observation := *flight.LatestObservation
	position := *observation.Surveillance
	position.LatitudeDegrees, position.LongitudeDegrees = 55.14, 12.14
	positionAt := now
	position.ObservedAt = &positionAt
	observation.Surveillance, observation.ReconciledAt = &position, now
	var err error
	flight, err = service.reconcileFlight(context.Background(), state, flight, observation, now)
	require.NoError(t, err)
	require.Nil(t, flight.Prediction.HoldingFixETA, "the uncleared route has already passed the holding fix")
	require.Nil(t, flight.Prediction.HoldingPlan)
	now = now.Add(time.Second)
	state.Flights = []aman.AMANFlight{flight}
	return service, state, now
}
