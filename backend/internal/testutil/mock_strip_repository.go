package testutil

import (
	"FlightStrips/internal/models"
	"context"
	"time"
)

// MockStripRepository is a configurable mock for repository.StripRepository.
// Set function fields before each test; unset fields panic on call.
type MockStripRepository struct {
	CreateFn                    func(ctx context.Context, strip *models.Strip) error
	GetByCallsignFn             func(ctx context.Context, session int32, callsign string) (*models.Strip, error)
	ListFn                      func(ctx context.Context, session int32) ([]*models.Strip, error)
	UpdateFn                    func(ctx context.Context, strip *models.Strip) (int64, error)
	DeleteFn                    func(ctx context.Context, session int32, callsign string) error
	ListByOriginFn              func(ctx context.Context, session int32, origin string) ([]*models.Strip, error)
	GetBayFn                    func(ctx context.Context, session int32, callsign string) (string, error)
	UpdateSequenceFn            func(ctx context.Context, session int32, callsign string, sequence int32) (int64, error)
	UpdateBayAndSequenceFn      func(ctx context.Context, session int32, callsign string, bay string, sequence int32) (int64, error)
	UpdateSequenceBulkFn        func(ctx context.Context, session int32, callsigns []string, sequences []int32) error
	RecalculateSequencesFn      func(ctx context.Context, session int32, bay string, spacing int32) error
	ListSequencesFn             func(ctx context.Context, session int32, bay string) ([]*models.StripSequence, error)
	GetSequenceFn               func(ctx context.Context, session int32, callsign string, bay string) (int32, error)
	GetMaxSequenceInBayFn       func(ctx context.Context, session int32, bay string) (int32, error)
	GetMinSequenceInBayFn       func(ctx context.Context, session int32, bay string) (int32, error)
	GetNextSequenceFn           func(ctx context.Context, session int32, bay string, sequence int32) (int32, error)
	GetPrevSequenceFn           func(ctx context.Context, session int32, bay string, sequence int32, excludeCallsign string) (int32, error)
	UpdateSquawkFn              func(ctx context.Context, session int32, callsign string, squawk *string, version *int32) (int64, error)
	UpdateAssignedSquawkFn      func(ctx context.Context, session int32, callsign string, assignedSquawk *string, version *int32) (int64, error)
	UpdateClearedAltitudeFn     func(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error)
	UpdateRequestedAltitudeFn   func(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error)
	UpdateCommunicationTypeFn   func(ctx context.Context, session int32, callsign string, commType *string, version *int32) (int64, error)
	UpdateGroundStateFn         func(ctx context.Context, session int32, callsign string, state *string, bay string, version *int32) (int64, error)
	UpdateClearedFlagFn         func(ctx context.Context, session int32, callsign string, cleared bool, bay string, version *int32) (int64, error)
	UpdateAircraftPositionFn    func(ctx context.Context, session int32, callsign string, lat *float64, lon *float64, alt *int32, bay string, version *int32) (int64, error)
	UpdateBayFn                 func(ctx context.Context, session int32, callsign string, bay string, version *int32) (int64, error)
	UpdateHeadingFn             func(ctx context.Context, session int32, callsign string, heading *int32, version *int32) (int64, error)
	UpdateStandFn               func(ctx context.Context, session int32, callsign string, stand *string, version *int32) (int64, error)
	UpdateRunwayFn              func(ctx context.Context, session int32, callsign string, runway *string, version *int32) (int64, error)
	UpdateMarkedFn              func(ctx context.Context, session int32, callsign string, marked bool, version *int32) (int64, error)
	UpdateRunwayClearanceFn     func(ctx context.Context, session int32, callsign string) (int64, error)
	UpdateRegistrationFn        func(ctx context.Context, session int32, callsign string, registration string) error
	UpdateTrackingControllerFn  func(ctx context.Context, session int32, callsign string, trackingController string) (int64, error)
	SetOwnerFn                  func(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error)
	SetNextOwnersFn             func(ctx context.Context, session int32, callsign string, nextOwners []string) error
	SetPreviousOwnersFn         func(ctx context.Context, session int32, callsign string, previousOwners []string) error
	SetNextAndPreviousOwnersFn  func(ctx context.Context, session int32, callsign string, nextOwners []string, previousOwners []string) error
	GetCdmDataFn                func(ctx context.Context, session int32) ([]*models.CdmData, error)
	GetCdmDataForCallsignFn     func(ctx context.Context, session int32, callsign string) (*models.CdmData, error)
	UpdateCdmDataFn             func(ctx context.Context, session int32, callsign string, tobt *string, tsat *string, ttot *string, ctot *string, aobt *string, eobt *string, cdmStatus *string) (int64, error)
	SetCdmStatusFn              func(ctx context.Context, session int32, callsign string, cdmStatus *string) (int64, error)
	UpdateReleasePointFn        func(ctx context.Context, session int32, callsign string, releasePoint *string) (int64, error)
	SetPdcRequestedFn           func(ctx context.Context, session int32, callsign string, pdcState string, pdcRequestedAt *time.Time) error
	SetPdcMessageSentFn         func(ctx context.Context, session int32, callsign string, pdcState string, pdcMessageSequence *int32, pdcMessageSent *time.Time) error
	UpdatePdcStatusFn           func(ctx context.Context, session int32, callsign string, pdcState string) error
}

