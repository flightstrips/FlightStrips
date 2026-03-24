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
	session     int32
	observation euroscopeEvents.CdmLocalDataEvent
	called      bool
}

func (s *spyLocalCdmService) HandleReadyRequest(_ context.Context, _ int32, _ string) error {
	panic("HandleReadyRequest should not be called from handleCdmLocalData")
}

func (s *spyLocalCdmService) HandleLocalObservation(_ context.Context, session int32, observation euroscopeEvents.CdmLocalDataEvent) error {
	s.called = true
	s.session = session
	s.observation = observation
	return nil
}

func (s *spyLocalCdmService) RequestBetterTobt(_ context.Context, _ int32, _ string) error {
	panic("RequestBetterTobt should not be called from handleCdmLocalData")
}

func TestHandleCdmLocalData_DefaultsSourceMetadataFromClient(t *testing.T) {
	cdmService := &spyLocalCdmService{}
	server := &testutil.MockServer{
		CdmServiceVal: cdmService,
	}
	hub := &Hub{
		server: server,
		master: map[int32]*Client{},
	}
	client := &Client{
		hub:      hub,
		session:  42,
		callsign: "EKCH_B_GND",
		user:     shared.NewAuthenticatedUser("1234567", 0, nil),
	}
	hub.master[42] = client

	payload, err := json.Marshal(euroscopeEvents.CdmLocalDataEvent{
		Callsign: "SAS321",
		Tobt:     "1210",
	})
	require.NoError(t, err)

	err = handleCdmLocalData(context.Background(), client, Message{
		Type:    euroscopeEvents.CdmLocalData,
		Message: payload,
	})
	require.NoError(t, err)
	assert.True(t, cdmService.called)
	assert.Equal(t, int32(42), cdmService.session)
	assert.Equal(t, "SAS321", cdmService.observation.Callsign)
	assert.Equal(t, "1210", cdmService.observation.Tobt)
	assert.Equal(t, "EKCH_B_GND", cdmService.observation.SourcePosition)
	assert.Equal(t, "master", cdmService.observation.SourceRole)
}
