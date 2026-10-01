package services

import (
	"context"
)

// ReleaseDepartureStand frees SAT capacity once PUSH is operationally active.
// The strip keeps its origin stand because downstream routing still needs it.
func (s *DepartureLifecycleService) ReleaseDepartureStand(ctx context.Context, session int32, callsign string) error {
	s.clearUnassignedStandWarning(session, callsign)
	assignment, err := s.assignments.GetAssignment(ctx, session, callsign)
	if err != nil {
		if isNotFound(err) {
			return nil
		}
		return err
	}
	return s.allocations.ReleaseAssignmentRetainingStand(ctx, assignment)
}