func (m *MockStripRepository) Create(ctx context.Context, strip *models.Strip) error {
	if m.CreateFn == nil {
		panic("unexpected call to MockStripRepository.Create")
	}
	return m.CreateFn(ctx, strip)
}

func (m *MockStripRepository) GetByCallsign(ctx context.Context, session int32, callsign string) (*models.Strip, error) {
	if m.GetByCallsignFn == nil {
		panic("unexpected call to MockStripRepository.GetByCallsign")
	}
	return m.GetByCallsignFn(ctx, session, callsign)
}

func (m *MockStripRepository) List(ctx context.Context, session int32) ([]*models.Strip, error) {
	if m.ListFn == nil {
		panic("unexpected call to MockStripRepository.List")
	}
	return m.ListFn(ctx, session)
}

func (m *MockStripRepository) Update(ctx context.Context, strip *models.Strip) (int64, error) {
	if m.UpdateFn == nil {
		panic("unexpected call to MockStripRepository.Update")
	}
	return m.UpdateFn(ctx, strip)
}

func (m *MockStripRepository) Delete(ctx context.Context, session int32, callsign string) error {
	if m.DeleteFn == nil {
		panic("unexpected call to MockStripRepository.Delete")
	}
	return m.DeleteFn(ctx, session, callsign)
}

func (m *MockStripRepository) ListByOrigin(ctx context.Context, session int32, origin string) ([]*models.Strip, error) {
	if m.ListByOriginFn == nil {
		panic("unexpected call to MockStripRepository.ListByOrigin")
	}
	return m.ListByOriginFn(ctx, session, origin)
}

func (m *MockStripRepository) GetBay(ctx context.Context, session int32, callsign string) (string, error) {
	if m.GetBayFn == nil {
		panic("unexpected call to MockStripRepository.GetBay")
	}
	return m.GetBayFn(ctx, session, callsign)
}

func (m *MockStripRepository) UpdateSequence(ctx context.Context, session int32, callsign string, sequence int32) (int64, error) {
	if m.UpdateSequenceFn == nil {
		panic("unexpected call to MockStripRepository.UpdateSequence")
	}
	return m.UpdateSequenceFn(ctx, session, callsign, sequence)
}

func (m *MockStripRepository) UpdateBayAndSequence(ctx context.Context, session int32, callsign string, bay string, sequence int32) (int64, error) {
	if m.UpdateBayAndSequenceFn == nil {
		panic("unexpected call to MockStripRepository.UpdateBayAndSequence")
	}
	return m.UpdateBayAndSequenceFn(ctx, session, callsign, bay, sequence)
}

func (m *MockStripRepository) UpdateSequenceBulk(ctx context.Context, session int32, callsigns []string, sequences []int32) error {
	if m.UpdateSequenceBulkFn == nil {
		panic("unexpected call to MockStripRepository.UpdateSequenceBulk")
	}
	return m.UpdateSequenceBulkFn(ctx, session, callsigns, sequences)
}

