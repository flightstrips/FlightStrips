package services

import (
	"context"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestAssumeStripCoordination_CompletedAssumptionDoesNotTouchNewCoordination(t *testing.T) {
	for _, pending := range []string{"none", "outgoing", "tag request"} {
		t.Run(pending, func(t *testing.T) {
			position := "118.580"
			strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Owner: &position, Version: 2}
			// Any mutation of coordination or ownership would panic.
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
			}
			hub := &testutil.MockFrontendHub{}
			svc := newWorkflowTestService(repo)
			svc.SetFrontendHub(hub)
			svc.SetCoordinationRepo(&testutil.MockCoordinationRepository{
				GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) {
					if pending == "none" {
						return nil, pgx.ErrNoRows
					}
					return &models.Coordination{ID: 1, StripID: strip.ID, FromPosition: position, ToPosition: "121.730", IsTagRequest: pending == "tag request"}, nil
				},
			})
			for range 8 {
				require.NoError(t, svc.AssumeStripCoordination(context.Background(), 1, "DLH9TK", position))
			}
			require.Empty(t, hub.CoordinationAssumes, "do not publish an assume that could clear a newer transfer")
			require.Equal(t, int32(2), strip.Version)
		})
	}
}

func TestAssumeStripCoordination_CurrentOwnerAcceptsIncomingEsCoordination(t *testing.T) {
	position := "118.580"
	strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Owner: &position, Version: 2}
	var pending *models.Coordination
	deleted := false
	coordRepo := &testutil.MockCoordinationRepository{
		CreateFn: func(_ context.Context, coordination *models.Coordination) error {
			pending = coordination
			pending.ID = 7
			return nil
		},
		GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) {
			if pending == nil {
				return nil, pgx.ErrNoRows
			}
			return pending, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			require.Equal(t, int32(7), id)
			pending = nil
			deleted = true
			return nil
		},
	}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn:            func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		SetNextAndPreviousOwnersFn: func(context.Context, int32, string, []string, []string) error { return nil },
		SetOwnerFn: func(_ context.Context, _ int32, _ string, owner *string, _ int32) (int64, error) {
			require.Equal(t, position, *owner)
			return 1, nil
		},
	}
	hub := &testutil.MockFrontendHub{}
	svc := newWorkflowTestService(repo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	// With no ES commander, owned arrival coordination uses this backend fallback.
	require.NoError(t, svc.CreateEsArrivalCoordination(t.Context(), 1, strip.Callsign, "119.805", position, nil))
	require.NoError(t, svc.AssumeStripCoordination(t.Context(), 1, strip.Callsign, position))
	require.True(t, deleted)
	require.Nil(t, pending)
	require.Len(t, hub.CoordinationAssumes, 1)
	require.NoError(t, svc.AssumeStripCoordination(t.Context(), 1, strip.Callsign, position))
	require.Len(t, hub.CoordinationAssumes, 1, "a subsequent duplicate remains a no-op")
}
