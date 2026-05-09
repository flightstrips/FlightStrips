package frontend

import (
	"context"
	"encoding/json"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyCdmService struct {
	session     int32
	callsign    string
	sourcePos   string
	sourceRole  string
	called      bool
	clxSession  int32
	clxCallsign string
	clxTobt     string
	clxCalled   bool
}

func (s *spyCdmService) TriggerRecalculate(_ context.Context, _ int32, _ string) {
	panic("TriggerRecalculate should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleReadyRequest(_ context.Context, session int32, callsign string, sourcePosition string, sourceRole string) error {
	s.called = true
	s.session = session
	s.callsign = callsign
	s.sourcePos = sourcePosition
	s.sourceRole = sourceRole
	return nil
}

func (s *spyCdmService) HandleEobtUpdate(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	panic("HandleEobtUpdate should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleTobtUpdate(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	panic("HandleTobtUpdate should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleClxTobtUpdate(_ context.Context, session int32, callsign string, tobt string, _ string, _ string) error {
	s.clxCalled = true
	s.clxSession = session
	s.clxCallsign = callsign
	s.clxTobt = tobt
	return nil
}

func (s *spyCdmService) HandleDeiceUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleDeiceUpdate should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleAsrtToggle(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleAsrtToggle should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleTsacUpdate(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleTsacUpdate should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleManualCtot(_ context.Context, _ int32, _ string, _ string) error {
	panic("HandleManualCtot should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleCtotRemove(_ context.Context, _ int32, _ string) error {
	panic("HandleCtotRemove should not be called directly from handleCdmReady")
}

func (s *spyCdmService) HandleApproveReqTobt(_ context.Context, _ int32, _ string, _ string, _ string) error {
	panic("HandleApproveReqTobt should not be called directly from handleCdmReady")
}

func (s *spyCdmService) SyncAsatForGroundState(_ context.Context, _ int32, _ string, _ string) error {
	panic("SyncAsatForGroundState should not be called directly from handleCdmReady")
}

func (s *spyCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("RequestBetterTobt should not be called directly from handleCdmReady")
}

func (s *spyCdmService) SetSessionCdmMaster(_ context.Context, _ int32, _ bool) error {
	panic("SetSessionCdmMaster should not be called in this test")
}

func TestHandleCdmReady_UsesOrchestrationMethod(t *testing.T) {
	cdmService := &spyCdmService{}
	server := &testutil.MockServer{
		CdmServiceVal:  cdmService,
		FrontendHubVal: &testutil.MockFrontendHub{},
	}
	hub := &Hub{server: server}
	client := &Client{hub: hub, session: 42, position: "EKCH_DEL"}

	payload, err := json.Marshal(frontendEvents.CdmReadyEvent{Callsign: "SAS321"})
	require.NoError(t, err)

	err = handleCdmReady(context.Background(), client, Message{Message: payload})
	require.NoError(t, err)
	assert.True(t, cdmService.called)
	assert.Equal(t, int32(42), cdmService.session)
	assert.Equal(t, "SAS321", cdmService.callsign)
	assert.Equal(t, "EKCH_DEL", cdmService.sourcePos)
	assert.Equal(t, "ATC", cdmService.sourceRole)
}

func TestHandleClxUpdateTobt_UsesClxOrchestrationMethod(t *testing.T) {
	cdmService := &spyCdmService{}
	server := &testutil.MockServer{
		CdmServiceVal: cdmService,
		StripRepoVal: &testutil.MockStripRepository{
			GetByCallsignFn: func(_ context.Context, gotSession int32, gotCallsign string) (*models.Strip, error) {
				assert.Equal(t, int32(42), gotSession)
				assert.Equal(t, "SAS321", gotCallsign)
				return &models.Strip{Session: gotSession, Callsign: gotCallsign}, nil
			},
		},
	}
	hub := &Hub{
		server:       server,
		send:         make(chan internalMessage, 1),
		clxOverrides: make(map[int32]map[string]bool),
	}
	client := &Client{hub: hub, session: 42, position: "EKCH_B_GND"}

	payload, err := json.Marshal(frontendEvents.ClxUpdateTobtAction{Callsign: "SAS321"})
	require.NoError(t, err)

	err = handleClxUpdateTobt(context.Background(), client, Message{Message: payload})
	require.NoError(t, err)
	assert.True(t, cdmService.clxCalled)
	assert.Equal(t, int32(42), cdmService.clxSession)
	assert.Equal(t, "SAS321", cdmService.clxCallsign)
	assert.Len(t, cdmService.clxTobt, 4)

	select {
	case sent := <-hub.send:
		_, ok := sent.message.(frontendEvents.StripUpdateEvent)
		assert.True(t, ok)
	default:
		t.Fatal("expected strip update to be queued")
	}
}
