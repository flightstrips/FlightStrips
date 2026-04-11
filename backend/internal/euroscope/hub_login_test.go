package euroscope

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	payload, err := json.Marshal(euroscopeEvents.LoginEvent{
		Type:       euroscopeEvents.Login,
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

	payload, err := json.Marshal(euroscopeEvents.LoginEvent{
		Type:       euroscopeEvents.Login,
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
