package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRemovingSessionCancelsOnlyItsPDCWorkAndKeepsSharedClient(t *testing.T) {
	deps := validPdcServiceDependencies()
	client := &countedPDCClient{}
	deps.Client = client
	deps.Sessions = &testutil.MockSessionRepository{GetByNamesFn: func(context.Context, string) ([]*models.Session, error) {
		return []*models.Session{{ID: 1, Airport: "AAAA", Name: "LIVE"}, {ID: 2, Airport: "BBBB", Name: "LIVE"}}, nil
	}}
	service, err := NewPDCService(deps)
	require.NoError(t, err)
	a, cancelA := context.WithCancel(t.Context())
	b, cancelB := context.WithCancel(t.Context())
	defer cancelB()
	service.timeouts["SAS101_1"] = &timeoutTracker{sessionID: 1, cancel: cancelA}
	service.timeouts["SAS101_2"] = &timeoutTracker{sessionID: 2, cancel: cancelB}
	service.RemoveSession(1)
	service.RemoveSession(1)
	require.ErrorIs(t, a.Err(), context.Canceled)
	require.NoError(t, b.Err())
	require.Len(t, service.timeouts, 1)
	require.Same(t, deps.Client, service.client)
	service.startClearanceTimeoutAfter(t.Context(), 1, sessionInformation{id: 1}, "SAS101", "123", false, time.Hour)
	require.Len(t, service.timeouts, 1, "removed sessions cannot restart timeout work")
	require.Error(t, service.requireCapability(t.Context(), 1))
	require.NoError(t, service.pollAndProcess(t.Context()))
	require.Equal(t, []string{"BBBB"}, client.polls, "the surviving airport keeps using the same shared client")
}
