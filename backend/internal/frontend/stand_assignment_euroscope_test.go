package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/services"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
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

	hub.sendAllocatedStandToEuroscope(t.Context(), esHub, assignment, stand)

	assert.Empty(t, esHub.Stands, "ASSIGNED arrivals must not update EuroScope yet")
	assert.Empty(t, esHub.Broadcasts, "arrival stands must never be broadcast to every EuroScope client")

	assignment.Stage = services.StageConfirmed
	hub.sendAllocatedStandToEuroscope(t.Context(), esHub, assignment, stand)

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

	hub.sendAllocatedStandToEuroscope(t.Context(), esHub, assignment, assignment.Stand)

	assert.Empty(t, esHub.Stands)
	assert.Empty(t, esHub.Broadcasts)
}

func TestSendAllocatedStandToEuroscope_DepartureTargetsMaster(t *testing.T) {
	hub := &Hub{}
	esHub := &testutil.MockEuroscopeHub{GetMasterCidFn: func(int32) string { return "MASTER-CID" }}
	assignment := models.StandAssignment{
		SessionID: 7,
		Callsign:  "SAS501",
		Stand:     "B12",
		Direction: string(sat.AssignmentDirectionDeparture),
		Stage:     services.StageDepartureBlock,
	}

	hub.sendAllocatedStandToEuroscope(t.Context(), esHub, assignment, assignment.Stand)

	require.Len(t, esHub.Stands, 1)
	assert.Equal(t, testutil.StandCall{Session: assignment.SessionID, Cid: "MASTER-CID", Callsign: assignment.Callsign, Stand: assignment.Stand}, esHub.Stands[0])
	assert.Empty(t, esHub.Broadcasts)
}

func TestSendAllocatedStandToEuroscope_PrefersTrackingController(t *testing.T) {
	const (
		session      = int32(7)
		callsign     = "SAS502"
		trackingCid  = "TRACKING-CID"
		masterCid    = "MASTER-CID"
		controllerID = "EKCH_APP"
	)
	hub := &Hub{server: &testutil.MockServer{
		StripRepoVal: &testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				assert.Equal(t, session, gotSession)
				assert.Equal(t, callsign, gotCallsign)
				return &models.Strip{Callsign: callsign, TrackingController: controllerID}, nil
			},
		},
		ControllerRepoVal: &testutil.MockControllerRepository{
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Controller, error) {
				assert.Equal(t, session, gotSession)
				assert.Equal(t, controllerID, gotCallsign)
				return &models.Controller{Callsign: controllerID, Cid: ptr(trackingCid)}, nil
			},
		},
	}}
	esHub := &testutil.MockEuroscopeHub{GetMasterCidFn: func(int32) string { return masterCid }}
	assignment := models.StandAssignment{
		SessionID: session,
		Callsign:  callsign,
		Stand:     "B13",
		Direction: string(sat.AssignmentDirectionDeparture),
		Stage:     services.StageDepartureBlock,
	}

	hub.sendAllocatedStandToEuroscope(t.Context(), esHub, assignment, assignment.Stand)

	require.Len(t, esHub.Stands, 1)
	assert.Equal(t, trackingCid, esHub.Stands[0].Cid)
	assert.Empty(t, esHub.Broadcasts)
}

func TestPublishStandAllocationForwardsPilotRequestToControllersAndEuroscope(t *testing.T) {
	const (
		session  = int32(7)
		callsign = "SAS503"
		stand    = "B14"
	)
	esHub := &testutil.MockEuroscopeHub{GetMasterCidFn: func(int32) string { return "MASTER-CID" }}
	hub := &Hub{
		send: make(chan internalMessage, 2),
		server: &testutil.MockServer{
			EuroscopeHubVal: esHub,
			SessionRepoVal: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*models.Session, error) {
				return &models.Session{ID: session, Airport: "EKCH"}, nil
			}},
		},
	}

	err := hub.PublishStandAllocation(t.Context(), services.StandAllocationResult{
		Assignment: models.StandAssignment{
			SessionID: session, Callsign: callsign, Stand: stand,
			Direction: string(sat.AssignmentDirectionDeparture), Stage: services.StageReserved,
		},
		NotifyEuroscope: true,
	})

	require.NoError(t, err)
	message := <-hub.send
	update, ok := message.message.(frontendEvents.StandAssignmentUpdateEvent)
	require.True(t, ok)
	assert.Equal(t, callsign, update.Assignment.Callsign)
	assert.Equal(t, stand, update.Assignment.Stand)
	require.Len(t, esHub.Stands, 1)
	require.Equal(t, testutil.StandCall{Session: session, Cid: "MASTER-CID", Callsign: callsign, Stand: stand}, esHub.Stands[0])
}
