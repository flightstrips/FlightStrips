package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAssumeStripCoordination_CompletedAssumptionDoesNotTouchNewCoordination(t *testing.T) {
	position := "118.580"
	strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Owner: &position, Version: 2}
	// Any access to coordination or owner persistence would panic: a duplicate
	// must leave even a newer outgoing transfer/tag request completely untouched.
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
	}
	hub := &testutil.MockFrontendHub{}
	svc := newWorkflowTestService(repo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(&testutil.MockCoordinationRepository{})
	for range 8 {
		require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "DLH9TK", position))
	}
	require.Empty(t, hub.CoordinationAssumes, "do not publish an assume that could clear a newer transfer")
	require.Equal(t, int32(2), strip.Version)
}
