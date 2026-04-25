package euroscope

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGenerateSquawkCid_PrefersConnectedDelOwner(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_DEL", Frequency: "121.730"},
	}))
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	delCid := "DEL-CID"
	controllerRepo := &testutil.MockControllerRepository{
		GetByPositionFn: func(_ context.Context, session int32, position string) ([]*models.Controller, error) {
			assert.Equal(t, int32(7), session)
			assert.Equal(t, "121.730", position)
			return []*models.Controller{{Callsign: "EKCH_DEL", Position: position, Cid: &delCid}}, nil
		},
	}
	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
			assert.Equal(t, int32(7), session)
			return []*models.SectorOwner{
				{Sector: []string{"DEL"}, Position: "121.730"},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: controllerRepo,
			SectorRepoVal:     sectorRepo,
		},
		master: map[int32]*Client{
			7: {user: shared.NewAuthenticatedUser("MASTER-CID", 0, nil)},
		},
	}

	assert.Equal(t, delCid, hub.resolveGenerateSquawkCid(context.Background(), 7))
}

func TestResolveGenerateSquawkCid_FallsBackToMasterWhenDelOwnerHasNoEsClient(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_DEL", Frequency: "121.730"},
	}))
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	controllerRepo := &testutil.MockControllerRepository{
		GetByPositionFn: func(_ context.Context, _ int32, _ string) ([]*models.Controller, error) {
			return []*models.Controller{{Callsign: "EKCH_DEL", Position: "121.730"}}, nil
		},
	}
	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Sector: []string{"DEL"}, Position: "121.730"},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: controllerRepo,
			SectorRepoVal:     sectorRepo,
		},
		master: map[int32]*Client{
			7: {user: shared.NewAuthenticatedUser("MASTER-CID", 0, nil)},
		},
	}

	assert.Equal(t, "MASTER-CID", hub.resolveGenerateSquawkCid(context.Background(), 7))
}

func TestResolveGenerateSquawkCid_IgnoresControllersWithWrongPrefixOnDelFrequency(t *testing.T) {
	t.Cleanup(config.SetPositionsForTest([]config.Position{
		{Name: "EKCH_DEL", Frequency: "121.730"},
	}))
	t.Cleanup(config.SetOwnerCallsignPrefixesForTest([]string{"EKCH", "EKDK"}))

	wrongCid := "WRONG-CID"
	delCid := "DEL-CID"
	controllerRepo := &testutil.MockControllerRepository{
		GetByPositionFn: func(_ context.Context, _ int32, position string) ([]*models.Controller, error) {
			return []*models.Controller{
				{Callsign: "ESMS_DEL", Position: position, Cid: &wrongCid},
				{Callsign: "EKCH_DEL", Position: position, Cid: &delCid},
			}, nil
		},
	}
	sectorRepo := &testutil.MockSectorOwnerRepository{
		ListBySessionFn: func(_ context.Context, _ int32) ([]*models.SectorOwner, error) {
			return []*models.SectorOwner{
				{Sector: []string{"DEL"}, Position: "121.730"},
			}, nil
		},
	}

	hub := &Hub{
		server: &testutil.MockServer{
			ControllerRepoVal: controllerRepo,
			SectorRepoVal:     sectorRepo,
		},
		master: map[int32]*Client{
			7: {user: shared.NewAuthenticatedUser("MASTER-CID", 0, nil)},
		},
	}

	assert.Equal(t, delCid, hub.resolveGenerateSquawkCid(context.Background(), 7))
}

func TestSendGenerateSquawk_RateLimitsPerSession(t *testing.T) {
	t.Parallel()

	hub := &Hub{
		send: make(chan internalMessage, 4),
	}
	hub.squawkThrottle = newSquawkThrottle(25*time.Millisecond, hub.readAssignedSquawk, hub.dispatchGenerateSquawkRequest)

	hub.SendGenerateSquawk(7, "CID-1", "SAS101")
	first := readGenerateSquawkMessage(t, hub.send, 50*time.Millisecond)
	assert.Equal(t, "CID-1", *first.cid)
	assert.Equal(t, "SAS101", first.message.(euroscopeEvents.GenerateSquawkEvent).Callsign)

	start := time.Now()
	hub.SendGenerateSquawk(7, "CID-2", "SAS202")

	select {
	case message := <-hub.send:
		t.Fatalf("generate_squawk sent too early: %+v", message)
	case <-time.After(10 * time.Millisecond):
	}

	second := readGenerateSquawkMessage(t, hub.send, 200*time.Millisecond)
	assert.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond)
	assert.Equal(t, "CID-2", *second.cid)
	assert.Equal(t, "SAS202", second.message.(euroscopeEvents.GenerateSquawkEvent).Callsign)
}

