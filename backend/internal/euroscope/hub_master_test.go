package euroscope

import (
	"sync"
	"testing"

	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	baseEvents "FlightStrips/pkg/events"
	"FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func masterTestClient(session int32, callsign, cid string) *Client {
	return startQueuedTestClient(&Client{
		session:  session,
		callsign: callsign,
		user:     shared.NewAuthenticatedUser(cid, 0, nil),
		send:     make(chan baseEvents.OutgoingMessage, 8),
	})
}

func lastMasterTestRole(t *testing.T, client *Client) euroscope.SessionInfoRole {
	t.Helper()
	var role euroscope.SessionInfoRole
	for len(client.send) > 0 {
		role = (<-client.send).(euroscope.SessionInfoEvent).Role
	}
	return role
}

func TestPreferredMasterClient_UsesOperationalPriorityOrder(t *testing.T) {
	const session = int32(42)
	other := masterTestClient(session, "EKCH_D_TWR", "other")
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	ucCtr := masterTestClient(session, "EKDK_UC_CTR", "uc-ctr")
	dCtr := masterTestClient(session, "EKDK_D_CTR", "d-ctr")
	bCtr := masterTestClient(session, "EKDK_B_CTR", "b-ctr")
	fmp := masterTestClient(session, "EKDK_FMP", "fmp")

	assert.Same(t, fmp, preferredMasterClient([]*Client{other, aTower, ucCtr, dCtr, bCtr, fmp}, session, nil))
	assert.Same(t, bCtr, preferredMasterClient([]*Client{other, aTower, ucCtr, dCtr, bCtr}, session, nil))
	assert.Same(t, dCtr, preferredMasterClient([]*Client{other, aTower, ucCtr, dCtr}, session, nil))
	assert.Same(t, ucCtr, preferredMasterClient([]*Client{other, aTower, ucCtr}, session, nil))
	assert.Same(t, aTower, preferredMasterClient([]*Client{other, aTower}, session, nil))
	assert.Same(t, other, preferredMasterClient([]*Client{other}, session, nil))
}

func TestMasterClientPriority_RecognizesPreferredFrequencies(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKDK_B_CTR", Frequency: "119.555"},
		{Name: "EKDK_D_CTR", Frequency: "133.155"},
		{Name: "EKDK_UC_CTR", Frequency: "127.865"},
		{Name: "EKCH_A_TWR", Frequency: "118.105"},
	}))

	fmp := masterTestClient(42, "CUSTOM_FMP_ALIAS", "fmp")
	fmp.position = "131.040"
	bCtr := masterTestClient(42, "CUSTOM_B_ALIAS", "b-ctr")
	bCtr.position = "119.555"
	dCtr := masterTestClient(42, "CUSTOM_D_ALIAS", "d-ctr")
	dCtr.position = "133.155"
	ucCtr := masterTestClient(42, "CUSTOM_UC_ALIAS", "uc-ctr")
	ucCtr.position = "127.865"
	aTower := masterTestClient(42, "CUSTOM_TOWER_ALIAS", "tower")
	aTower.position = "118.105"

	assert.Equal(t, fmpMasterPriority, masterClientPriority(fmp))
	assert.Equal(t, bCtrMasterPriority, masterClientPriority(bCtr))
	assert.Equal(t, dCtrMasterPriority, masterClientPriority(dCtr))
	assert.Equal(t, ucCtrMasterPriority, masterClientPriority(ucCtr))
	assert.Equal(t, aTowerMasterPriority, masterClientPriority(aTower))
}

func TestMasterClientPriority_UsesAtomicIdentitySnapshot(t *testing.T) {
	client := masterTestClient(42, "EKCH_A_TWR", "controller")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 1_000 {
			client.updateIdentity("131.040", "EKDK_FMP", false, "")
			client.updateIdentity("118.105", "EKCH_A_TWR", false, "")
		}
	}()
	go func() {
		defer wg.Done()
		for range 1_000 {
			priority := masterClientPriority(client)
			assert.Contains(t, []int{fmpMasterPriority, aTowerMasterPriority}, priority)
		}
	}()
	wg.Wait()
}

func TestPromoteMasterIfPreferred_FMPPreemptsATower(t *testing.T) {
	const session = int32(42)
	hub := &Hub{master: make(map[int32]*Client)}
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	fmp := masterTestClient(session, "EKDK_FMP", "fmp")
	hub.master[session] = aTower

	require.True(t, hub.promoteMasterIfPreferred(fmp, true))
	assert.Same(t, fmp, hub.getMasterClient(session))

	oldRole := (<-aTower.send).(euroscope.SessionInfoEvent)
	newRole := (<-fmp.send).(euroscope.SessionInfoEvent)
	assert.Equal(t, euroscope.SessionInfoSlave, oldRole.Role)
	assert.Equal(t, euroscope.SessionInfoMaster, newRole.Role)
}

