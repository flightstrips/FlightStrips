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

func TestOnRegister_EnqueuesInitialSnapshot(t *testing.T) {
	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: &testutil.MockControllerRepository{
				ListBySessionFn: func(_ context.Context, session int32) ([]*internalModels.Controller, error) {
					assert.Equal(t, int32(42), session)
					return []*internalModels.Controller{}, nil
				},
			},
			StripRepoVal: &testutil.MockStripRepository{
				ListFn: func(_ context.Context, session int32) ([]*internalModels.Strip, error) {
					assert.Equal(t, int32(42), session)
					return []*internalModels.Strip{}, nil
				},
			},
			SectorRepoVal: &testutil.MockSectorOwnerRepository{
				ListBySessionFn: func(_ context.Context, session int32) ([]*internalModels.SectorOwner, error) {
					assert.Equal(t, int32(42), session)
					return []*internalModels.SectorOwner{}, nil
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
			CoordRepoVal: &testutil.MockCoordinationRepository{
				ListBySessionFn: func(_ context.Context, session int32) ([]*internalModels.Coordination, error) {
					assert.Equal(t, int32(42), session)
					return []*internalModels.Coordination{}, nil
				},
			},
		},
		messages:         map[int32][]frontendEvents.MessageReceivedEvent{},
		metarCache:       map[int32]string{},
		arrAtisCodeCache: map[int32]string{},
		depAtisCodeCache: map[int32]string{},
	}

	client := startQueuedTestClient(&Client{
		hub:      hub,
		session:  42,
		position: "118.105",
		airport:  "EKCH",
		callsign: "EKCH_A_TWR",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
		readOnly: true,
	})

	hub.OnRegister(client)

	message := waitForOutgoingMessage(t, client.send)
	event, ok := message.(frontendEvents.InitialEvent)
	require.True(t, ok)
	assert.Equal(t, "EKCH", event.Airport)
	assert.Equal(t, "EKCH_A_TWR", event.Callsign)
}
