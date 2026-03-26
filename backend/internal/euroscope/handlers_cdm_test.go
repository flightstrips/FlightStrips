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

func (s *spyLocalCdmService) HandleReadyRequest(_ context.Context, _ int32, _ string) error {
	panic("HandleReadyRequest should not be called in this test")
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

func (s *spyLocalCdmService) HandleApproveReqTobt(_ context.Context, _ int32, _ string, _ string, _ string) error {
	panic("HandleApproveReqTobt should not be called in this test")
}

func (s *spyLocalCdmService) SyncAsatForGroundState(_ context.Context, _ int32, _ string, _ string) error {
	panic("SyncAsatForGroundState should not be called in this test")
}

func (s *spyLocalCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("RequestBetterTobt should not be called in this test")
}

func (s *spyLocalCdmService) SetSessionCdmMaster(_ context.Context, _ int32, _ bool) error {
	panic("SetSessionCdmMaster should not be called in this test")
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

// ---- handleCdmMasterToggle ----

type spyCdmMasterToggleService struct {
	spyLocalCdmService
	masterCalled  bool
	masterSession int32
	masterValue   bool
}

func (s *spyCdmMasterToggleService) SetSessionCdmMaster(_ context.Context, sessionID int32, master bool) error {
	s.masterCalled = true
	s.masterSession = sessionID
	s.masterValue = master
	return nil
}

func TestHandleCdmMasterToggle_TrueCallsSetSessionCdmMaster(t *testing.T) {
	cdmService := &spyCdmMasterToggleService{}
	server := &testutil.MockServer{CdmServiceVal: cdmService}
	hub := &Hub{server: server, master: map[int32]*Client{}}
	client := &Client{hub: hub, session: int32(42)}

	payload, err := json.Marshal(euroscopeEvents.CdmMasterToggleEvent{Master: true})
	require.NoError(t, err)

	err = handleCdmMasterToggle(context.Background(), client, Message{
		Type:    euroscopeEvents.CdmMasterToggle,
		Message: payload,
	})
	require.NoError(t, err)
	assert.True(t, cdmService.masterCalled)
	assert.Equal(t, int32(42), cdmService.masterSession)
	assert.True(t, cdmService.masterValue)
}

func TestHandleCdmMasterToggle_FalseCallsSetSessionCdmMaster(t *testing.T) {
	cdmService := &spyCdmMasterToggleService{}
	server := &testutil.MockServer{CdmServiceVal: cdmService}
	hub := &Hub{server: server, master: map[int32]*Client{}}
	client := &Client{hub: hub, session: int32(99)}

	payload, err := json.Marshal(euroscopeEvents.CdmMasterToggleEvent{Master: false})
	require.NoError(t, err)

	err = handleCdmMasterToggle(context.Background(), client, Message{
		Type:    euroscopeEvents.CdmMasterToggle,
		Message: payload,
	})
	require.NoError(t, err)
	assert.True(t, cdmService.masterCalled)
	assert.Equal(t, int32(99), cdmService.masterSession)
	assert.False(t, cdmService.masterValue)
}

