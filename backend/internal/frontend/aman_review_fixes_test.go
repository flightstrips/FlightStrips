package frontend

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

// lockedFailingClearStore is safe for the background retry goroutine.
type lockedFailingClearStore struct {
	mu        sync.Mutex
	grants    map[int32]aman.ForcedFMPGrant
	failClear bool
}

func (s *lockedFailingClearStore) LoadForcedFMP(_ context.Context, session int32) (aman.ForcedFMPGrant, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grant, ok := s.grants[session]
	return grant, ok, nil
}

func (s *lockedFailingClearStore) SaveForcedFMP(_ context.Context, grant aman.ForcedFMPGrant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.grants[grant.SessionID] = grant
	return nil
}

func (s *lockedFailingClearStore) ClearForcedFMP(_ context.Context, session int32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failClear {
		return errLookup
	}
	delete(s.grants, session)
	return nil
}

func (s *lockedFailingClearStore) setFail(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failClear = fail
}

func (s *lockedFailingClearStore) has(session int32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.grants[session]
	return ok
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

func TestRestartDoesNotRestoreARevokedGrantWhoseDeleteFailed(t *testing.T) {
	store := &failingClearStore{memoryForcedFMPStore: memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{}}}
	online := []*internalModels.Controller{{Session: forcedFMPTestSession, Callsign: "EKCH_W_APP"}}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)

	// The holder logs off and the delete fails, then the backend restarts
	// before the retry.
	store.failClear = true
	online = nil
	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "EKCH_W_APP")
	require.Contains(t, store.grants, forcedFMPTestSession)
	store.failClear = false

	restarted, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	restarted.SetAMANForcedFMPStore(store)
	require.False(t, restarted.hasAMANFMPAuthority(client), "a holder who is offline is not restored")
	require.NotContains(t, store.grants, forcedFMPTestSession, "the stale row is deleted")
}

func TestRestartDoesNotRestoreGrantAfterHolderLogsOffAndOnAgain(t *testing.T) {
	store := &failingClearStore{memoryForcedFMPStore: memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{}}}
	online := []*internalModels.Controller{{ID: 7, Session: forcedFMPTestSession, Callsign: "EKCH_W_APP"}}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)
	require.Equal(t, int32(7), store.grants[forcedFMPTestSession].ControllerID)

	// 1. The holder logs off and the delete fails.
	store.failClear = true
	online = nil
	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "EKCH_W_APP")
	// 2. The holder logs on again (a new controller record) while deletes still fail.
	online = []*internalModels.Controller{{ID: 8, Session: forcedFMPTestSession, Callsign: "EKCH_W_APP"}}
	require.False(t, hub.hasAMANFMPAuthority(holder))
	require.Contains(t, store.grants, forcedFMPTestSession)
	// 3. The backend restarts before a delete succeeds.
	restarted, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	restarted.SetAMANForcedFMPStore(store)

	require.False(t, restarted.hasAMANFMPAuthority(client), "a grant from an earlier logon is never restored")
}

func TestInitialLookupSuccessResetsRetryBudget(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	for range authorityRetryMaxAttempts {
		hub.amanAuthorityRefresh.reserveRetry(forcedFMPTestSession)
		hub.amanAuthorityRefresh.finishRetry(forcedFMPTestSession)
	}
	require.False(t, hub.amanAuthorityRefresh.reserveRetry(forcedFMPTestSession), "budget exhausted")

	hub.sendAMANFMPAuthority(context.Background(), client)

	require.True(t, hub.amanAuthorityRefresh.reserveRetry(forcedFMPTestSession), "a successful initial lookup resets the budget")
}