func (m *MockStripRepository) RecalculateSequences(ctx context.Context, session int32, bay string, spacing int32) error {
	if m.RecalculateSequencesFn == nil {
		panic("unexpected call to MockStripRepository.RecalculateSequences")
	}
	return m.RecalculateSequencesFn(ctx, session, bay, spacing)
}

func (m *MockStripRepository) ListSequences(ctx context.Context, session int32, bay string) ([]*models.StripSequence, error) {
	if m.ListSequencesFn == nil {
		panic("unexpected call to MockStripRepository.ListSequences")
	}
	return m.ListSequencesFn(ctx, session, bay)
}

func (m *MockStripRepository) GetSequence(ctx context.Context, session int32, callsign string, bay string) (int32, error) {
	if m.GetSequenceFn == nil {
		panic("unexpected call to MockStripRepository.GetSequence")
	}
	return m.GetSequenceFn(ctx, session, callsign, bay)
}

func (m *MockStripRepository) GetMaxSequenceInBay(ctx context.Context, session int32, bay string) (int32, error) {
	if m.GetMaxSequenceInBayFn == nil {
		panic("unexpected call to MockStripRepository.GetMaxSequenceInBay")
	}
	return m.GetMaxSequenceInBayFn(ctx, session, bay)
}

func (m *MockStripRepository) GetMinSequenceInBay(ctx context.Context, session int32, bay string) (int32, error) {
	if m.GetMinSequenceInBayFn == nil {
		panic("unexpected call to MockStripRepository.GetMinSequenceInBay")
	}
	return m.GetMinSequenceInBayFn(ctx, session, bay)
}

func (m *MockStripRepository) GetNextSequence(ctx context.Context, session int32, bay string, sequence int32) (int32, error) {
	if m.GetNextSequenceFn == nil {
		panic("unexpected call to MockStripRepository.GetNextSequence")
	}
	return m.GetNextSequenceFn(ctx, session, bay, sequence)
}

func (m *MockStripRepository) GetPrevSequence(ctx context.Context, session int32, bay string, sequence int32, excludeCallsign string) (int32, error) {
	if m.GetPrevSequenceFn == nil {
		panic("unexpected call to MockStripRepository.GetPrevSequence")
	}
	return m.GetPrevSequenceFn(ctx, session, bay, sequence, excludeCallsign)
}

func (m *MockStripRepository) UpdateSquawk(ctx context.Context, session int32, callsign string, squawk *string, version *int32) (int64, error) {
	if m.UpdateSquawkFn == nil {
		panic("unexpected call to MockStripRepository.UpdateSquawk")
	}
	return m.UpdateSquawkFn(ctx, session, callsign, squawk, version)
}

func (m *MockStripRepository) UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, assignedSquawk *string, version *int32) (int64, error) {
	if m.UpdateAssignedSquawkFn == nil {
		panic("unexpected call to MockStripRepository.UpdateAssignedSquawk")
	}
	return m.UpdateAssignedSquawkFn(ctx, session, callsign, assignedSquawk, version)
}

func (m *MockStripRepository) UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error) {
	if m.UpdateClearedAltitudeFn == nil {
		panic("unexpected call to MockStripRepository.UpdateClearedAltitude")
	}
	return m.UpdateClearedAltitudeFn(ctx, session, callsign, altitude, version)
}

func (m *MockStripRepository) UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude *int32, version *int32) (int64, error) {
	if m.UpdateRequestedAltitudeFn == nil {
		panic("unexpected call to MockStripRepository.UpdateRequestedAltitude")
	}
	return m.UpdateRequestedAltitudeFn(ctx, session, callsign, altitude, version)
}

func (m *MockStripRepository) UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType *string, version *int32) (int64, error) {
	if m.UpdateCommunicationTypeFn == nil {
		panic("unexpected call to MockStripRepository.UpdateCommunicationType")
	}
	return m.UpdateCommunicationTypeFn(ctx, session, callsign, commType, version)
}

