package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendAllocatedStandToEuroscope_ArrivalWaitsForConfirmedAndTargetsMaster(t *testing.T) {
	const (
		session   = int32(7)
		callsign  = "SAS401"
		stand     = "A12"
		masterCid = "MASTER-CID"
	)

	hub := &Hub{}
	esHub := &testutil.MockEuroscopeHub{
		GetMasterCidFn: func(gotSession int32) string {
			assert.Equal(t, session, gotSession)
			return masterCid
		},
	}
	assignment := models.StandAssignment{
		SessionID: session,
		Callsign:  callsign,
		Stand:     stand,
		Direction: string(sat.AssignmentDirectionArrival),
		Stage:     services.StageAssigned,
	}

	hub.sendAllocatedStandToEuroscope(esHub, assignment, stand)

	assert.Empty(t, esHub.Stands, "ASSIGNED arrivals must not update EuroScope yet")
	assert.Empty(t, esHub.Broadcasts, "arrival stands must never be broadcast to every EuroScope client")

	assignment.Stage = services.StageConfirmed
	hub.sendAllocatedStandToEuroscope(esHub, assignment, stand)

	require.Len(t, esHub.Stands, 1)
	assert.Equal(t, testutil.StandCall{
		Session:  session,
		Cid:      masterCid,
		Callsign: callsign,
		Stand:    stand,
	}, esHub.Stands[0])
	assert.Empty(t, esHub.Broadcasts)
}

func TestSendAllocatedStandToEuroscope_ConfirmedArrivalWithoutMasterDoesNotBroadcast(t *testing.T) {
	hub := &Hub{}
	esHub := &testutil.MockEuroscopeHub{}
	assignment := models.StandAssignment{
		SessionID: 7,
		Callsign:  "SAS402",
		Stand:     "A14",
		Direction: string(sat.AssignmentDirectionArrival),
		Stage:     services.StageConfirmed,
	}

	hub.sendAllocatedStandToEuroscope(esHub, assignment, assignment.Stand)

	assert.Empty(t, esHub.Stands)
	assert.Empty(t, esHub.Broadcasts)
}

func TestSendAllocatedStandToEuroscope_DepartureBehaviorRemainsBroadcast(t *testing.T) {
	hub := &Hub{}
	esHub := &testutil.MockEuroscopeHub{}
	assignment := models.StandAssignment{
		SessionID: 7,
		Callsign:  "SAS501",
		Stand:     "B12",
		Direction: string(sat.AssignmentDirectionDeparture),
		Stage:     services.StageDepartureBlock,
	}

	hub.sendAllocatedStandToEuroscope(esHub, assignment, assignment.Stand)

	assert.Empty(t, esHub.Stands)
	require.Len(t, esHub.Broadcasts, 1)
	event, ok := esHub.Broadcasts[0].(euroscopeEvents.StandEvent)
	require.True(t, ok)
	assert.Equal(t, assignment.Callsign, event.Callsign)
	assert.Equal(t, assignment.Stand, event.Stand)
}