func TestSendGenerateSquawk_SendsQueuedRequestWhenAssignedSquawkRemainsInvalid(t *testing.T) {
	t.Parallel()

	queuedSquawk := "2000"
	updatedSquawk := "2200"
	callCounts := map[string]int{}
	currentSAS303 := queuedSquawk
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, int32(7), session)
			callCounts[callsign]++
			switch callsign {
			case "SAS101":
				return &models.Strip{Callsign: callsign, AssignedSquawk: &queuedSquawk}, nil
			case "SAS303":
				return &models.Strip{Callsign: callsign, AssignedSquawk: &currentSAS303}, nil
			default:
				t.Fatalf("unexpected callsign %s", callsign)
				return nil, nil
			}
		},
	}

	hub := &Hub{
		send:   make(chan internalMessage, 4),
		server: &testutil.MockServer{StripRepoVal: stripRepo},
	}
	hub.squawkThrottle = newSquawkThrottle(25*time.Millisecond, hub.readAssignedSquawk, hub.dispatchGenerateSquawkRequest)

	hub.SendGenerateSquawk(7, "CID-1", "SAS101")
	_ = readGenerateSquawkMessage(t, hub.send, 50*time.Millisecond)

	start := time.Now()
	hub.SendGenerateSquawk(7, "CID-2", "SAS303")
	currentSAS303 = updatedSquawk

	second := readGenerateSquawkMessage(t, hub.send, 200*time.Millisecond)

	assert.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond)
	assert.Equal(t, "CID-2", *second.cid)
	assert.Equal(t, "SAS303", second.message.(euroscopeEvents.GenerateSquawkEvent).Callsign)
	assert.Equal(t, 1, callCounts["SAS303"])
}

func TestSendGenerateSquawk_SkipsQueuedRequestWhenAssignedSquawkBecomesValid(t *testing.T) {
	t.Parallel()

	queuedSquawk := "2000"
	updatedSquawk := "2401"
	callCounts := map[string]int{}
	currentSAS303 := queuedSquawk
	stripRepo := &testutil.MockStripRepository{
		GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
			assert.Equal(t, int32(7), session)
			callCounts[callsign]++
			switch callsign {
			case "SAS101":
				return &models.Strip{Callsign: callsign, AssignedSquawk: &queuedSquawk}, nil
			case "SAS303":
				return &models.Strip{Callsign: callsign, AssignedSquawk: &currentSAS303}, nil
			default:
				t.Fatalf("unexpected callsign %s", callsign)
				return nil, nil
			}
		},
	}

	hub := &Hub{
		send:   make(chan internalMessage, 4),
		server: &testutil.MockServer{StripRepoVal: stripRepo},
	}
	hub.squawkThrottle = newSquawkThrottle(25*time.Millisecond, hub.readAssignedSquawk, hub.dispatchGenerateSquawkRequest)

	hub.SendGenerateSquawk(7, "CID-1", "SAS101")
	_ = readGenerateSquawkMessage(t, hub.send, 50*time.Millisecond)

	start := time.Now()
	hub.SendGenerateSquawk(7, "CID-2", "SAS303")
	currentSAS303 = updatedSquawk

	select {
	case message := <-hub.send:
		t.Fatalf("queued generate_squawk should have been skipped after becoming valid: %+v", message)
	case <-time.After(80 * time.Millisecond):
	}

	assert.GreaterOrEqual(t, time.Since(start), 80*time.Millisecond)
	assert.Equal(t, 1, callCounts["SAS303"])
}

func readGenerateSquawkMessage(t *testing.T, ch <-chan internalMessage, timeout time.Duration) internalMessage {
	t.Helper()

	select {
	case message := <-ch:
		require.NotNil(t, message.cid)
		require.IsType(t, euroscopeEvents.GenerateSquawkEvent{}, message.message)
		return message
	case <-time.After(timeout):
		t.Fatal("timed out waiting for generate_squawk message")
		return internalMessage{}
	}
}
