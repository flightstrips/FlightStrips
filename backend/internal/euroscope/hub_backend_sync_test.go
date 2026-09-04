package euroscope

import (
	"context"
	"testing"
	"time"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendBackendSyncIfNeeded_SendsLineupForDepartBayWithoutStoredState(t *testing.T) {
	const session = int32(1)

	departState := euroscopeEvents.GroundStateDepart
	seenAt := time.Now().UTC()
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{
				{
					Callsign:        "SAS101",
					Origin:          "EKCH",
					Bay:             shared.BAY_DEPART,
					EuroscopeSeenAt: &seenAt,
				},
				{
					Callsign:        "SAS102",
					Origin:          "EKCH",
					Bay:             shared.BAY_DEPART,
					State:           &departState,
					EuroscopeSeenAt: &seenAt,
				},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{StripRepoVal: stripRepo},
	}
	client := startQueuedTestClient(&Client{
		session: session,
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	})

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

func TestSendBackendSyncIfNeeded_ExcludesVatsimOnlyPlanningStrips(t *testing.T) {
	const session = int32(1)
	seenAt := time.Now().UTC()
	stripRepo := &testutil.MockStripRepository{
		ListFn: func(_ context.Context, gotSession int32) ([]*internalModels.Strip, error) {
			assert.Equal(t, session, gotSession)
			return []*internalModels.Strip{
				{Callsign: "PREFILE1", Bay: shared.BAY_DEP_HIDDEN},
				{Callsign: "INBOUND1", Bay: shared.BAY_ARR_HIDDEN},
				{Callsign: "UNSYNCED1", Bay: shared.BAY_NOT_CLEARED},
				{Callsign: "SAS101", Bay: shared.BAY_NOT_CLEARED, EuroscopeSeenAt: &seenAt},
			}, nil
		},
	}

	hub := &Hub{server: &testutil.MockServer{StripRepoVal: stripRepo}}
	client := startQueuedTestClient(&Client{
		session: session,
		user:    shared.NewAuthenticatedUser("1234567", 0, nil),
	})

	hub.sendBackendSyncIfNeeded(client)

	message := <-client.send
	syncEvent, ok := message.(euroscopeEvents.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, syncEvent.Strips, 1)
	assert.Equal(t, "SAS101", syncEvent.Strips[0].Callsign)
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

func TestBackendSyncGroundState_AirborneDoesNotSendStaleTaxiState(t *testing.T) {
	taxiState := euroscopeEvents.GroundStateTaxi
	strip := &internalModels.Strip{
		Callsign: "SAS104",
		Origin:   "EKCH",
		Bay:      shared.BAY_AIRBORNE,
		State:    &taxiState,
	}

	assert.Equal(t, euroscopeEvents.GroundStateUnknown, backendSyncGroundState(strip))
}
