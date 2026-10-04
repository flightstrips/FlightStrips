package euroscope

import (
	"context"
	"strings"
	"testing"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestHandleLogin_UsesReportedSweatboxSessionName(t *testing.T) {
	var gotAirport string
	var gotSessionName string

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, _ string, _ int32) (*internalModels.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *internalModels.Controller) error {
			return nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		GetOrCreateSessionFn: func(airport string, name string) (shared.Session, error) {
			gotAirport = airport
			gotSessionName = name
			return shared.Session{Id: 42, Airport: airport, Name: name}, nil
		},
	}

	hub := &Hub{server: server}
	user := shared.NewAuthenticatedUser("1234567", 0, nil)

	payload, err := proto.Marshal(&euroscopeEvents.LoginEvent{
		Connection: "SWEATBOX",
		Airport:    "EKCH",
		Position:   "121.500",
		Callsign:   "EKCH_GND",
		Range:      150,
	})
	require.NoError(t, err)

	_, _, err = hub.handleLogin(payload, user)
	require.NoError(t, err)
	assert.Equal(t, "EKCH", gotAirport)
	assert.Equal(t, "SWEATBOX", gotSessionName)
}

func TestHandleLogin_PlaybackSessionGetsUniqueName(t *testing.T) {
	var gotSessionName string

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, _ string, _ int32) (*internalModels.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *internalModels.Controller) error {
			return nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		GetOrCreateSessionFn: func(airport string, name string) (shared.Session, error) {
			gotSessionName = name
			return shared.Session{Id: 7, Airport: airport, Name: name}, nil
		},
	}

	hub := &Hub{server: server}
	user := shared.NewAuthenticatedUser("1234567", 0, nil)

	payload, err := proto.Marshal(&euroscopeEvents.LoginEvent{
		Connection: "PLAYBACK",
		Airport:    "EKCH",
		Position:   "121.500",
		Callsign:   "EKCH_GND",
		Range:      150,
	})
	require.NoError(t, err)

	_, _, err = hub.handleLogin(payload, user)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(gotSessionName, "PLAYBACK_"), "expected playback session name to be namespaced")
}

func TestGetClientLocalIP_ReturnsCidScopedValue(t *testing.T) {
	hub := &Hub{
		localIPByClient: map[string]string{
			clientLocalIPKey(42, "1234567"): "192.168.1.25",
		},
	}

	assert.Equal(t, "192.168.1.25", hub.GetClientLocalIP(42, "1234567"))
	assert.Empty(t, hub.GetClientLocalIP(42, "7654321"))
	assert.Empty(t, hub.GetClientLocalIP(7, "1234567"))
}

func TestHandleLogin_PlaybackIdentitySurvivesHubRestart(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, _ string, _ int32) (*internalModels.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, _ *internalModels.Controller) error { return nil },
	}
	sessions := map[string]int32{}
	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		GetOrCreateSessionFn: func(airport, name string) (shared.Session, error) {
			require.Equal(t, "EKCH", airport)
			id, ok := sessions[name]
			if !ok {
				id = int32(len(sessions) + 1)
				sessions[name] = id
			}
			return shared.Session{Id: id, Airport: airport, Name: name}, nil
		},
	}
	user := shared.NewAuthenticatedUser("1234567", 0, nil)
	for _, name := range []string{
		"PLAYBACK_0123456789ABCDEF0123456789ABCDEF",
		"PLAYBACK_FEDCBA9876543210FEDCBA9876543210",
	} {
		payload, err := proto.Marshal(&euroscopeEvents.LoginEvent{
			Connection: name, Airport: "EKCH", Callsign: "EKCH_GND", Position: "121.500",
		})
		require.NoError(t, err)
		// A new hub has no memory of the previous login, as after a restart.
		for attempt := 0; attempt < 2; attempt++ {
			hub := &Hub{server: server}
			event, id, err := hub.handleLogin(payload, user)
			require.NoError(t, err)
			assert.Equal(t, name, event.Connection)
			assert.Equal(t, sessions[name], id)
		}
	}
	assert.Len(t, sessions, 2)
}