func (m *MockStripRepository) UpdateGroundState(ctx context.Context, session int32, callsign string, state *string, bay string, version *int32) (int64, error) {
	if m.UpdateGroundStateFn == nil {
		panic("unexpected call to MockStripRepository.UpdateGroundState")
	}
	return m.UpdateGroundStateFn(ctx, session, callsign, state, bay, version)
}

func (m *MockStripRepository) UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool, bay string, version *int32) (int64, error) {
	if m.UpdateClearedFlagFn == nil {
		panic("unexpected call to MockStripRepository.UpdateClearedFlag")
	}
	return m.UpdateClearedFlagFn(ctx, session, callsign, cleared, bay, version)
}

func (m *MockStripRepository) UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat *float64, lon *float64, alt *int32, bay string, version *int32) (int64, error) {
	if m.UpdateAircraftPositionFn == nil {
		panic("unexpected call to MockStripRepository.UpdateAircraftPosition")
	}
	return m.UpdateAircraftPositionFn(ctx, session, callsign, lat, lon, alt, bay, version)
}

func (m *MockStripRepository) UpdateBay(ctx context.Context, session int32, callsign string, bay string, version *int32) (int64, error) {
	if m.UpdateBayFn == nil {
		panic("unexpected call to MockStripRepository.UpdateBay")
	}
	return m.UpdateBayFn(ctx, session, callsign, bay, version)
}

func (m *MockStripRepository) UpdateHeading(ctx context.Context, session int32, callsign string, heading *int32, version *int32) (int64, error) {
	if m.UpdateHeadingFn == nil {
		panic("unexpected call to MockStripRepository.UpdateHeading")
	}
	return m.UpdateHeadingFn(ctx, session, callsign, heading, version)
}

func (m *MockStripRepository) UpdateStand(ctx context.Context, session int32, callsign string, stand *string, version *int32) (int64, error) {
	if m.UpdateStandFn == nil {
		panic("unexpected call to MockStripRepository.UpdateStand")
	}
	return m.UpdateStandFn(ctx, session, callsign, stand, version)
}

func (m *MockStripRepository) UpdateRunway(ctx context.Context, session int32, callsign string, runway *string, version *int32) (int64, error) {
	if m.UpdateRunwayFn == nil {
		panic("unexpected call to MockStripRepository.UpdateRunway")
	}
	return m.UpdateRunwayFn(ctx, session, callsign, runway, version)
}

func (m *MockStripRepository) UpdateMarked(ctx context.Context, session int32, callsign string, marked bool, version *int32) (int64, error) {
	if m.UpdateMarkedFn == nil {
		panic("unexpected call to MockStripRepository.UpdateMarked")
	}
	return m.UpdateMarkedFn(ctx, session, callsign, marked, version)
}

func (m *MockStripRepository) UpdateRunwayClearance(ctx context.Context, session int32, callsign string) (int64, error) {
	if m.UpdateRunwayClearanceFn == nil {
		panic("unexpected call to MockStripRepository.UpdateRunwayClearance")
	}
	return m.UpdateRunwayClearanceFn(ctx, session, callsign)
}

func (m *MockStripRepository) UpdateRegistration(ctx context.Context, session int32, callsign string, registration string) error {
	if m.UpdateRegistrationFn == nil {
		panic("unexpected call to MockStripRepository.UpdateRegistration")
	}
	return m.UpdateRegistrationFn(ctx, session, callsign, registration)
}

func (m *MockStripRepository) UpdateTrackingController(ctx context.Context, session int32, callsign string, trackingController string) (int64, error) {
	if m.UpdateTrackingControllerFn == nil {
		panic("unexpected call to MockStripRepository.UpdateTrackingController")
	}
	return m.UpdateTrackingControllerFn(ctx, session, callsign, trackingController)
}

func (m *MockStripRepository) SetOwner(ctx context.Context, session int32, callsign string, owner *string, version int32) (int64, error) {
	if m.SetOwnerFn == nil {
		panic("unexpected call to MockStripRepository.SetOwner")
	}
	return m.SetOwnerFn(ctx, session, callsign, owner, version)
}

