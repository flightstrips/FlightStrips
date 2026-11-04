package euroscope

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAutomaticExecutorWaitsForActualRestorationAndRoleAnnouncement(t *testing.T) {
	hub := membershipTestHub()
	masterRoot := membershipTestRoot(hub, "123", "master")
	master := membershipTestMember(hub, masterRoot, 1, "EKCH", "LIVE", "master")
	joiningRoot := membershipTestRoot(hub, "456", "joining")
	joining := membershipTestMember(hub, joiningRoot, 1, "EKCH", "LIVE", "joining")
	// A real JoinMembership starts both flags false; drive the production
	// OnRegister restoration, delay, and role publication from that state.
	joining.announced.Store(false)
	joining.roleEligible.Store(false)
	joining.workflow = backendWorkflowFixture()
	hub.master[1] = master
	reading, release := make(chan struct{}), make(chan struct{})
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) {
		close(reading)
		<-release
		return nil, nil
	}}
	hub.OnRegister(joining)
	<-reading
	_, err := hub.ResolveAutomaticCommandBinding(1, "456")
	require.Error(t, err, "restoration has not completed")
	restoredAt := time.Now()
	close(release)
	// The first MasterInfo and restoration are sent before the startup delay.
	for {
		wire, err := (<-joiningRoot.send).Marshal()
		require.NoError(t, err)
		var envelope es.Envelope
		require.NoError(t, proto.Unmarshal(wire, &envelope))
		if envelope.GetBackendSync() != nil {
			break
		}
	}
	_, err = hub.ResolveAutomaticCommandBinding(1, "456")
	require.Error(t, err, "successful restoration must retain the startup delay")
	require.Eventually(t, func() bool { return joining.announced.Load() && joining.roleEligible.Load() }, 10*time.Second, time.Millisecond)
	require.GreaterOrEqual(t, time.Since(restoredAt), 1900*time.Millisecond)
	binding, err := hub.ResolveAutomaticCommandBinding(1, "456")
	require.NoError(t, err)
	require.Same(t, joining, binding.(*commandBinding).client)
	// Already authorized human results remain valid while a new master inventory
	// is pending; only new own commands require session-full-sync readiness.
	hub.syncedSessions.Delete(1)
	_, err = shared.WithHumanCommand(context.Background(), &commandBinding{hub: hub, client: joining, session: 1, cid: "456"})
	require.NoError(t, err)
}

func TestRestorationSnapshotOrdersElectionWithoutBlockingOtherSession(t *testing.T) {
	hub := membershipTestHub()
	hub.send = make(chan internalMessage, 4)
	oneRoot := membershipTestRoot(hub, "123", "one")
	a := membershipTestMember(hub, oneRoot, 1, "EKCH", "LIVE", "a")
	twoRoot := membershipTestRoot(hub, "456", "two")
	b := membershipTestMember(hub, twoRoot, 2, "EKCH", "LIVE", "b")
	replacementRoot := membershipTestRoot(hub, "789", "replacement")
	replacement := membershipTestMember(hub, replacementRoot, 1, "EKCH", "LIVE", "replacement")
	hub.master[1] = a
	a.workflow, b.workflow = backendWorkflowFixture(), backendWorkflowFixture()
	reading, release := make(chan struct{}), make(chan struct{})
	seen := time.Now()
	hub.server.(*testutil.MockServer).StripRepoVal = &testutil.MockStripRepository{
		ListFn: func(_ context.Context, session int32) ([]*models.Strip, error) {
			if session == 1 {
				close(reading)
				<-release
			}
			return []*models.Strip{{Callsign: "SAS123", EuroscopeSeenAt: &seen, AssignedSquawk: stringPointer("1234")}}, nil
		},
		UpdateFsScratchPadFn: func(context.Context, int32, string, string) (int64, error) { return 1, nil },
	}
	oldEpoch := a.wireContext().MasterEpoch
	restored := make(chan bool, 1)
	go func() { restored <- hub.sendBackendSyncIfNeeded(a) }()
	<-reading
	elected := make(chan struct{})
	go func() { hub.setMasterClient(replacement); close(elected) }()
	select {
	case <-elected:
		close(release)
		t.Fatal("election passed a restoration snapshot still being assembled")
	case <-time.After(30 * time.Millisecond):
	}
	otherRestored := make(chan bool, 1)
	go func() { otherRestored <- hub.sendBackendSyncIfNeeded(b) }()
	select {
	case ok := <-otherRestored:
		require.True(t, ok)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("airport B restoration waited for airport A database read")
	}
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.FsScratchPad, handleFsScratchPad)
	edited := make(chan error, 1)
	go func() {
		edited <- handlers.Handle(context.Background(), b, authorityMessage(t, es.FsScratchPad, &es.FsScratchPadEvent{Callsign: "SAS123", Text: "NOTE"}, b.wireContext()))
	}()
	select {
	case err := <-edited:
		require.NoError(t, err)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("airport B scratchpad edit waited for airport A restoration")
	}
	close(release)
	require.True(t, <-restored)
	<-elected
	wire, err := (<-oneRoot.send).Marshal()
	require.NoError(t, err)
	var snapshot es.Envelope
	require.NoError(t, proto.Unmarshal(wire, &snapshot))
	require.NotNil(t, snapshot.GetBackendSync())
	require.Equal(t, oldEpoch, snapshot.Context.MasterEpoch)
	require.Greater(t, a.wireContext().MasterEpoch, snapshot.Context.MasterEpoch)
}

