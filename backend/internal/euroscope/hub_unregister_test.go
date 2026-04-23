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
