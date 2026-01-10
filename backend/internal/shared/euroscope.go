package shared

import (
	"FlightStrips/pkg/events/euroscope"
)

type EuroscopeHub interface {
	ServerInjectable
	Hub[euroscope.OutgoingMessage]
	SendGenerateSquawk(session int32, cid string, callsign string)
	SendGroundState(session int32, cid string, callsign string, state string)
	SendClearedFlag(session int32, cid string, callsign string, flag bool)
	SendStand(session int32, cid string, callsign string, stand string)
	SendRoute(session int32, cid string, callsign string, route string)
	SendRemarks(session int32, cid string, callsign string, remarks string)
	SendSid(session int32, cid string, callsign string, sid string)
	SendAssignedSquawk(session int32, cid string, callsign string, squawk string)
	SendRunway(session int32, cid string, callsign string, runway string)
	SendClearedAltitude(session int32, cid string, callsign string, altitude int32)
	SendHeading(session int32, cid string, callsign string, heading int32)
}
