package euroscope

import (
	"testing"
	"time"

	"FlightStrips/internal/testutil"
	esEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"

	"github.com/stretchr/testify/require"
)

type masterInfoFrontendHub struct {
	testutil.MockFrontendHub
	sessions []int32
	updates  []frontendEvents.MasterInfoEvent
}

func (h *masterInfoFrontendHub) Broadcast(session int32, message frontendEvents.OutgoingMessage) {
	h.sessions = append(h.sessions, session)
	h.updates = append(h.updates, message.(frontendEvents.MasterInfoEvent))
}

type queuedMasterInfoFrontendHub struct {
	testutil.MockFrontendHub
	queue   chan frontendEvents.MasterInfoEvent
	started chan struct{}
	stop    chan struct{}
}

func (h *queuedMasterInfoFrontendHub) Broadcast(_ int32, message frontendEvents.OutgoingMessage) {
	h.started <- struct{}{}
	select {
	case h.queue <- message.(frontendEvents.MasterInfoEvent):
	case <-h.stop:
	}
}

func TestMasterInfoBackpressureAllowsRegistrationLookupAndHandover(t *testing.T) {
	frontend := &queuedMasterInfoFrontendHub{
		queue: make(chan frontendEvents.MasterInfoEvent, 1), started: make(chan struct{}, 2), stop: make(chan struct{}),
	}
	// Fill the frontend queue before publication, as when its dispatch loop is
	// initializing a client and has not yet drained pending messages.
	frontend.queue <- frontendEvents.MasterInfoEvent{}
	hub := &Hub{server: &testutil.MockServer{FrontendHubVal: frontend}, clients: map[*Client]bool{}}
	tower := masterTestClient(42, "EKCH_A_TWR", "tower")
	fmp := masterTestClient(42, "EKDK_FMP", "fmp")
	hub.clients[tower] = true
	hub.master = map[int32]*Client{42: tower}
	hub.masterCallsigns.Store(int32(42), tower.GetCallsign())

	firstDone := make(chan struct{})
	go func() {
		hub.broadcastMasterInfo(42)
		close(firstDone)
	}()
	t.Cleanup(func() { close(frontend.stop) })
	wait := func(done <-chan struct{}, operation string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s blocked by frontend backpressure", operation)
		}
	}
	wait(frontend.started, "initial broadcast")

	registered := make(chan struct{})
	go func() {
		hub.clientsMu.Lock()
		hub.clients[fmp] = true
		hub.clientsMu.Unlock()
		close(registered)
	}()
	wait(registered, "client registration")

	lookupDone := make(chan struct{})
	go func() {
		hub.GetFrontendController("fmp")
		close(lookupDone)
	}()
	wait(lookupDone, "frontend controller lookup")

	// Election must also remain available while publication is blocked. Its
	// metadata broadcast will follow the pending publication once space opens.
	handover := make(chan struct{})
	go func() {
		hub.promoteMasterCandidate(fmp, true)
		close(handover)
	}()
	wait(handover, "master handover")
	require.Equal(t, "EKDK_FMP", hub.GetMasterCallsign(42))
	secondDone := make(chan struct{})
	go func() {
		hub.broadcastMasterInfo(42)
		close(secondDone)
	}()

	<-frontend.queue // Let the frontend resume draining its queue.
	wait(firstDone, "initial publication")
	wait(frontend.started, "handover broadcast")
	require.Equal(t, "EKCH_A_TWR", (<-frontend.queue).MasterCallsign)
	wait(secondDone, "handover publication")
	require.Equal(t, "EKDK_FMP", (<-frontend.queue).MasterCallsign)
}

func TestMasterInfoFollowsElectionCallsignChangeAndLoss(t *testing.T) {
	frontend := &masterInfoFrontendHub{}
	hub := &Hub{server: &testutil.MockServer{FrontendHubVal: frontend}, clients: map[*Client]bool{}}
	tower := masterTestClient(42, "EKCH_A_TWR", "tower")
	fmp := masterTestClient(42, "EKDK_FMP", "fmp")
	observer := masterTestClient(42, "EKCH_OBS", "observer")
	observer.observer = true
	otherSession := masterTestClient(99, "EKYT_TWR", "other")
	for _, client := range []*Client{tower, fmp, observer, otherSession} {
		hub.clients[client] = true
	}
	check := func(want string) {
		t.Helper()
		for _, client := range []*Client{tower, fmp, observer} {
			var info *esEvents.MasterInfoEvent
			for len(client.send) > 0 {
				if event, ok := (<-client.send).(esEvents.MasterInfoEvent); ok {
					info = &event
				}
			}
			require.NotNil(t, info)
			require.Equal(t, want, info.MasterCallsign)
		}
		require.Empty(t, otherSession.send)
		require.Equal(t, int32(42), frontend.sessions[len(frontend.sessions)-1])
		require.Equal(t, want, frontend.updates[len(frontend.updates)-1].MasterCallsign)
	}
	hub.setMasterClient(tower)
	check("EKCH_A_TWR")
	isMaster, registered := hub.promoteRegisteredMasterIfPreferred(fmp)
	require.True(t, isMaster)
	require.True(t, registered)
	check("EKDK_FMP")
	fmp.updateIdentity("131.040", "CUSTOM_FMP", false, "")
	require.True(t, hub.refreshMasterClientIfCurrent(fmp))
	check("CUSTOM_FMP")
	require.True(t, hub.replaceMasterIfCurrent(fmp, tower))
	check("EKCH_A_TWR")
	require.True(t, hub.replaceMasterIfCurrent(tower, nil))
	check("")
}

func TestRunwayMismatchAlertIncludesMasterAndRefreshesOnHandover(t *testing.T) {
	hub := &Hub{runwayStates: map[string]*clientRunwayState{}}
	master := models.ActiveRunways{DepartureRunways: []string{"22R"}, ArrivalRunways: []string{"22L"}}
	current := models.ActiveRunways{DepartureRunways: []string{"04R"}}
	hub.masterCallsigns.Store(int32(42), "EKCH_A_TWR")
	result := hub.evaluateClientRunwayState(42, "slave", "EKCH_D_TWR", current, master, false)
	require.NotNil(t, result.Alert)
	require.Equal(t, "EKCH_A_TWR", result.Alert.MasterCallsign)
	require.Nil(t, hub.evaluateClientRunwayState(42, "slave", "EKCH_D_TWR", current, master, false).Alert)
	hub.masterCallsigns.Store(int32(42), "EKDK_FMP")
	result = hub.evaluateClientRunwayState(42, "slave", "EKCH_D_TWR", current, master, false)
	require.NotNil(t, result.Alert)
	require.Equal(t, "EKDK_FMP", result.Alert.MasterCallsign)
}
