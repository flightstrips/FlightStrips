package frontend

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	fe "FlightStrips/pkg/events/frontend"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type revisionedStripService struct {
	noOpStripService
	enabled bool
}

func (s *revisionedStripService) UsesRevisionedStripState(context.Context, int32) bool {
	return s.enabled
}

func TestRADISIncrementalServiceAndSDKEventsPublishCommittedStripRevision(t *testing.T) {
	reads := 0
	repo := &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, callsign string) (*models.Strip, error) {
		reads++
		require.EqualValues(t, 7, session)
		require.Equal(t, "RADIS1", callsign)
		stand := "S3"
		return &models.Strip{ID: 12, Version: 9, Callsign: callsign, Session: session, Origin: "EKBI", Destination: "EKYT", Stand: &stand, BoardState: &models.StripBoardState{}}, nil
	}}
	service := &revisionedStripService{enabled: true}
	hub := buildFrontendTestHub(mockServerWithStripRepo(repo), service)
	hub.send = make(chan internalMessage, 8)
	for _, publish := range []func(){func() { hub.SendStandEvent(7, "RADIS1", "S3") }, func() { hub.SendAssignedSquawkEvent(7, "RADIS1", "2401") }, func() { hub.SendCdmUpdates(7, []fe.CdmDataEvent{{Callsign: "RADIS1"}}) }} {
		publish()
		<-hub.send
		message := <-hub.send
		event, ok := message.message.(fe.StripUpdateEvent)
		require.True(t, ok)
		require.EqualValues(t, 9, event.Strip.Version)
		require.EqualValues(t, 12, event.Strip.ID)
		require.Equal(t, "S3", event.Strip.Stand)
	}
	require.Equal(t, 3, reads)
	service.enabled = false
	hub.SendStandEvent(7, "RADIS1", "S3")
	<-hub.send
	require.Empty(t, hub.send)
	require.Equal(t, 3, reads, "legacy notifications do not add repository reads")
}
