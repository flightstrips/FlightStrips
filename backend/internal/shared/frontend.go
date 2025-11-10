package shared

import (
	"FlightStrips/pkg/events/frontend"
)

type FrontendHub interface {
	ServerInjectable
	Hub[frontend.OutgoingMessage]

	CidOnline(session int32, cid string)
	CidDisconnect(cid string)
	SendStripUpdate(session int32, callsign string)
	SendControllerOnline(session int32, callsign string, position string, identifier string)
	SendControllerOffline(session int32, callsign string, position string, identifier string)
	SendAssignedSquawkEvent(session int32, callsign string, squawk string)
	SendSquawkEvent(session int32, callsign string, squawk string)
	SendRequestedAltitudeEvent(session int32, callsign string, altitude int)
	SendClearedAltitudeEvent(session int32, callsign string, altitude int)
	SendBayEvent(session int32, callsign string, bay string, sequence int32)
	SendAircraftDisconnect(session int32, callsign string)
	SendStandEvent(session int32, callsign string, stand string)
	SendSetHeadingEvent(session int32, callsign string, heading int)
	SendCommunicationTypeEvent(session int32, callsign string, communicationType string)
	SendCoordinationTransfer(session int32, callsign, from, to string)
	SendCoordinationAssume(session int32, callsign, position string)
	SendCoordinationReject(session int32, callsign, position string)
	SendCoordinationFree(session int32, callsign string)
	SendOwnersUpdate(session int32, callsign string, nextOwners []string, previousOwners []string)
	SendLayoutUpdates(session int32, layoutMap map[string]string)
}
