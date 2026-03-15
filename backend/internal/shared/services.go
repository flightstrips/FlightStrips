package shared

import (
	"FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"context"
)

// ControllerOnlineResult describes what happened when a controller came online.
type ControllerOnlineResult struct {
	// SectorChanges is non-nil when sectors were updated and a broadcast notification
	// should be scheduled.
	SectorChanges []SectorChange
	// SingleOnPosition is true when this controller is the only one on the position.
	SingleOnPosition bool
	// NotifyOnline is true when the frontend should be sent a controller_online event.
	// False for heartbeat events where the position did not change.
	NotifyOnline bool
}

// ControllerOfflineResult describes whether the handler should schedule an offline timer.
type ControllerOfflineResult struct {
	// ShouldScheduleTimer is true when the handler should call scheduleOfflineActions.
	ShouldScheduleTimer bool
	// PositionFrequency is the controller's position frequency string.
	PositionFrequency string
	// PositionName is the human-readable position name.
	PositionName string
}

// ControllerService owns all business logic for controller online/offline events.
type ControllerService interface {
	// ControllerOnline handles a controller coming online. positionName is pre-resolved
	// from config by the caller. Returns sector changes and whether the controller is
	// the only one on its position, so the caller can schedule a broadcast notification.
	ControllerOnline(ctx context.Context, session int32, callsign, position, positionName string) (ControllerOnlineResult, error)

	// ControllerOffline handles a controller going offline. Returns whether the caller
	// should schedule a delayed offline timer.
	ControllerOffline(ctx context.Context, session int32, callsign string) (ControllerOfflineResult, error)

	// UpsertController creates or updates a controller's position (used by sync).
	UpsertController(ctx context.Context, session int32, callsign, position string) error
}

type StripService interface {
	// Movement & ordering
	MoveToBay(ctx context.Context, session int32, callsign string, bay string, sendNotification bool) error
	MoveStripBetween(ctx context.Context, session int32, callsign string, insertAfter *frontend.StripRef, bay string) error
	MoveTacticalStripBetween(ctx context.Context, session int32, id int64, insertAfter *frontend.StripRef, bay string) error

	// Coordination
	CreateCoordinationTransfer(ctx context.Context, session int32, callsign string, from string, to string) error
	CreateEsArrivalCoordination(ctx context.Context, session int32, callsign string, from string, to string, esHandoverCid *string) error
	AcceptCoordination(ctx context.Context, session int32, callsign string, assumingPosition string) error
	AssumeStripCoordination(ctx context.Context, session int32, callsign string, position string) error
	ForceAssumeStrip(ctx context.Context, session int32, callsign string, position string) error
	RejectCoordination(ctx context.Context, session int32, callsign string, position string) error
	CancelCoordinationTransfer(ctx context.Context, session int32, callsign string, position string) error
	FreeStrip(ctx context.Context, session int32, callsign string, position string) error
	AutoTransferAirborneStrip(ctx context.Context, session int32, callsign string) error

	// Cleared bay operations
	ClearStrip(ctx context.Context, session int32, callsign string, cid string) error
	UnclearStrip(ctx context.Context, session int32, callsign string, cid string) error

	// Auto-assumption
	AutoAssumeForClearedStrip(ctx context.Context, session int32, callsign string, stripVersion int32) error
	AutoAssumeForControllerOnline(ctx context.Context, session int32, controllerPosition string) error

	// EuroScope field updates — each method reads, applies business rules, persists, and notifies frontend.
	UpdateAssignedSquawk(ctx context.Context, session int32, callsign string, squawk string) error
	UpdateSquawk(ctx context.Context, session int32, callsign string, squawk string) error
	UpdateRequestedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error
	UpdateClearedAltitude(ctx context.Context, session int32, callsign string, altitude int32) error
	UpdateCommunicationType(ctx context.Context, session int32, callsign string, commType string) error
	UpdateHeading(ctx context.Context, session int32, callsign string, heading int32) error
	DeleteStrip(ctx context.Context, session int32, callsign string) error
	UpdateGroundState(ctx context.Context, session int32, callsign string, groundState string, airport string) error
	UpdateClearedFlag(ctx context.Context, session int32, callsign string, cleared bool) error
	UpdateStand(ctx context.Context, session int32, callsign string, stand string) error
	UpdateAircraftPosition(ctx context.Context, session int32, callsign string, lat, lon float64, altitude int32, airport string) error
	HandleTrackingControllerChanged(ctx context.Context, session int32, callsign string, trackingController string) error
	HandleCoordinationReceived(ctx context.Context, session int32, callsign string, controllerCallsign string) error
	SyncStrip(ctx context.Context, session int32, strip interface{}, airport string) error

	// Frontend move operations — called when a frontend user drags a strip to a new bay.
	UpdateClearedFlagForMove(ctx context.Context, session int32, callsign string, isCleared bool, bay string, cid string) error
	UpdateGroundStateForMove(ctx context.Context, session int32, callsign string, bay string, cid string, airport string) error

	// Frontend strip mutations
	UpdateReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string) error
	// ApplyReleasePoint updates the release point with ownership enforcement.
	// Non-owners may overwrite an existing value (marks the cell yellow); non-owners
	// setting a value on a strip that has none are rejected.
	ApplyReleasePoint(ctx context.Context, session int32, callsign string, releasePoint string, clientPosition string) error
	UpdateMarked(ctx context.Context, session int32, callsign string, marked bool) error
	RunwayClearance(ctx context.Context, session int32, callsign string) error
	PropagateRunwayChange(ctx context.Context, session int32, airport string, oldRunways models.ActiveRunways, newRunways models.ActiveRunways) error
}