func TestQueuedAutomaticNonMasterCannotAdoptNewElection(t *testing.T) {
	hub := membershipTestHub()
	masterRoot := membershipTestRoot(hub, "123", "master")
	master := membershipTestMember(hub, masterRoot, 1, "EKCH", "LIVE", "master")
	ownRoot := membershipTestRoot(hub, "456", "assuming")
	own := membershipTestMember(hub, ownRoot, 1, "EKCH", "LIVE", "assuming")
	replacementRoot := membershipTestRoot(hub, "789", "replacement")
	replacement := membershipTestMember(hub, replacementRoot, 1, "EKCH", "LIVE", "replacement")
	hub.master[1] = master
	hub.squawkThrottle = newSquawkThrottle(time.Hour, nil, hub.dispatchGenerateSquawkRequest)
	t.Cleanup(func() {
		hub.squawkThrottle.mu.Lock()
		defer hub.squawkThrottle.mu.Unlock()
		if timer := hub.squawkThrottle.states[1].timer; timer != nil {
			timer.Stop()
		}
	})
	binding, err := hub.ResolveAutomaticCommandBinding(1, "456")
	require.NoError(t, err)
	require.Same(t, own, binding.(*commandBinding).client)
	require.NoError(t, binding.Send(es.GenerateSquawkEvent{Callsign: "FIRST"}))
	<-ownRoot.send
	require.NoError(t, binding.Send(es.GenerateSquawkEvent{Callsign: "QUEUED"}))
	hub.setMasterClient(replacement)
	consumeAutomaticTestElection(t, masterRoot, ownRoot, replacementRoot)
	hub.squawkThrottle.flush(1)
	require.Empty(t, ownRoot.send, "automatic assuming-controller work retains its original epoch")
	require.Error(t, binding.Send(es.ClearedFlagEvent{Callsign: "SAS123", Cleared: true}))
	// Fresh automatic work can still use this eligible nonmaster without waiting
	// for a new master full sync; it simply captures the current election.
	fresh, err := hub.ResolveAutomaticCommandBinding(1, "456")
	require.NoError(t, err)
	require.NoError(t, fresh.Send(es.ClearedFlagEvent{Callsign: "SAS123", Cleared: true}))
	require.Len(t, ownRoot.send, 1)
}

type blockedHoldingRestoration struct{ reading, release chan struct{} }

func (p blockedHoldingRestoration) CurrentAMANHoldingEAT(context.Context, string) ([]es.HoldEvent, error) {
	close(p.reading)
	<-p.release
	return []es.HoldEvent{{Callsign: "SAS123", Hold: "OLPIB", HoldEat: "1422"}}, nil
}

