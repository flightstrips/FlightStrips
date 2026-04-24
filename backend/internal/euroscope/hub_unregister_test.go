package euroscope

import (
	"context"
	"testing"

	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClearClientCid_IgnoresMissingControllerRow(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		SetCidFn: func(_ context.Context, session int32, callsign string, cid *string) (int64, error) {
			assert.Equal(t, int32(42), session)
			assert.Equal(t, "EKCH_A__TWR", callsign)
			assert.Nil(t, cid)
			return 0, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: controllerRepo,
		},
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_A__TWR",
		user:     shared.NewAuthenticatedUser("10000007", 0, nil),
	}

	err := hub.clearClientCid(client)

	require.NoError(t, err)
}

func TestClearClientCid_ReturnsUnexpectedRowCountError(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 2, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: controllerRepo,
		},
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_A__TWR",
		user:     shared.NewAuthenticatedUser("10000007", 0, nil),
	}

	err := hub.clearClientCid(client)

	require.Error(t, err)
	assert.EqualError(t, err, "unexpected controller CID cleanup row count: 2")
}

func TestOnUnregister_LastOperationalClientDisconnectsObserverFrontends(t *testing.T) {
	frontendHub := &testutil.MockFrontendHub{}
	controllerRepo := &testutil.MockControllerRepository{
		SetCidFn: func(_ context.Context, session int32, callsign string, cid *string) (int64, error) {
			assert.Equal(t, int32(42), session)
			assert.Equal(t, "EKCH_A_TWR", callsign)
			assert.Nil(t, cid)
			return 1, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			FrontendHubVal:    frontendHub,
			ControllerRepoVal: controllerRepo,
		},
		clients: map[*Client]bool{},
		master:  map[int32]*Client{},
		observerByCid: map[string]bool{
			"obs-cid": true,
		},
		airportClientCount: map[string]int{
			"EKCH": 1,
		},
	}

	observer := &Client{
		hub:      hub,
		session:  42,
		airport:  "EKCH",
		observer: true,
		user:     shared.NewAuthenticatedUser("obs-cid", 0, nil),
	}
	master := &Client{
		hub:      hub,
		session:  42,
		airport:  "EKCH",
		callsign: "EKCH_A_TWR",
		user:     shared.NewAuthenticatedUser("master-cid", 0, nil),
	}

	hub.clients[observer] = true
	hub.master[42] = master

	hub.OnUnregister(master)

	assert.False(t, hub.HasActiveClientForAirport("EKCH"))
	require.Len(t, frontendHub.CidDisconnects, 1)
	assert.Equal(t, "obs-cid", frontendHub.CidDisconnects[0].Cid)
}
