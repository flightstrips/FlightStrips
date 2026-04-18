package shared

import (
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
)

// StripEventPublisher is the narrow interface of frontend-broadcast methods
// the strip service uses. It is a subset of FrontendHub so any concrete hub
// automatically satisfies it; services depend on this narrow view to make
// their event surface explicit.
type StripEventPublisher interface {
	ServerInjectable

	Broadcast(session int32, message frontend.OutgoingMessage)
	SendStripUpdate(session int32, callsign string)
	SendAssignedSquawkEvent(session int32, callsign string, squawk string)
	SendSquawkEvent(session int32, callsign string, squawk string)
	SendRequestedAltitudeEvent(session int32, callsign string, altitude int32)
	SendClearedAltitudeEvent(session int32, callsign string, altitude int32)
	SendBayEvent(session int32, callsign string, bay string, sequence int32)
	SendBulkBayEvent(session int32, bay string, strips []frontend.BulkBayEntry)
	SendAircraftDisconnect(session int32, callsign string)
	SendStandEvent(session int32, callsign string, stand string)
	SendSetHeadingEvent(session int32, callsign string, heading int32)
	SendCommunicationTypeEvent(session int32, callsign string, communicationType string)
	SendCoordinationTransfer(session int32, callsign, from, to string)
	SendCoordinationAssume(session int32, callsign, position string)
	SendCoordinationReject(session int32, callsign, position string)
	SendCoordinationFree(session int32, callsign string)
	SendCoordinationTagRequest(session int32, callsign, from, to string)
	SendOwnersUpdate(session int32, callsign string, owner string, nextOwners []string, previousOwners []string)
	SendTacticalStripMoved(session int32, id int64, bay string, sequence int32)
}

// EuroscopeStripCommander is the narrow interface of CID-targeted EuroScope
// commands the strip service uses. It is a subset of EuroscopeHub.
type EuroscopeStripCommander interface {
	SendGenerateSquawk(session int32, cid string, callsign string)
	SendGroundState(session int32, cid string, callsign string, state string)
	SendClearedFlag(session int32, cid string, callsign string, flag bool)
	SendClearedAltitude(session int32, cid string, callsign string, altitude int32)
	SendCoordinationHandover(session int32, cid string, callsign string, targetCallsign string)
	SendAssumeOnly(session int32, cid string, callsign string)
	SendDropTracking(session int32, cid string, callsign string)
	SendCreateFPL(session int32, cid string, event euroscope.CreateFPLEvent)
}

// CdmEventPublisher is the narrow frontend-broadcast interface the CDM service
// uses. It is a subset of FrontendHub.
type CdmEventPublisher interface {
	ServerInjectable

	SendCdmUpdate(session int32, callsign, eobt, tobt, tsat, ctot string)
	SendCdmWait(session int32, callsign string)
}
