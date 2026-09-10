package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

func TestOnRegister_EnqueuesLatestAMANReplacementOnInitialConnectAndReconnect(t *testing.T) {
	provider := &reconnectAMANProvider{revision: 4}
	hub := newAMANInitialTestHub(t, provider)

	connect := func() frontendEvents.AMANStateEvent {
		client := startQueuedTestClient(&Client{
			hub: hub, session: 42, position: "118.105", airport: "EKCH", callsign: "EKCH_A_TWR",
			user: shared.NewAuthenticatedUser("1234567", 0, nil), readOnly: true,
			send: make(chan events.OutgoingMessage, 3),
		})
		hub.OnRegister(client)
		_, ok := waitForOutgoingMessage(t, client.send).(frontendEvents.InitialEvent)
		require.True(t, ok)
		state, ok := waitForOutgoingMessage(t, client.send).(frontendEvents.AMANStateEvent)
		require.True(t, ok)
		return state
	}

	require.Equal(t, uint64(4), connect().Data.Revision)
	provider.revision = 9 // simulates committed revisions missed while disconnected
	require.Equal(t, uint64(9), connect().Data.Revision)
	require.Equal(t, []string{"EKCH", "EKCH"}, provider.airports)
}

func TestOnRegister_DerivesAMANFMPCapabilityFromAuthenticatedServerRole(t *testing.T) {
	tests := []struct {
		name       string
		role       string
		authorized bool
	}{
		{name: "authorized FMP", role: "EKCH_FMH", authorized: true},
		{name: "unauthorized controller", role: "EKCH_A_TWR", authorized: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hub := newAMANInitialTestHub(t, nil)
			hub.amanFMPRoles = map[string]struct{}{"EKCH_FMH": {}}
			hub.amanRoleForPosition = func(string) string { return test.role }
			client := startQueuedTestClient(&Client{
				hub: hub, session: 42, position: "118.105", airport: "EKCH", callsign: test.role,
				user: validFrontendUser("1234567"), send: make(chan events.OutgoingMessage, 2),
			})

			hub.OnRegister(client)

			event, ok := waitForOutgoingMessage(t, client.send).(frontendEvents.InitialEvent)
			require.True(t, ok)
			assert.Equal(t, test.authorized, event.Capabilities.AMANFMP)
		})
	}
}

func validFrontendUser(cid string) shared.AuthenticatedUser {
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())})
	return shared.NewAuthenticatedUser(cid, 0, token)
}

type reconnectAMANProvider struct {
	revision uint64
	airports []string
}

func (p *reconnectAMANProvider) CurrentAMANState(_ context.Context, airport string) (frontendEvents.AMANStateEvent, error) {
	p.airports = append(p.airports, airport)
	return frontendEvents.AMANStateEvent{Version: 1, Data: frontendEvents.AMANState{Airport: airport, Revision: p.revision}}, nil
}

func newAMANInitialTestHub(t *testing.T, provider AMANStateProvider) *Hub {
	t.Helper()
	return &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: &testutil.MockControllerRepository{ListBySessionFn: func(context.Context, int32) ([]*internalModels.Controller, error) {
				return []*internalModels.Controller{}, nil
			}},
			StripRepoVal: &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*internalModels.Strip, error) { return []*internalModels.Strip{}, nil }},
			SectorRepoVal: &testutil.MockSectorOwnerRepository{ListBySessionFn: func(context.Context, int32) ([]*internalModels.SectorOwner, error) {
				return []*internalModels.SectorOwner{}, nil
			}},
			SessionRepoVal: &testutil.MockSessionRepository{GetByIDFn: func(context.Context, int32) (*internalModels.Session, error) {
				return &internalModels.Session{ID: 42, Name: "LIVE", Airport: "EKCH"}, nil
			}},
			CoordRepoVal: &testutil.MockCoordinationRepository{ListBySessionFn: func(context.Context, int32) ([]*internalModels.Coordination, error) {
				return []*internalModels.Coordination{}, nil
			}},
		},
		amanStateProvider: provider,
		messages:          map[int32][]frontendEvents.MessageReceivedEvent{}, metarCache: map[int32]string{},
		arrAtisCodeCache: map[int32]string{}, depAtisCodeCache: map[int32]string{},
	}
}
