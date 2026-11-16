package services

import (
	"context"
	"errors"
	"strings"

	"FlightStrips/internal/models"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"

	"github.com/jackc/pgx/v5"
)

type departureStandReleaser interface {
	ReleaseDepartureStand(ctx context.Context, session int32, callsign string) error
}

type departureStandReleaseResetter interface {
	ResetDepartureStandRelease(ctx context.Context, session int32, callsign string) error
}

var ErrDepartureStandReleased = errors.New("departure stand has been released after push clearance")

func (s *DepartureLifecycleService) ResetDepartureStandRelease(ctx context.Context, session int32, callsign string) error {
	_, err := s.allocations.pool.Exec(ctx, `DELETE FROM departure_stand_releases
		USING strips WHERE departure_stand_releases.strip_id = strips.id AND strips.session = $1 AND strips.callsign = $2`, session, callsign)
	return err
}

func departureStandReleasedByMovement(strip *models.Strip) bool {
	if strip == nil {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(strip.Bay)) {
	case shared.BAY_PUSH, shared.BAY_TAXI, shared.BAY_TAXI_LWR, shared.BAY_TAXI_TWR, shared.BAY_DEPART, shared.BAY_AIRBORNE, shared.BAY_HIDDEN_DEP:
		return true
	}
	if strip.State != nil {
		switch strings.ToUpper(strings.TrimSpace(*strip.State)) {
		case euroscope.GroundStatePush, euroscope.GroundStateTaxi, euroscope.GroundStateLineup, euroscope.GroundStateDepart:
			return true
		}
	}
	return false
}

// ReleaseDepartureStand frees SAT capacity once PUSH is operationally active.
// The strip keeps its origin stand because downstream routing still needs it.
func (s *DepartureLifecycleService) ReleaseDepartureStand(ctx context.Context, session int32, callsign string) error {
	s.clearUnassignedStandWarning(session, callsign)
	// Every later radar tick used to take the session write lock again, even
	// after the release marker existed and the departure assignment was gone.
	// Check both facts in one fresh MVCC snapshot. This read is the no-op's
	// linearization point; a concurrent reset/allocation is a later operation.
	// Never trust a message cache or the marker alone: a new departure
	// assignment still needs the locked transaction below.
	var released bool
	if err := s.allocations.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM strips s
		JOIN departure_stand_releases r ON r.strip_id = s.id
		WHERE s.session = $1 AND s.callsign = $2
		AND NOT EXISTS (SELECT 1 FROM stand_assignments a
		 WHERE a.session_id = s.session AND a.callsign = s.callsign
		 AND upper(a.direction) = 'DEPARTURE'))`, session, callsign).Scan(&released); err != nil {
		return err
	}
	if released {
		return nil
	}
	return retrySerializableOperation(ctx, func() error {
		tx, err := s.allocations.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT id FROM sessions WHERE id = $1 FOR UPDATE", session); err != nil {
			return err
		}
		strip, err := s.allocations.strips.WithTx(tx).LockByCallsign(ctx, session, callsign)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO departure_stand_releases (strip_id) VALUES ($1) ON CONFLICT DO NOTHING", strip.ID); err != nil {
			return err
		}
		store := s.assignments.WithTx(tx)
		assignment, err := store.GetAssignment(ctx, session, callsign)
		if err != nil && !isNotFound(err) {
			return err
		}
		var removed *models.StandAssignment
		if assignment != nil && strings.EqualFold(assignment.Direction, string(sat.AssignmentDirectionDeparture)) {
			deleted, err := store.DeleteAssignment(ctx, session, assignment.ID, assignment.Version)
			if err != nil {
				return err
			}
			if deleted != 1 {
				return errAllocationVersionConflict
			}
			removed = assignment
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if removed != nil {
			s.allocations.publishCommitted(ctx, StandAllocationResult{Assignment: *removed, Removed: true})
		}
		return nil
	})
}

func (s *StripService) releaseDepartureStandOnPush(ctx context.Context, session int32, strip *models.Strip, groundState, airport string) error {
	if strip == nil || !strings.EqualFold(strings.TrimSpace(groundState), shared.BAY_PUSH) ||
		!strings.EqualFold(strings.TrimSpace(strip.Origin), strings.TrimSpace(airport)) {
		return nil
	}
	releaser, ok := s.departureObserver.(departureStandReleaser)
	if !ok {
		return nil
	}
	return releaser.ReleaseDepartureStand(ctx, session, strip.Callsign)
}