func TestInitialHoldingRestorationRetainsItsElection(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "one")
	a := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "a")
	other := membershipTestRoot(hub, "456", "two")
	b := membershipTestMember(hub, other, 1, "EKCH", "LIVE", "b")
	hub.master[1] = a
	provider := blockedHoldingRestoration{reading: make(chan struct{}), release: make(chan struct{})}
	hub.amanHoldingEAT = provider
	oldEpoch := a.wireContext().MasterEpoch
	sent := make(chan struct{})
	go func() { hub.sendInitialAMANHoldingEAT(a); close(sent) }()
	<-provider.reading
	elected := make(chan struct{})
	go func() { hub.setMasterClient(b); close(elected) }()
	select {
	case <-elected:
		close(provider.release)
		t.Fatal("holding restoration was relabeled by an election during its read")
	case <-time.After(30 * time.Millisecond):
	}
	close(provider.release)
	<-sent
	<-elected
	wire, err := (<-root.send).Marshal()
	require.NoError(t, err)
	var envelope es.Envelope
	require.NoError(t, proto.Unmarshal(wire, &envelope))
	require.NotNil(t, envelope.GetHold())
	require.Equal(t, oldEpoch, envelope.Context.MasterEpoch)
}

type suspendedPositionService struct {
	noOpStripService
	waiting, finish chan struct{}
}

func (s *suspendedPositionService) UpdateAircraftPosition(ctx context.Context, _ int32, callsign string, _, _ float64, _ int32, _ string) error {
	if callsign == "FIRST" {
		resume := shared.SuspendPositionExecution(ctx)
		close(s.waiting)
		<-s.finish
		resume()
	}
	return nil
}

func TestScopedPositionBatchYieldsExecutionWhileElectionWaits(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "one")
	a := membershipTestMember(hub, root, 1, "EKCH", "LIVE", "a")
	other := membershipTestRoot(hub, "456", "two")
	b := membershipTestMember(hub, other, 1, "EKCH", "LIVE", "b")
	hub.master[1] = a
	service := &suspendedPositionService{waiting: make(chan struct{}), finish: make(chan struct{})}
	hub.stripService = service
	dispatcher := shared.NewBatchPositionDispatcher(1, 16, make(chan struct{}, 1), postgres.NewPositionBatch)
	defer dispatcher.Cancel()
	var finishOnce sync.Once
	finish := func() { finishOnce.Do(func() { close(service.finish) }) }
	defer finish()
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.PositionUpdate, handlePositionUpdate)
	scope := a.wireContext()
	firstDone, secondStarted, secondDone := make(chan error, 1), make(chan struct{}), make(chan error, 1)
	first := authorityMessage(t, es.PositionUpdate, &es.AircraftPositionUpdateEvent{Callsign: "FIRST"}, scope)
	second := authorityMessage(t, es.PositionUpdate, &es.AircraftPositionUpdateEvent{Callsign: "SECOND"}, scope)
	require.NoError(t, dispatcher.Submit(context.Background(), "FIRST", func(ctx context.Context) { firstDone <- handlers.Handle(ctx, a, first) }))
	<-service.waiting
	elected := make(chan struct{})
	go func() { hub.setMasterClient(b); close(elected) }()
	require.Eventually(t, func() bool {
		if hub.election(1).gate.TryRLock() {
			hub.election(1).gate.RUnlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond)
	require.NoError(t, dispatcher.Submit(context.Background(), "SECOND", func(ctx context.Context) {
		close(secondStarted)
		secondDone <- handlers.Handle(ctx, a, second)
	}))
	<-secondStarted
	finish()
	select {
	case err := <-firstDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("position completion could not reacquire a slot held by a gated report")
	}
	select {
	case <-elected:
	case <-time.After(time.Second):
		t.Fatal("election remained blocked by suspended position work")
	}
	select {
	case err := <-secondDone:
		require.Error(t, err, "queued old-epoch report must be rejected after election")
	case <-time.After(time.Second):
		t.Fatal("later report never resumed after election")
	}
}