func TestPromoteMasterIfPreferred_ATowerCannotPreemptFMP(t *testing.T) {
	const session = int32(42)
	hub := &Hub{master: make(map[int32]*Client)}
	fmp := masterTestClient(session, "EKDK_FMP", "fmp")
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	hub.master[session] = fmp

	assert.False(t, hub.promoteMasterIfPreferred(aTower, true))
	assert.Same(t, fmp, hub.getMasterClient(session))
	assert.Empty(t, fmp.send)
	assert.Equal(t, euroscope.SessionInfoSlave, (<-aTower.send).(euroscope.SessionInfoEvent).Role)
}

func TestPromoteMasterIfPreferred_ATowerPreemptsFallback(t *testing.T) {
	const session = int32(42)
	hub := &Hub{master: make(map[int32]*Client)}
	fallback := masterTestClient(session, "EKCH_D_TWR", "fallback")
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	hub.master[session] = fallback

	require.True(t, hub.promoteMasterIfPreferred(aTower, true))
	assert.Same(t, aTower, hub.getMasterClient(session))
}

func TestPromoteRegisteredMasterIfPreferred_IgnoresDisconnectedClient(t *testing.T) {
	const session = int32(42)
	hub := &Hub{
		clients: make(map[*Client]bool),
		master:  make(map[int32]*Client),
	}
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	disconnectedFMP := masterTestClient(session, "EKDK_FMP", "fmp")
	hub.clients[aTower] = true
	hub.master[session] = aTower

	isMaster, stillRegistered := hub.promoteRegisteredMasterIfPreferred(disconnectedFMP)

	assert.False(t, isMaster)
	assert.False(t, stillRegistered)
	assert.Same(t, aTower, hub.getMasterClient(session))
	assert.Empty(t, aTower.send)
	assert.Empty(t, disconnectedFMP.send)
}

func TestPreferredMasterClient_IgnoresObserversAndOtherSessions(t *testing.T) {
	const session = int32(42)
	fallback := masterTestClient(session, "EKCH_D_TWR", "fallback")
	observerFMP := masterTestClient(session, "EKDK_FMP", "observer")
	observerFMP.observer = true
	otherSessionFMP := masterTestClient(99, "EKDK_FMP", "other-session")

	assert.Same(t, fallback, preferredMasterClient([]*Client{observerFMP, otherSessionFMP, fallback}, session, nil))
}

func TestReconsiderMasterAfterLogin_SelectsBestCandidate(t *testing.T) {
	const session = int32(42)
	hub := &Hub{
		clients: make(map[*Client]bool),
		master:  make(map[int32]*Client),
	}
	current := masterTestClient(session, "EKCH_D_TWR", "current")
	aTower := masterTestClient(session, "EKCH_A_TWR", "tower")
	bCtr := masterTestClient(session, "EKDK_B_CTR", "b-ctr")
	hub.clients[current] = true
	hub.clients[aTower] = true
	hub.clients[bCtr] = true
	hub.master[session] = current

	hub.reconsiderMasterAfterLogin(current)

	assert.Same(t, bCtr, hub.getMasterClient(session))
	assert.Equal(t, euroscope.SessionInfoSlave, (<-current.send).(euroscope.SessionInfoEvent).Role)
	assert.Equal(t, euroscope.SessionInfoMaster, (<-bCtr.send).(euroscope.SessionInfoEvent).Role)
}

func TestPromoteMasterIfPreferred_ConcurrentPromotionsKeepHighestPriority(t *testing.T) {
	const session = int32(42)
	for range 100 {
		hub := &Hub{master: make(map[int32]*Client)}
		fallback := masterTestClient(session, "EKCH_D_TWR", "fallback")
		bCtr := masterTestClient(session, "EKDK_B_CTR", "b-ctr")
		fmp := masterTestClient(session, "EKDK_FMP", "fmp")
		hub.master[session] = fallback

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			hub.promoteMasterIfPreferred(bCtr, true)
		}()
		go func() {
			defer wg.Done()
			hub.promoteMasterIfPreferred(fmp, true)
		}()
		wg.Wait()

		assert.Same(t, fmp, hub.getMasterClient(session))
		assert.Equal(t, euroscope.SessionInfoSlave, lastMasterTestRole(t, bCtr))
		assert.Equal(t, euroscope.SessionInfoMaster, lastMasterTestRole(t, fmp))
	}
}
