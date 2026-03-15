package testutil

import (
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
)

// NoOpStripService implements shared.StripService with all methods as no-ops.
// Embed it in a test-specific struct and override only the methods under test.
type NoOpStripService struct {
	PropagateRunwayChangeFn func(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error
}

func (s *NoOpStripService) MoveToBay(_ context.Context, _ int32, _ string, _ string, _ bool) error {
	return nil
}
func (s *NoOpStripService) MoveStripBetween(_ context.Context, _ int32, _ string, _ *frontend.StripRef, _ string) error {
	return nil
}
func (s *NoOpStripService) MoveTacticalStripBetween(_ context.Context, _ int32, _ int64, _ *frontend.StripRef, _ string) error {
	return nil
}
func (s *NoOpStripService) CreateCoordinationTransfer(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) CreateEsArrivalCoordination(_ context.Context, _ int32, _ string, _ string, _ string, _ *string) error {
	return nil
}
func (s *NoOpStripService) AcceptCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) AssumeStripCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) ForceAssumeStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) RejectCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) CancelCoordinationTransfer(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) FreeStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) AutoTransferAirborneStrip(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *NoOpStripService) ClearStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UnclearStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) AutoAssumeForClearedStrip(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *NoOpStripService) AutoAssumeForControllerOnline(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateAssignedSquawk(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateSquawk(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateRequestedAltitude(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *NoOpStripService) UpdateClearedAltitude(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *NoOpStripService) UpdateCommunicationType(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateHeading(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *NoOpStripService) DeleteStrip(_ context.Context, _ int32, _ string) error { return nil }
func (s *NoOpStripService) UpdateGroundState(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateClearedFlag(_ context.Context, _ int32, _ string, _ bool) error {
	return nil
}
func (s *NoOpStripService) UpdateStand(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateAircraftPosition(_ context.Context, _ int32, _ string, _, _ float64, _ int32, _ string) error {
	return nil
}
func (s *NoOpStripService) HandleTrackingControllerChanged(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) HandleCoordinationReceived(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) SyncStrip(_ context.Context, _ int32, _ interface{}, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateClearedFlagForMove(_ context.Context, _ int32, _ string, _ bool, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateGroundStateForMove(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateReleasePoint(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) ApplyReleasePoint(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *NoOpStripService) UpdateMarked(_ context.Context, _ int32, _ string, _ bool) error {
	return nil
}
func (s *NoOpStripService) RunwayClearance(_ context.Context, _ int32, _ string) error { return nil }
func (s *NoOpStripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	if s.PropagateRunwayChangeFn != nil {
		return s.PropagateRunwayChangeFn(ctx, session, airport, oldRunways, newRunways)
	}
	return nil
}
