package frontend

import (
	"context"
	"encoding/json"
	"testing"

	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyCdmService struct {
	session  int32
	callsign string
	called   bool
}

func (s *spyCdmService) HandleReadyRequest(_ context.Context, session int32, callsign string) error {
	s.called = true
	s.session = session
	s.callsign = callsign
	return nil
}

func (s *spyCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("RequestBetterTobt should not be called directly from handleCdmReady")
}

func TestHandleCdmReady_UsesOrchestrationMethod(t *testing.T) {
	cdmService := &spyCdmService{}
	server := &testutil.MockServer{
		CdmServiceVal:  cdmService,
		FrontendHubVal: &testutil.MockFrontendHub{},
	}
	hub := &Hub{server: server}
	client := &Client{hub: hub, session: 42}

	payload, err := json.Marshal(frontendEvents.CdmReadyEvent{Callsign: "SAS321"})
	require.NoError(t, err)

	err = handleCdmReady(context.Background(), client, Message{Message: payload})
	require.NoError(t, err)
	assert.True(t, cdmService.called)
	assert.Equal(t, int32(42), cdmService.session)
	assert.Equal(t, "SAS321", cdmService.callsign)
}
