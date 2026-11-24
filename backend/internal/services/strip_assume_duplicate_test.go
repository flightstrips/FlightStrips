package services

import (
	"context"
	"errors"
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

func TestAssumeStripCoordination_PublishesCommitWhenValidationReevaluationFails(t *testing.T) {
	const position = "118.580"
	strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Version: 2, NextOwners: []string{position, "121.730"}}
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil },
		CommitStripAssumptionFn: func(_ context.Context, _ int32, _ *models.Strip, _ *models.Coordination, owner string, nextOwners, _ []string) (bool, error) {
			require.Equal(t, position, owner)
			require.Equal(t, []string{"121.730"}, nextOwners)
			return true, nil
		},
	}
	hub := &testutil.MockFrontendHub{}
	service := newWorkflowTestService(repo)
	service.SetFrontendHub(hub)
	service.SetCoordinationRepo(&testutil.MockCoordinationRepository{
		GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) { return nil, pgx.ErrNoRows },
	})
	reevaluationFailed := false
	service.SetSessionRepo(&testutil.MockSessionRepository{
		GetByIDFn: func(context.Context, int32) (*models.Session, error) {
			reevaluationFailed = true
			return nil, errors.New("session read failed")
		},
	})

	require.NoError(t, service.AssumeStripCoordination(t.Context(), 1, strip.Callsign, position))
	require.True(t, reevaluationFailed)
	require.Equal(t, []testutil.CoordinationAssumeCall{{Session: 1, Callsign: strip.Callsign, Position: position}}, hub.CoordinationAssumes)
	require.Len(t, hub.OwnersUpdates, 1)
	require.Equal(t, position, hub.OwnersUpdates[0].Owner)
	require.Equal(t, []string{"121.730"}, hub.OwnersUpdates[0].NextOwners)
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

func TestAssumeStripCoordination_AcceptsValidatedIncomingInsteadOfNewOutgoing(t *testing.T) {
	position := "118.580"
	strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Owner: &position, Version: 2}
	incoming := &models.Coordination{ID: 7, Session: 1, StripID: strip.ID, FromPosition: "119.805", ToPosition: position}
	outgoing := &models.Coordination{ID: 8, Session: 1, StripID: strip.ID, FromPosition: position, ToPosition: "121.730"}
	pending := map[int32]*models.Coordination{incoming.ID: incoming}
	stripReads, coordinationReads := 0, 0
	repo := &testutil.MockStripRepository{
		GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
			stripReads++
			if stripReads == 2 {
				// A new transfer is inserted after the assume guard selected the
				// incoming coordination, but before acceptance refreshes the strip.
				pending[outgoing.ID] = outgoing
			}
			return strip, nil
		},
		SetNextAndPreviousOwnersFn: func(_ context.Context, _ int32, _ string, _ []string, previous []string) error {
			require.Equal(t, []string{incoming.FromPosition}, previous)
			return nil
		},
		SetOwnerFn: func(context.Context, int32, string, *string, int32) (int64, error) { return 1, nil },
	}
	coordRepo := &testutil.MockCoordinationRepository{
		GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) {
			coordinationReads++
			if coordinationReads == 1 {
				return incoming, nil
			}
			// Without an ORDER BY, a second lookup may return the new row.
			return outgoing, nil
		},
		DeleteFn: func(_ context.Context, id int32) error {
			require.Equal(t, incoming.ID, id, "acceptance must delete only the validated coordination")
			delete(pending, id)
			return nil
		},
	}
	hub := &testutil.MockFrontendHub{}
	svc := newWorkflowTestService(repo)
	svc.SetFrontendHub(hub)
	svc.SetCoordinationRepo(coordRepo)
	require.NoError(t, svc.AssumeStripCoordination(t.Context(), 1, strip.Callsign, position))
	require.Equal(t, 1, coordinationReads)
	require.Equal(t, map[int32]*models.Coordination{outgoing.ID: outgoing}, pending)
	require.Len(t, hub.CoordinationAssumes, 1)
}

func TestAssumeStripCoordination_DoesNotReuseConsumedOrCancelledIncoming(t *testing.T) {
	for _, transition := range []string{"consumed", "cancelled", "consumed and transferred again"} {
		t.Run(transition, func(t *testing.T) {
			position := "118.580"
			strip := &models.Strip{ID: 70, Callsign: "DLH9TK", Version: 2}
			incoming := &models.Coordination{ID: 7, Session: 1, StripID: strip.ID, FromPosition: "119.805", ToPosition: position}
			reads := 0
			repo := &testutil.MockStripRepository{
				GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
					reads++
					if reads == 2 && transition != "cancelled" {
						owner := position
						if transition == "consumed and transferred again" {
							owner = "121.730"
						}
						strip.Owner = &owner
						strip.Version++
					}
					return strip, nil
				},
			}
			coord := &testutil.MockCoordinationRepository{
				GetByStripIDFn: func(context.Context, int32, int32) (*models.Coordination, error) { return incoming, nil },
				// The selected row was consumed/cancelled after validation. No
				// owner/history write is allowed even with a fresh strip version.
				DeleteFn: func(context.Context, int32) error { return pgx.ErrNoRows },
			}
			hub := &testutil.MockFrontendHub{}
			svc := newWorkflowTestService(repo)
			svc.SetFrontendHub(hub)
			svc.SetCoordinationRepo(coord)
			require.ErrorContains(t, svc.AssumeStripCoordination(t.Context(), 1, strip.Callsign, position), "changed before acceptance")
			require.Empty(t, hub.CoordinationAssumes)
			require.Empty(t, hub.OwnersUpdates)
		})
	}
}