func TestFailedDeleteIsRetriedInTheBackground(t *testing.T) {
	previous := forcedFMPClearRetryDelay
	forcedFMPClearRetryDelay = 10 * time.Millisecond
	t.Cleanup(func() { forcedFMPClearRetryDelay = previous })
	store := &lockedFailingClearStore{grants: map[int32]aman.ForcedFMPGrant{}}
	online := []*internalModels.Controller{{ID: 7, Callsign: "EKCH_W_APP"}}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)

	// An FMP logs on and the delete fails; nothing touches the grant again.
	store.setFail(true)
	hub.resetForcedFMPOnControllerOnline(forcedFMPTestSession, "EKDK_FMP", "131.040")
	require.True(t, store.has(forcedFMPTestSession))

	// The database recovers: the background retry removes the row without
	// any further access, so a later restart has nothing to restore.
	store.setFail(false)
	require.Eventually(t, func() bool { return !store.has(forcedFMPTestSession) }, time.Second, 5*time.Millisecond)
}

func TestForceFMPUndoneWhenHolderLogsOffDuringForce(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	lookups := 0
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*internalModels.Controller, error) {
			lookups++
			// Presence check, record ID read, presence recheck: holder online.
			// Final record recheck: holder has logged off.
			if lookups <= 3 {
				return []*internalModels.Controller{{ID: 7, Callsign: "EKCH_W_APP"}}, nil
			}
			return nil, nil
		},
	}}

	sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)

	require.Equal(t, string(aman.ErrorInvalidTransition), lastRejection(t, client).Data.Code)
	_, held := hub.forcedFMP.get(context.Background(), forcedFMPTestSession)
	require.False(t, held)
}

func TestRestartDropsStoredGrantWhileFMPOnline(t *testing.T) {
	store := &memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{
		forcedFMPTestSession: {SessionID: forcedFMPTestSession, Airport: "EKCH", Callsign: "EKCH_W_APP", CID: "1234567", ControllerID: 7},
	}}
	online := []*internalModels.Controller{{ID: 7, Callsign: "EKCH_W_APP"}, {ID: 9, Callsign: "EKDK_FMP", Position: "131.040"}}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)

	require.False(t, hub.hasAMANFMPAuthority(client))
	require.Empty(t, store.grants)
}

func TestStoredGrantNotUsedWhilePresenceUnknown(t *testing.T) {
	store := &memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{
		forcedFMPTestSession: {SessionID: forcedFMPTestSession, Airport: "EKCH", Callsign: "EKCH_W_APP", CID: "1234567", ControllerID: 7},
	}}
	online := []*internalModels.Controller{}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	failControllerLookup(hub)

	require.False(t, hub.hasAMANFMPAuthority(client), "fail closed while presence is unknown")
	require.Contains(t, store.grants, forcedFMPTestSession, "kept to be checked again later")
}

func TestForceFMPUndoneWhenFMPLogsOnDuringForce(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	lookups := 0
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*internalModels.Controller, error) {
			lookups++
			if lookups == 1 {
				return nil, nil
			}
			return []*internalModels.Controller{{Callsign: "EKDK_FMP", Position: "131.040"}}, nil
		},
	}}

	sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)

	require.Equal(t, string(aman.ErrorInvalidTransition), lastRejection(t, client).Data.Code)
	require.False(t, hub.hasAMANFMPAuthority(client))
}

func TestFMPPresenceUnknownWithoutControllerRepository(t *testing.T) {
	hub := &Hub{}
	_, err := hub.fmpOnline(context.Background(), forcedFMPTestSession)
	require.ErrorIs(t, err, errControllerPresenceUnavailable)
}

func TestAuthorityRetriesAreBounded(t *testing.T) {
	queue := newSessionRefreshQueue()
	require.True(t, queue.reserveRetry(1))
	require.False(t, queue.reserveRetry(1), "one pending retry per session")
	for attempt := 1; attempt < authorityRetryMaxAttempts; attempt++ {
		queue.finishRetry(1)
		require.True(t, queue.reserveRetry(1))
	}
	queue.finishRetry(1)
	require.False(t, queue.reserveRetry(1), "stops after the maximum")

	queue.presenceRecovered(1)
	require.True(t, queue.reserveRetry(1), "a successful lookup resets the budget")
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
