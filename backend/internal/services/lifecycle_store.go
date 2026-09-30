package services

import (
	"FlightStrips/internal/models"
	"context"
	"errors"
)

var errLifecycleNotFound = errors.New("lifecycle entity absent")

// Lifecycle stores are domain planning ports. SQL repositories implement them
// in the legacy runtime; the candidate uses an isolated planning snapshot.
type lifecycleAssignments interface {
	CreateAssignment(context.Context, *models.StandAssignment) error
	GetAssignment(context.Context, int32, string) (*models.StandAssignment, error)
	ListAssignments(context.Context, int32) ([]*models.StandAssignment, error)
	UpdateAssignment(context.Context, *models.StandAssignment) (int64, error)
	DeleteAssignment(context.Context, int32, int64, int32) (int64, error)
}
type lifecycleStrips interface {
	GetByCallsign(context.Context, int32, string) (*models.Strip, error)
	LockByCallsign(context.Context, int32, string) (*models.Strip, error)
	UpdateStand(context.Context, int32, string, *string, *int32) (int64, error)
}
type lifecycleAllocator interface {
	Allocate(context.Context, StandAllocationRequest) (*StandAllocationResult, error)
	Reallocate(context.Context, StandAllocationRequest) (*StandAllocationResult, error)
	assignObservedStand(context.Context, StandAllocationRequest) (*StandAllocationResult, error)
	ReleaseAssignment(context.Context, *models.StandAssignment) error
	ReleaseAssignmentRetainingStand(context.Context, *models.StandAssignment) error
	releaseAssignment(context.Context, *models.StandAssignment, bool) error
	PublishAssignment(context.Context, models.StandAssignment) error
	PublishConfirmedArrival(context.Context, models.StandAssignment) error
	StandAvailable(context.Context, StandAllocationRequest, string) (bool, error)
	ConfirmedArrivalConflictAtStand(context.Context, StandAllocationRequest, string) (bool, error)
	ReconcileObservedDepartureConflict(context.Context, StandAllocationRequest, string) error
	ReconcileUnsafeAssignments(context.Context, int32, string) error
	ReleaseExpiredBlocks(context.Context, int32) error
	SetDisplacedArrivalHandler(DisplacedArrivalHandler)
}
