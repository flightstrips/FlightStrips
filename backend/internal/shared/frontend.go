package shared

import (
	"FlightStrips/pkg/events/frontend"
)

type FrontendHub interface {
	ServerInjectable
	Broadcast(session int32, message frontend.OutgoingMessage)
	Send(session int32, cid string, message frontend.OutgoingMessage)
	CidOnline(session int32, cid string)
	CidDisconnect(cid string)
	SendStripUpdate(session int32, callsign string)
	SendControllerOnline(session int32, callsign string, position string, identifier string)
	SendControllerOffline(session int32, callsign string, position string, identifier string)
	SendAssignedSquawkEvent(session int32, callsign string, squawk string)
	SendSquawkEvent(session int32, callsign string, squawk string)
	SendRequestedAltitudeEvent(session int32, callsign string, altitude int32)
	SendClearedAltitudeEvent(session int32, callsign string, altitude int32)
	SendBayEvent(session int32, callsign string, bay string, sequence int32)
	SendAircraftDisconnect(session int32, callsign string)
	SendStandEvent(session int32, callsign string, stand string)
	SendSetHeadingEvent(session int32, callsign string, heading int32)
	SendCommunicationTypeEvent(session int32, callsign string, communicationType string)
	SendCoordinationTransfer(session int32, callsign, from, to string)
	SendCoordinationAssume(session int32, callsign, position string)
	SendCoordinationReject(session int32, callsign, position string)
	SendCoordinationFree(session int32, callsign string)
	SendOwnersUpdate(session int32, callsign string, owner string, nextOwners []string, previousOwners []string)
	SendLayoutUpdates(session int32, layoutMap map[string]string)
	SendCdmUpdate(session int32, callsign, eobt, tobt, tsat, ctot string)
	SendCdmWait(session int32, callsign string)
	SendPdcStateChange(session int32, callsign, state string)
	SendRunwayConfiguration(session int32, departure, arrival []string)
}
