package euroscope

import (
	"context"
	"encoding/json"
	"testing"

	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildLoginPayload(t *testing.T, callsign, position, airport string) []byte {
	return buildLoginPayloadWithObserver(t, callsign, position, airport, false)
}

func buildLoginPayloadWithObserver(t *testing.T, callsign, position, airport string, observer bool) []byte {
	t.Helper()
	payload, err := json.Marshal(euroscopeEvents.LoginEvent{
		Type:       euroscopeEvents.Login,
		Callsign:   callsign,
		Position:   position,
		Airport:    airport,
		Connection: "LIVE",
		Range:      100,
		Observer:   observer,
	})
	require.NoError(t, err)
	return payload
}

// TestHandleLoginEvent_UpdatesPositionOnSwitch covers the production incident:
// ES sends a login event over an existing connection when the controller
// switches position (unprime + prime). The handler must update the DB and the
// client's in-memory position field.
func TestHandleLoginEvent_UpdatesPositionOnSwitch(t *testing.T) {
	setPositionCalled := false
	newPosition := ""

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, callsign string, session int32) (*internalModels.Controller, error) {
			return &internalModels.Controller{
				Callsign: callsign,
				Session:  session,
				Position: "118.105", // old position
			}, nil
		},
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, _ int32, _ string, observer bool) (int64, error) {
			assert.False(t, observer)
			return 1, nil
		},
		SetPositionFn: func(_ context.Context, _ int32, _ string, position string) (int64, error) {
			setPositionCalled = true
			newPosition = position
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_M_TWR",
		position: "118.105",
		airport:  "EKCH",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	err := handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: buildLoginPayload(t, "EKCH_M_TWR", "119.700", "EKCH"),
	})

	require.NoError(t, err)
	assert.True(t, setPositionCalled, "SetPosition should be called when position changes")
	assert.Equal(t, "119.700", newPosition)
	assert.Equal(t, "119.700", client.position, "client.position must reflect the new position")
}

// TestHandleLoginEvent_NoSetPositionWhenUnchanged verifies that SetPosition is
// not called when the controller re-logs in on the same frequency.
func TestHandleLoginEvent_NoSetPositionWhenUnchanged(t *testing.T) {
	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, callsign string, session int32) (*internalModels.Controller, error) {
			return &internalModels.Controller{
				Callsign: callsign,
				Session:  session,
				Position: "118.105", // same as login event
			}, nil
		},
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, _ int32, _ string, observer bool) (int64, error) {
			assert.False(t, observer)
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_M_TWR",
		position: "118.105",
		airport:  "EKCH",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	err := handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: buildLoginPayload(t, "EKCH_M_TWR", "118.105", "EKCH"),
	})

	require.NoError(t, err)
	assert.Equal(t, "118.105", client.position)
}

// TestHandleLoginEvent_CreatesControllerIfNew verifies that a controller not
// yet in the DB (pgx.ErrNoRows) is created rather than updated.
func TestHandleLoginEvent_CreatesControllerIfNew(t *testing.T) {
	createCalled := false

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, _ string, _ int32) (*internalModels.Controller, error) {
			return nil, pgx.ErrNoRows
		},
		CreateFn: func(_ context.Context, controller *internalModels.Controller) error {
			createCalled = true
			assert.False(t, controller.Observer)
			return nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_D_GND",
		position: "",
		airport:  "EKCH",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	err := handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: buildLoginPayload(t, "EKCH_D_GND", "121.750", "EKCH"),
	})

	require.NoError(t, err)
	assert.True(t, createCalled, "Create should be called for a new controller")
	assert.Equal(t, "121.750", client.position)
	assert.Equal(t, "EKCH_D_GND", client.callsign)
}

// TestHandleLoginEvent_CallsUpdateLayouts verifies that UpdateLayouts is
// always invoked so sector/layout recalculation reflects the new position.
func TestHandleLoginEvent_CallsUpdateLayouts(t *testing.T) {
	updateLayoutsCalled := false

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, callsign string, session int32) (*internalModels.Controller, error) {
			return &internalModels.Controller{Callsign: callsign, Session: session, Position: "118.105"}, nil
		},
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, _ int32, _ string, observer bool) (int64, error) {
			assert.False(t, observer)
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalled = true
			return nil
		},
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_M_TWR",
		position: "118.105",
		airport:  "EKCH",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	err := handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: buildLoginPayload(t, "EKCH_M_TWR", "118.105", "EKCH"),
	})

	require.NoError(t, err)
	assert.True(t, updateLayoutsCalled, "UpdateLayouts must be called after every re-login")
}

func TestHandleLoginEvent_ObserverSkipsUpdateLayouts(t *testing.T) {
	updateLayoutsCalled := false

	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, callsign string, session int32) (*internalModels.Controller, error) {
			return &internalModels.Controller{Callsign: callsign, Session: session, Position: "118.105"}, nil
		},
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, _ int32, _ string, observer bool) (int64, error) {
			assert.True(t, observer)
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		UpdateLayoutsFn: func(_ int32) error {
			updateLayoutsCalled = true
			return nil
		},
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_M_TWR",
		position: "118.105",
		airport:  "EKCH",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	payload, err := json.Marshal(euroscopeEvents.LoginEvent{
		Type:     euroscopeEvents.Login,
		Callsign: "EKCH_M_TWR",
		Position: "118.105",
		Airport:  "EKCH",
		Observer: true,
	})
	require.NoError(t, err)

	err = handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: payload,
	})

	require.NoError(t, err)
	assert.False(t, updateLayoutsCalled, "UpdateLayouts must not be called for observer re-login")
	assert.True(t, client.observer)
	assert.True(t, hub.IsObserverCid(client.GetCid()))
}

func TestHandleLoginEvent_ObserverPositionChangeRefreshesFrontend(t *testing.T) {
	frontendHub := &testutil.MockFrontendHub{}
	controllerRepo := &testutil.MockControllerRepository{
		GetFn: func(_ context.Context, callsign string, session int32) (*internalModels.Controller, error) {
			return &internalModels.Controller{
				Callsign: callsign,
				Session:  session,
				Position: "121.700",
			}, nil
		},
		SetCidFn: func(_ context.Context, _ int32, _ string, _ *string) (int64, error) {
			return 1, nil
		},
		SetObserverFn: func(_ context.Context, _ int32, _ string, observer bool) (int64, error) {
			assert.True(t, observer)
			return 1, nil
		},
		SetPositionFn: func(_ context.Context, _ int32, _ string, _ string) (int64, error) {
			return 1, nil
		},
	}

	server := &testutil.MockServer{
		ControllerRepoVal: controllerRepo,
		FrontendHubVal:    frontendHub,
	}
	hub := &Hub{
		server:              server,
		sessionUpdateTimers: make(map[int32]*sessionUpdatePending),
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "FR_OBS",
		position: "121.700",
		airport:  "EKCH",
		observer: true,
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}

	err := handleLoginEvent(context.Background(), client, Message{
		Type:    euroscopeEvents.Login,
		Message: buildLoginPayloadWithObserver(t, "FR_OBS", "118.105", "EKCH", true),
	})

	require.NoError(t, err)
	require.Len(t, frontendHub.CidOnlines, 1)
	assert.Equal(t, int32(42), frontendHub.CidOnlines[0].Session)
	assert.Equal(t, "1234567", frontendHub.CidOnlines[0].Cid)
	assert.Equal(t, "118.105", client.position)
}
