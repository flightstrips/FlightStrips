package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/testutil"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandBlockNeighborsAreBidirectional(t *testing.T) {
	registry, err := sat.LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
BLOCKS:A2
STAND:EKCH:A2:N055.37.42.710:E012.38.33.451:30
STAND:EKCH:A3:N055.37.42.710:E012.38.33.452:30
BLOCKS:A1
`))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"A2", "A3"}, standBlockNeighborsFromRegistry(registry, "EKCH", "A1"))
	assert.Equal(t, []string{"A1"}, standBlockNeighborsFromRegistry(registry, "EKCH", "A2"))
}

func TestSnapshotBuilder_Build_IncludesAssociatedLocalIP(t *testing.T) {
	ctx := context.WithValue(context.Background(), "request-id", "snapshot-test")

	builder := NewSnapshotBuilder(SnapshotBuilderDependencies{
		ControllerRepo: &testutil.MockControllerRepository{
			ListBySessionFn: func(callCtx context.Context, session int32) ([]*internalModels.Controller, error) {
				assert.Same(t, ctx, callCtx)
				assert.Equal(t, int32(42), session)
				return []*internalModels.Controller{}, nil
			},
		},
		StripRepo: &testutil.MockStripRepository{
			ListFn: func(callCtx context.Context, session int32) ([]*internalModels.Strip, error) {
				assert.Same(t, ctx, callCtx)
				assert.Equal(t, int32(42), session)
				return []*internalModels.Strip{}, nil
			},
		},
		SectorRepo: &testutil.MockSectorOwnerRepository{
			ListBySessionFn: func(callCtx context.Context, session int32) ([]*internalModels.SectorOwner, error) {
				assert.Same(t, ctx, callCtx)
				assert.Equal(t, int32(42), session)
				return []*internalModels.SectorOwner{}, nil
			},
		},
		SessionRepo: &testutil.MockSessionRepository{
			GetByIDFn: func(callCtx context.Context, id int32) (*internalModels.Session, error) {
				assert.Same(t, ctx, callCtx)
				assert.Equal(t, int32(42), id)
				return &internalModels.Session{
					ID:      42,
					Name:    "LIVE",
					Airport: "EKCH",
				}, nil
			},
		},
		CoordinationRepo: &testutil.MockCoordinationRepository{
			ListBySessionFn: func(callCtx context.Context, session int32) ([]*internalModels.Coordination, error) {
				assert.Same(t, ctx, callCtx)
				assert.Equal(t, int32(42), session)
				return []*internalModels.Coordination{}, nil
			},
		},
		EuroscopeHub: &testutil.MockEuroscopeHub{
			GetClientLocalIPFn: func(session int32, cid string) string {
				assert.Equal(t, int32(42), session)
				assert.Equal(t, "1234567", cid)
				return "192.168.1.25"
			},
		},
	})

	event, cachedAtis, err := builder.Build(ctx, InitialSnapshotRequest{
		SessionID: 42,
		Position:  "118.105",
		Airport:   "EKCH",
		Callsign:  "EKCH_A_TWR",
		UserCID:   "1234567",
		ReadOnly:  true,
	})

	require.NoError(t, err)
	assert.Nil(t, cachedAtis)
	assert.Equal(t, "192.168.1.25", event.LocalIP)
}
