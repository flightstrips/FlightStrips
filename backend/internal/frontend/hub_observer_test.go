package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFrontendController_UsesPositionMetadata(t *testing.T) {
	controller := buildFrontendController("OBS_CTR", "121.730", map[string]*internalModels.SectorOwner{
		"121.730": {
			Identifier: "TW",
			Sector:     []string{"TE", "TW"},
		},
	})

	assert.Equal(t, "OBS_CTR", controller.Callsign)
	assert.Equal(t, "121.730", controller.Position)
	assert.Equal(t, "TW", controller.Identifier)
	assert.Equal(t, []string{"TE", "TW"}, controller.OwnedSectors)
}

func TestIsObserverController_UsesEuroscopeObserverLookup(t *testing.T) {
	controller := &internalModels.Controller{Observer: true}

	assert.True(t, isObserverController(controller, &testutil.MockEuroscopeHub{}))
	assert.False(t, isObserverController(&internalModels.Controller{}, &testutil.MockEuroscopeHub{}))
	assert.False(t, isObserverController(nil, &testutil.MockEuroscopeHub{}))
}

func TestOnRegister_WaitingObserverReceivesObserverDisconnectState(t *testing.T) {
	hub := &Hub{}
	client := &Client{
		hub:      hub,
		session:  WaitingForEuroscopeConnectionSessionId,
		readOnly: true,
		send:     make(chan events.OutgoingMessage, 1),
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	hub.OnRegister(client)

	select {
	case message := <-client.send:
		event, ok := message.(frontendEvents.DisconnectEvent)
		require.True(t, ok)
		assert.True(t, event.ReadOnly)
	default:
		t.Fatal("expected waiting disconnect event")
	}
}

func TestCidDisconnect_ClearsObserverWaitingStateWhenObserverIsOffline(t *testing.T) {
	hub := &Hub{
		clients: map[*Client]bool{},
		server: &testutil.MockServer{
			EuroscopeHubVal: &testutil.MockEuroscopeHub{
				IsObserverCidFn: func(string) bool { return false },
			},
		},
	}
	client := &Client{
		hub:      hub,
		session:  42,
		readOnly: true,
		send:     make(chan events.OutgoingMessage, 1),
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}
	hub.clients[client] = true

	hub.CidDisconnect("1234567")

	select {
	case message := <-client.send:
		event, ok := message.(frontendEvents.DisconnectEvent)
		require.True(t, ok)
		assert.False(t, event.ReadOnly)
	default:
		t.Fatal("expected disconnect event")
	}
}
