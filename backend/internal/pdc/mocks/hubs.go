package mocks

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/mock"
)

type FrontendHub struct {
	mock.Mock
}

func (m *FrontendHub) SetServer(server shared.Server) {
	m.Called(server)
}

func (m *FrontendHub) GetServer() shared.Server {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(shared.Server)
}

func (m *FrontendHub) Broadcast(session int32, message frontend.OutgoingMessage) {
	m.Called(session, message)
}

func (m *FrontendHub) Send(session int32, cid string, message frontend.OutgoingMessage) {
	m.Called(session, cid, message)
}

func (m *FrontendHub) CidOnline(session int32, cid string) {
	m.Called(session, cid)
}

func (m *FrontendHub) CidDisconnect(cid string) {
	m.Called(cid)
}

func (m *FrontendHub) SendStripUpdate(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *FrontendHub) SendControllerOnline(session int32, callsign string, position string, identifier string) {
	m.Called(session, callsign, position, identifier)
}

func (m *FrontendHub) SendControllerOffline(session int32, callsign string, position string, identifier string) {
	m.Called(session, callsign, position, identifier)
}

func (m *FrontendHub) SendAssignedSquawkEvent(session int32, callsign string, squawk string) {
	m.Called(session, callsign, squawk)
}

func (m *FrontendHub) SendSquawkEvent(session int32, callsign string, squawk string) {
	m.Called(session, callsign, squawk)
}

func (m *FrontendHub) SendRequestedAltitudeEvent(session int32, callsign string, altitude int32) {
	m.Called(session, callsign, altitude)
}

func (m *FrontendHub) SendClearedAltitudeEvent(session int32, callsign string, altitude int32) {
	m.Called(session, callsign, altitude)
}

func (m *FrontendHub) SendBayEvent(session int32, callsign string, bay string, sequence int32) {
	m.Called(session, callsign, bay, sequence)
}

func (m *FrontendHub) SendAircraftDisconnect(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *FrontendHub) SendStandEvent(session int32, callsign string, stand string) {
	m.Called(session, callsign, stand)
}

func (m *FrontendHub) SendSetHeadingEvent(session int32, callsign string, heading int32) {
	m.Called(session, callsign, heading)
}

func (m *FrontendHub) SendCommunicationTypeEvent(session int32, callsign string, communicationType string) {
	m.Called(session, callsign, communicationType)
}

func (m *FrontendHub) SendCoordinationTransfer(session int32, callsign, from, to string) {
	m.Called(session, callsign, from, to)
}

func (m *FrontendHub) SendCoordinationAssume(session int32, callsign, position string) {
	m.Called(session, callsign, position)
}

func (m *FrontendHub) SendCoordinationReject(session int32, callsign, position string) {
	m.Called(session, callsign, position)
}

func (m *FrontendHub) SendCoordinationFree(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *FrontendHub) SendOwnersUpdate(session int32, callsign string, owner string, nextOwners []string, previousOwners []string) {
	m.Called(session, callsign, owner, nextOwners, previousOwners)
}

func (m *FrontendHub) SendLayoutUpdates(session int32, layoutMap map[string]string) {
	m.Called(session, layoutMap)
}

func (m *FrontendHub) SendCdmUpdate(session int32, callsign, eobt, tobt, tsat, ctot string) {
	m.Called(session, callsign, eobt, tobt, tsat, ctot)
}

func (m *FrontendHub) SendCdmWait(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *FrontendHub) SendPdcStateChange(session int32, callsign, state string) {
	m.Called(session, callsign, state)
}

func (m *FrontendHub) SendRunwayConfiguration(session int32, departure, arrival []string) {
	m.Called(session, departure, arrival)
}

type EuroscopeHub struct {
	mock.Mock
}

func (m *EuroscopeHub) SetServer(server shared.Server) {
	m.Called(server)
}

func (m *EuroscopeHub) GetServer() shared.Server {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(shared.Server)
}

func (m *EuroscopeHub) Broadcast(session int32, message euroscope.OutgoingMessage) {
	m.Called(session, message)
}

func (m *EuroscopeHub) Send(session int32, cid string, message euroscope.OutgoingMessage) {
	m.Called(session, cid, message)
}

func (m *EuroscopeHub) SendGenerateSquawk(session int32, cid string, callsign string) {
	m.Called(session, cid, callsign)
}

func (m *EuroscopeHub) SendGroundState(session int32, cid string, callsign string, state string) {
	m.Called(session, cid, callsign, state)
}

func (m *EuroscopeHub) SendClearedFlag(session int32, cid string, callsign string, flag bool) {
	m.Called(session, cid, callsign, flag)
}

func (m *EuroscopeHub) SendStand(session int32, cid string, callsign string, stand string) {
	m.Called(session, cid, callsign, stand)
}

func (m *EuroscopeHub) SendRoute(session int32, cid string, callsign string, route string) {
	m.Called(session, cid, callsign, route)
}

func (m *EuroscopeHub) SendRemarks(session int32, cid string, callsign string, remarks string) {
	m.Called(session, cid, callsign, remarks)
}

func (m *EuroscopeHub) SendSid(session int32, cid string, callsign string, sid string) {
	m.Called(session, cid, callsign, sid)
}

func (m *EuroscopeHub) SendAssignedSquawk(session int32, cid string, callsign string, squawk string) {
	m.Called(session, cid, callsign, squawk)
}

func (m *EuroscopeHub) SendRunway(session int32, cid string, callsign string, runway string) {
	m.Called(session, cid, callsign, runway)
}

func (m *EuroscopeHub) SendClearedAltitude(session int32, cid string, callsign string, altitude int32) {
	m.Called(session, cid, callsign, altitude)
}

func (m *EuroscopeHub) SendHeading(session int32, cid string, callsign string, heading int32) {
	m.Called(session, cid, callsign, heading)
}