func (m *MockStripRepository) SetNextOwners(ctx context.Context, session int32, callsign string, nextOwners []string) error {
	if m.SetNextOwnersFn == nil {
		panic("unexpected call to MockStripRepository.SetNextOwners")
	}
	return m.SetNextOwnersFn(ctx, session, callsign, nextOwners)
}

func (m *MockStripRepository) SetPreviousOwners(ctx context.Context, session int32, callsign string, previousOwners []string) error {
	if m.SetPreviousOwnersFn == nil {
		panic("unexpected call to MockStripRepository.SetPreviousOwners")
	}
	return m.SetPreviousOwnersFn(ctx, session, callsign, previousOwners)
}

func (m *MockStripRepository) SetNextAndPreviousOwners(ctx context.Context, session int32, callsign string, nextOwners []string, previousOwners []string) error {
	if m.SetNextAndPreviousOwnersFn == nil {
		panic("unexpected call to MockStripRepository.SetNextAndPreviousOwners")
	}
	return m.SetNextAndPreviousOwnersFn(ctx, session, callsign, nextOwners, previousOwners)
}

func (m *MockStripRepository) GetCdmData(ctx context.Context, session int32) ([]*models.CdmData, error) {
	if m.GetCdmDataFn == nil {
		panic("unexpected call to MockStripRepository.GetCdmData")
	}
	return m.GetCdmDataFn(ctx, session)
}

func (m *MockStripRepository) GetCdmDataForCallsign(ctx context.Context, session int32, callsign string) (*models.CdmData, error) {
	if m.GetCdmDataForCallsignFn == nil {
		panic("unexpected call to MockStripRepository.GetCdmDataForCallsign")
	}
	return m.GetCdmDataForCallsignFn(ctx, session, callsign)
}

func (m *MockStripRepository) UpdateCdmData(ctx context.Context, session int32, callsign string, tobt *string, tsat *string, ttot *string, ctot *string, aobt *string, eobt *string, cdmStatus *string) (int64, error) {
	if m.UpdateCdmDataFn == nil {
		panic("unexpected call to MockStripRepository.UpdateCdmData")
	}
	return m.UpdateCdmDataFn(ctx, session, callsign, tobt, tsat, ttot, ctot, aobt, eobt, cdmStatus)
}

func (m *MockStripRepository) SetCdmStatus(ctx context.Context, session int32, callsign string, cdmStatus *string) (int64, error) {
	if m.SetCdmStatusFn == nil {
		panic("unexpected call to MockStripRepository.SetCdmStatus")
	}
	return m.SetCdmStatusFn(ctx, session, callsign, cdmStatus)
}

func (m *MockStripRepository) UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint *string) (int64, error) {
	if m.UpdateReleasePointFn == nil {
		panic("unexpected call to MockStripRepository.UpdateReleasePoint")
	}
	return m.UpdateReleasePointFn(ctx, session, callsign, releasePoint)
}

func (m *MockStripRepository) SetPdcRequested(ctx context.Context, session int32, callsign string, pdcState string, pdcRequestedAt *time.Time) error {
	if m.SetPdcRequestedFn == nil {
		panic("unexpected call to MockStripRepository.SetPdcRequested")
	}
	return m.SetPdcRequestedFn(ctx, session, callsign, pdcState, pdcRequestedAt)
}

func (m *MockStripRepository) SetPdcMessageSent(ctx context.Context, session int32, callsign string, pdcState string, pdcMessageSequence *int32, pdcMessageSent *time.Time) error {
	if m.SetPdcMessageSentFn == nil {
		panic("unexpected call to MockStripRepository.SetPdcMessageSent")
	}
	return m.SetPdcMessageSentFn(ctx, session, callsign, pdcState, pdcMessageSequence, pdcMessageSent)
}

func (m *MockStripRepository) UpdatePdcStatus(ctx context.Context, session int32, callsign string, pdcState string) error {
	if m.UpdatePdcStatusFn == nil {
		panic("unexpected call to MockStripRepository.UpdatePdcStatus")
	}
	return m.UpdatePdcStatusFn(ctx, session, callsign, pdcState)
}
