package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
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
	client := startQueuedTestClient(&Client{
		hub:      hub,
		session:  WaitingForEuroscopeConnectionSessionId,
		readOnly: true,
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	})

	hub.OnRegister(client)

	message := waitForOutgoingMessage(t, client.send)
	event, ok := message.(frontendEvents.DisconnectEvent)
	require.True(t, ok)
	assert.True(t, event.ReadOnly)
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
	client := startQueuedTestClient(&Client{
		hub:      hub,
		session:  42,
		readOnly: true,
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	})
	hub.clients[client] = true

	hub.handleCidDisconnect("1234567")

	message := waitForOutgoingMessage(t, client.send)
	event, ok := message.(frontendEvents.DisconnectEvent)
	require.True(t, ok)
	assert.False(t, event.ReadOnly)
}

func TestCidDisconnect_ClearsSessionLabelsBeforeUnregister(t *testing.T) {
	hub := &Hub{
		clients: map[*Client]bool{},
		server: &testutil.MockServer{
			EuroscopeHubVal: &testutil.MockEuroscopeHub{
				IsObserverCidFn: func(string) bool { return false },
			},
		},
	}
	client := startQueuedTestClient(&Client{
		hub:         hub,
		session:     42,
		sessionName: "LIVE",
		position:    "118.105",
		airport:     "EKCH",
		callsign:    "EKCH_A_TWR",
		user:        shared.NewAuthenticatedUser("1234567", 0, nil),
	})
	hub.clients[client] = true

	hub.handleCidDisconnect("1234567")

	assert.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)
	assert.Empty(t, client.sessionName)
	assert.Equal(t, WaitingForEuroscopeConnectionPosition, client.position)
	assert.Equal(t, WaitingForEuroscopeConnectionAirport, client.airport)
	assert.Equal(t, WaitingForEuroscopeConnectionCallsign, client.callsign)
}

func TestAssociateCidOnlineClients_AssociatesAllMatchingClients(t *testing.T) {
	controllerCID := "1234567"
	hub := &Hub{
		clients: map[*Client]bool{},
		server: &testutil.MockServer{
			ControllerRepoVal: &testutil.MockControllerRepository{
				GetByCidFn: func(_ context.Context, cid string) (*internalModels.Controller, error) {
					assert.Equal(t, "1234567", cid)
					return &internalModels.Controller{
						Cid:      &controllerCID,
						Session:  42,
						Position: "118.105",
						Callsign: "EKCH_A_TWR",
					}, nil
				},
			},
			SessionRepoVal: &testutil.MockSessionRepository{
				GetByIDFn: func(_ context.Context, id int32) (*internalModels.Session, error) {
					assert.Equal(t, int32(42), id)
					return &internalModels.Session{
						ID:      42,
						Name:    "LIVE",
						Airport: "EKCH",
					}, nil
				},
			},
			EuroscopeHubVal: &testutil.MockEuroscopeHub{
				IsObserverCidFn: func(string) bool { return false },
			},
		},
	}
	first := &Client{
		hub:      hub,
		session:  WaitingForEuroscopeConnectionSessionId,
		position: WaitingForEuroscopeConnectionPosition,
		callsign: WaitingForEuroscopeConnectionCallsign,
		airport:  WaitingForEuroscopeConnectionAirport,
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}
	second := &Client{
		hub:      hub,
		session:  WaitingForEuroscopeConnectionSessionId,
		position: WaitingForEuroscopeConnectionPosition,
		callsign: WaitingForEuroscopeConnectionCallsign,
		airport:  WaitingForEuroscopeConnectionAirport,
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}
	other := &Client{
		hub:      hub,
		session:  WaitingForEuroscopeConnectionSessionId,
		position: WaitingForEuroscopeConnectionPosition,
		callsign: WaitingForEuroscopeConnectionCallsign,
		airport:  WaitingForEuroscopeConnectionAirport,
		user:     shared.NewAuthenticatedUser("7654321", 0, nil),
	}

	hub.clients[first] = true
	hub.clients[second] = true
	hub.clients[other] = true

	initialClients := hub.associateCidOnlineClients(cidOnlineMessage{session: 42, cid: "1234567"})

	assert.ElementsMatch(t, []*Client{first, second}, initialClients)
	for _, client := range []*Client{first, second} {
		assert.Equal(t, int32(42), client.session)
		assert.Equal(t, "LIVE", client.sessionName)
		assert.Equal(t, "118.105", client.position)
		assert.Equal(t, "EKCH", client.airport)
		assert.Equal(t, "EKCH_A_TWR", client.callsign)
	}
	assert.Equal(t, WaitingForEuroscopeConnectionSessionId, other.session)
}

func TestCidDisconnect_ClearsAllMatchingClients(t *testing.T) {
	hub := &Hub{
		clients: map[*Client]bool{},
		server: &testutil.MockServer{
			EuroscopeHubVal: &testutil.MockEuroscopeHub{
				IsObserverCidFn: func(string) bool { return false },
			},
		},
	}
	first := startQueuedTestClient(&Client{
		hub:         hub,
		session:     42,
		sessionName: "LIVE",
		position:    "118.105",
		airport:     "EKCH",
		callsign:    "EKCH_A_TWR",
		user:        shared.NewAuthenticatedUser("1234567", 0, nil),
	})
	second := startQueuedTestClient(&Client{
		hub:         hub,
		session:     42,
		sessionName: "LIVE",
		position:    "121.730",
		airport:     "EKCH",
		callsign:    "EKCH_A_GND",
		user:        shared.NewAuthenticatedUser("1234567", 0, nil),
	})
	other := startQueuedTestClient(&Client{
		hub:         hub,
		session:     42,
		sessionName: "LIVE",
		position:    "121.855",
		airport:     "EKCH",
		callsign:    "EKCH_DEL",
		user:        shared.NewAuthenticatedUser("7654321", 0, nil),
	})

	hub.clients[first] = true
	hub.clients[second] = true
	hub.clients[other] = true

	hub.handleCidDisconnect("1234567")

	for _, client := range []*Client{first, second} {
		assert.Equal(t, WaitingForEuroscopeConnectionSessionId, client.session)
		assert.Empty(t, client.sessionName)
		assert.Equal(t, WaitingForEuroscopeConnectionPosition, client.position)
		assert.Equal(t, WaitingForEuroscopeConnectionAirport, client.airport)
		assert.Equal(t, WaitingForEuroscopeConnectionCallsign, client.callsign)

		message := waitForOutgoingMessage(t, client.send)
		event, ok := message.(frontendEvents.DisconnectEvent)
		require.True(t, ok)
		assert.False(t, event.ReadOnly)
	}

	assert.Equal(t, int32(42), other.session)
	assert.Equal(t, "LIVE", other.sessionName)
	select {
	case <-other.send:
		t.Fatal("did not expect disconnect event for other cid")
	default:
	}
}
