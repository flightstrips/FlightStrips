package euroscope

import (
	"context"
	"encoding/json"
	"testing"

	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyLocalCdmService struct {
	session        int32
	tobtCallsign   string
	tobtValue      string
	tobtSourcePos  string
	tobtSourceRole string
	called         bool
}

func (s *spyLocalCdmService) TriggerRecalculate(_ context.Context, _ int32, _ string) {
	panic("TriggerRecalculate should not be called in this test")
}

func (s *spyLocalCdmService) SyncAirportLvoFromRunwayStatus(_ context.Context, _ string, _ map[string]string) {
	panic("SyncAirportLvoFromRunwayStatus should not be called in this test")
}

func (s *spyLocalCdmService) DeregisterMasterAirport(context.Context, string) error {
	panic("DeregisterMasterAirport should not be called in this test")
}

func (s *spyLocalCdmService) HandleReadyRequest(_ context.Context, _ int32, _ string, _ string, _ string) error {
	panic("HandleReadyRequest should not be called in this test")
}

func (s *spyLocalCdmService) HandleEobtUpdate(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	panic("HandleEobtUpdate should not be called in this test")
}

func (s *spyLocalCdmService) HandleTobtUpdate(_ context.Context, session int32, callsign string, tobt string, sourcePosition string, sourceRole string) error {
	s.called = true
	s.session = session
	s.tobtCallsign = callsign
	s.tobtValue = tobt
	s.tobtSourcePos = sourcePosition
	s.tobtSourceRole = sourceRole
	return nil
}

func (s *spyLocalCdmService) HandleClxTobtUpdate(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	panic("HandleClxTobtUpdate should not be called in this test")
}

func (s *spyLocalCdmService) HandleDeiceUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleDeiceUpdate should not be called in this test")
}

func (s *spyLocalCdmService) HandleAsrtToggle(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleAsrtToggle should not be called in this test")
}

func (s *spyLocalCdmService) HandleTsacUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleTsacUpdate should not be called in this test")
}

func (s *spyLocalCdmService) HandleManualCtot(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleManualCtot should not be called in this test")
}

func (s *spyLocalCdmService) HandleCtotRemove(_ context.Context, _ int32, _ string) error {
	panic("HandleCtotRemove should not be called in this test")
}

func (s *spyLocalCdmService) SyncAsatForGroundState(_ context.Context, _ int32, _ string, _ string) error {
	panic("SyncAsatForGroundState should not be called in this test")
}

func (s *spyLocalCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("RequestBetterTobt should not be called in this test")
}

func TestHandleCdmTobtUpdate_ForwardsValidatedEvent(t *testing.T) {
	cdmService := &spyLocalCdmService{}
	server := &testutil.MockServer{CdmServiceVal: cdmService}
	hub := &Hub{server: server, master: map[int32]*Client{}}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_B_GND",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}
	hub.master[42] = client

	payload, err := json.Marshal(euroscopeEvents.CdmTobtUpdateEvent{
		Callsign: "SAS321",
		Tobt:     "1030",
	})
	require.NoError(t, err)

	err = handleCdmTobtUpdate(context.Background(), client, Message{
		Type:    euroscopeEvents.CdmTobtUpdate,
		Message: payload,
	})
	require.NoError(t, err)
	assert.True(t, cdmService.called)
	assert.Equal(t, int32(42), cdmService.session)
	assert.Equal(t, "SAS321", cdmService.tobtCallsign)
	assert.Equal(t, "1030", cdmService.tobtValue)
	assert.Equal(t, "EKCH_B_GND", cdmService.tobtSourcePos)
	assert.Equal(t, "master", cdmService.tobtSourceRole)
}

func TestHandleCdmTobtUpdate_IgnoresInvalidClock(t *testing.T) {
	cdmService := &spyLocalCdmService{}
	server := &testutil.MockServer{CdmServiceVal: cdmService}
	hub := &Hub{server: server, master: map[int32]*Client{}}
	client := &Client{hub: hub, session: 42, callsign: "EKCH_B_GND"}

	payload, err := json.Marshal(euroscopeEvents.CdmTobtUpdateEvent{
		Callsign: "SAS321",
		Tobt:     "25:00",
	})
	require.NoError(t, err)

	err = handleCdmTobtUpdate(context.Background(), client, Message{
		Type:    euroscopeEvents.CdmTobtUpdate,
		Message: payload,
	})
	require.NoError(t, err)
	assert.False(t, cdmService.called)
}
