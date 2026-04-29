package euroscope

import (
	"context"
	"testing"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendBackendSyncIfNeeded_SendsLineupForDepartBayWithoutStoredState(t *testing.T) {
	const session = int32(1)

	departState := euroscopeEvents.GroundStateDepart
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{
				{
					Callsign: "SAS101",
					Origin:   "EKCH",
					Bay:      shared.BAY_DEPART,
				},
				{
					Callsign: "SAS102",
					Origin:   "EKCH",
					Bay:      shared.BAY_DEPART,
					State:    &departState,
				},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{StripRepoVal: stripRepo},
	}
	client := &Client{
		session: session,
		send:    make(chan events.OutgoingMessage, 1),
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	hub.sendBackendSyncIfNeeded(client)

	message := <-client.send
	syncEvent, ok := message.(euroscopeEvents.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, syncEvent.Strips, 2)

	groundStates := map[string]string{}
	for _, strip := range syncEvent.Strips {
		groundStates[strip.Callsign] = strip.GroundState
	}

	assert.Equal(t, euroscopeEvents.GroundStateLineup, groundStates["SAS101"])
	assert.Equal(t, euroscopeEvents.GroundStateDepart, groundStates["SAS102"])
}

func TestBackendSyncGroundState_DepartIgnoresStaleTaxiState(t *testing.T) {
	taxiState := euroscopeEvents.GroundStateTaxi
	strip := &internalModels.Strip{
		Callsign: "SAS103",
		Origin:   "EKCH",
		Bay:      shared.BAY_DEPART,
		State:    &taxiState,
	}

	assert.Equal(t, euroscopeEvents.GroundStateLineup, backendSyncGroundState(strip))
}
