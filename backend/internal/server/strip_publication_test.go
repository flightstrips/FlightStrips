package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicationSnapshotPreservesPendingRouteWithoutReads(t *testing.T) {
	strip := &models.Strip{ID: 7, Session: 42, NextDisplay: &models.NextDisplay{Label: "GW", Frequency: "118.580"}}
	snapshot := &models.StripPublicationSnapshot{Strip: strip, Session: &models.Session{ID: 42}, CoordinationPending: true}
	ctx := shared.WithStripPublication(context.Background(), snapshot)
	srv := &Server{coordRepo: &testutil.MockCoordinationRepository{GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) {
		t.Fatal("publication reread coordination")
		return nil, nil
	}}}
	display, err := srv.ComputeNextDisplayForStripContext(ctx, strip, 42)
	require.NoError(t, err)
	require.Equal(t, strip.NextDisplay, display)
	require.NotSame(t, strip.NextDisplay, display)
	session, err := routeSessionByID(ctx, nil, 42)
	require.NoError(t, err)
	require.Same(t, snapshot.Session, session)
	owners, err := routeSectorOwners(ctx, nil, 42)
	require.NoError(t, err)
	require.Empty(t, owners)
	controllers, err := getControllersForUpdate(ctx, nil, 42)
	require.NoError(t, err)
	require.Empty(t, controllers)
}
