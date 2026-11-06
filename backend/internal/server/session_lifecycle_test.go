package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type lifecycleDeleteRepository struct {
	repository.SessionRepository
	deleteFn func(context.Context, int32) (int64, error)
}

func (r *lifecycleDeleteRepository) Delete(ctx context.Context, id int32) (int64, error) {
	return r.deleteFn(ctx, id)
}

func TestSessionExpiryEvictsScopedWorkOnlyAfterSuccessfulDelete(t *testing.T) {
	for _, outcome := range []string{"success", "deregister failed", "delete failed", "already removed"} {
		t.Run(outcome, func(t *testing.T) {
			cdm := &expiredSessionCdmService{}
			if outcome == "deregister failed" {
				cdm.err = errors.New("offline provider")
			}
			deleted := false
			repo := &lifecycleDeleteRepository{deleteFn: func(context.Context, int32) (int64, error) {
				deleted = true
				if outcome == "delete failed" {
					return 0, errors.New("isolated database failure")
				}
				if outcome == "already removed" {
					return 0, nil
				}
				return 1, nil
			}}
			var evicted []int32
			server := &Server{sessionRepo: repo, cdmService: cdm, sessionCleanup: []func(int32){func(id int32) { evicted = append(evicted, id) }}}
			server.routeInputs.Store(int32(1), "expired")
			server.routeInputs.Store(int32(2), "another airport")
			server.routeInputs.Store(int32(3), "another environment")
			server.removeExpiredSession(t.Context(), &models.Session{ID: 1, Name: "LIVE", Airport: "AAAA"})
			require.Equal(t, outcome != "deregister failed", deleted)
			if outcome == "success" {
				require.Equal(t, []int32{1}, evicted)
				require.EqualValues(t, 1, cdm.removed)
				_, present := server.routeInputs.Load(int32(1))
				require.False(t, present)
			} else {
				require.Empty(t, evicted)
			}
			for _, id := range []int32{2, 3} {
				_, present := server.routeInputs.Load(id)
				require.True(t, present)
			}
		})
	}
}
