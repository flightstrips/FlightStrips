package frontend

import (
	"context"
	"errors"
	"testing"

	"FlightStrips/internal/aman"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

var errLookup = errors.New("database unavailable")

func failControllerLookup(hub *Hub) {
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*internalModels.Controller, error) { return nil, errLookup },
	}}
}

type failingClearStore struct {
	memoryForcedFMPStore
	failClear bool
}

func (s *failingClearStore) ClearForcedFMP(ctx context.Context, session int32) error {
	if s.failClear {
		return errLookup
	}
	return s.memoryForcedFMPStore.ClearForcedFMP(ctx, session)
}

func TestForceFMPRejectedWhenFMPPresenceUnknown(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	failControllerLookup(hub)

	sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)

	require.Equal(t, string(aman.ErrorDependencyUnavailable), lastRejection(t, client).Data.Code)
	require.False(t, hub.hasAMANFMPAuthority(client))
	require.False(t, hub.amanFMPAuthorityEvent(context.Background(), client, false, false).Data.CanForce)
}

func TestControllerSequencingRejectedWhenPresenceUnknown(t *testing.T) {
	hub, client, service := newSequencingTestHub(t, "EKDK_D_CTR", nil)
	failControllerLookup(hub)

	// SAS3 is MONAK, which D_CTR only gets when its neighbours are offline.
	sendSequence(t, hub, client, frontendEvents.AMANLockFlightType, `"callsign":"SAS3"`)

	require.Empty(t, service.operation)
	require.Equal(t, string(aman.ErrorDependencyUnavailable), lastRejection(t, client).Data.Code)
}

func TestForcedFMPRevokedEvenWhenStoreDeleteFails(t *testing.T) {
	store := &failingClearStore{memoryForcedFMPStore: memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{}}}
	online := []*internalModels.Controller{}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)
	require.True(t, hub.hasAMANFMPAuthority(holder))

	store.failClear = true
	hub.resetForcedFMPOnControllerOnline(forcedFMPTestSession, "EKDK_FMP", "131.040")
	require.False(t, hub.hasAMANFMPAuthority(holder), "revoked in memory although the delete failed")
	require.Contains(t, store.grants, forcedFMPTestSession, "the row is still in the store")

	store.failClear = false
	require.False(t, hub.hasAMANFMPAuthority(holder))
	require.NotContains(t, store.grants, forcedFMPTestSession, "the delete is retried on the next check")
}

func TestAuthorityRefreshesOnFMPPresenceWithoutGrant(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, _, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)

	hub.resetForcedFMPOnControllerOnline(forcedFMPTestSession, "EKDK_FMP", "131.040")
	require.Equal(t, []int32{forcedFMPTestSession}, hub.amanAuthorityRefresh.drain())

	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "EKDK_FMP")
	require.Equal(t, []int32{forcedFMPTestSession}, hub.amanAuthorityRefresh.drain())

	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "EKCH_O_APP")
	require.Empty(t, hub.amanAuthorityRefresh.drain(), "other logoffs without a grant change nothing")
}

func TestSessionRefreshQueueNeverDropsASession(t *testing.T) {
	queue := newSessionRefreshQueue()
	for range 100 {
		queue.add(1)
	}
	queue.add(2)

	<-queue.wake()
	require.ElementsMatch(t, []int32{1, 2}, queue.drain())
	require.Empty(t, queue.drain())
}
