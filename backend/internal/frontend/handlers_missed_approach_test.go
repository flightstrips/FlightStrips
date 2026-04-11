package frontend

import (
	"context"
	"testing"

	"FlightStrips/internal/testutil"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type spyMissedApproachStripService struct {
	testutil.NoOpStripService
	session  int32
	callsign string
	position string
	called   bool
}

func (s *spyMissedApproachStripService) MissedApproach(_ context.Context, session int32, callsign string, position string) error {
	s.called = true
	s.session = session
	s.callsign = callsign
	s.position = position
	return nil
}

func TestHandleMissedApproach_BroadcastsGoAroundEvent(t *testing.T) {
	const session = int32(17)
	const callsign = "SAS123"
	const position = "118.105"

	stripService := &spyMissedApproachStripService{}
	hub := buildFrontendTestHub(&testutil.MockServer{}, stripService)
	hub.send = make(chan internalMessage, 1)

	client := buildFrontendTestClient(hub, session, "EKCH")
	client.position = position

	msg := marshalMessage(t, frontendEvents.MissedApproachRequestEvent{
		Type:     frontendEvents.MissedApproachRequestType,
		Callsign: callsign,
	})

	err := handleMissedApproach(context.Background(), client, msg)
	require.NoError(t, err)

	require.True(t, stripService.called)
	assert.Equal(t, session, stripService.session)
	assert.Equal(t, callsign, stripService.callsign)
	assert.Equal(t, position, stripService.position)

	select {
	case broadcast := <-hub.send:
		assert.Equal(t, session, broadcast.session)
		assert.Nil(t, broadcast.cid)

		event, ok := broadcast.message.(frontendEvents.GoAroundEvent)
		require.True(t, ok)
		assert.Equal(t, callsign, event.Callsign)
	default:
		t.Fatal("expected go around broadcast")
	}
}
