package frontend

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
)

// NoOpStripService implements shared.StripService with all methods as no-ops.
// Embed it in a test-specific struct and override only the methods under test.
type noOpStripService struct {
	PropagateRunwayChangeFn func(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error
}

func (s *noOpStripService) MoveToBay(_ context.Context, _ int32, _ string, _ string, _ bool) error {
	return nil
}
func (s *noOpStripService) MoveFrontendStrip(_ context.Context, _ int32, _ string, _ string, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) MoveStripBetween(_ context.Context, _ int32, _ string, _ *frontend.StripRef, _ string) error {
	return nil
}
func (s *noOpStripService) MoveTacticalStripBetween(_ context.Context, _ int32, _ int64, _ *frontend.StripRef, _ string) error {
	return nil
}
func (s *noOpStripService) CreateCoordinationTransfer(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) CreateEsArrivalCoordination(_ context.Context, _ int32, _ string, _ string, _ string, _ *string) error {
	return nil
}
func (s *noOpStripService) AcceptCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) AssumeStripCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) ForceAssumeStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) RejectCoordination(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) CancelCoordinationTransfer(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) FreeStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) CreateTagRequest(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) AcceptTagRequest(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) AutoTransferAirborneStrip(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) ClearStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UnclearStrip(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) AutoAssumeForClearedStrip(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) AutoAssumeForClearedStripByCid(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) AutoAssumeForControllerOnline(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateAssignedSquawk(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateSquawk(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateRequestedAltitude(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *noOpStripService) UpdateClearedAltitude(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *noOpStripService) UpdateCommunicationType(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateHeading(_ context.Context, _ int32, _ string, _ int32) error {
	return nil
}
func (s *noOpStripService) DeleteStrip(_ context.Context, _ int32, _ string) error { return nil }
func (s *noOpStripService) UpdateGroundState(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateClearedFlag(_ context.Context, _ int32, _ string, _ bool) error {
	return nil
}
func (s *noOpStripService) UpdateStand(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateAircraftPosition(_ context.Context, _ int32, _ string, _, _ float64, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) HandleTrackingControllerChanged(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) HandleCoordinationReceived(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) SyncStrip(_ context.Context, _ int32, _ string, _ interface{}, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateClearedFlagForMove(_ context.Context, _ int32, _ string, _ bool, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) ConfirmPdcClearance(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateGroundStateForMove(_ context.Context, _ int32, _ string, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateReleasePoint(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) ApplyReleasePoint(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) UpdateStartReq(_ context.Context, _ int32, _ string, _ bool) error {
	return nil
}
func (s *noOpStripService) UpdateMarked(_ context.Context, _ int32, _ string, _ bool) error {
	return nil
}
func (s *noOpStripService) RunwayClearance(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) RunwayConfirmation(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error {
	if s.PropagateRunwayChangeFn != nil {
		return s.PropagateRunwayChangeFn(ctx, session, airport, oldRunways, newRunways)
	}
	return nil
}
func (s *noOpStripService) CreateManualFPL(_ context.Context, _ int32, _ frontend.CreateManualFPLAction, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) CreateVFRFPL(_ context.Context, _ int32, _ frontend.CreateVFRFPLAction, _ string) error {
	return nil
}
func (s *noOpStripService) MissedApproach(_ context.Context, _ int32, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) SetValidationStatus(_ context.Context, _ int32, _ string, _ *internalModels.ValidationStatus) error {
	return nil
}
func (s *noOpStripService) AcknowledgeValidationStatus(_ context.Context, _ int32, _ string, _ string, _ string) error {
	return nil
}
func (s *noOpStripService) ClearValidationStatus(_ context.Context, _ int32, _ string) error {
	return nil
}
func (s *noOpStripService) IsValidationBlocking(_ context.Context, _ int32, _ string) (bool, error) {
	return false, nil
}
func (s *noOpStripService) ReevaluatePdcInvalidValidation(_ context.Context, _ int32, _ string, _ bool, _ bool) error {
	return nil
}
func (s *noOpStripService) ReevaluatePdcRequestValidations(_ context.Context, _ int32, _ string, _ bool, _ bool) error {
	return nil
}

func (s *noOpStripService) ClearMandatoryRouteCdm(_ context.Context, _ int32, _ string) {}
