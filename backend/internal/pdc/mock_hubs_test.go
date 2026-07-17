package pdc

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"

	"github.com/stretchr/testify/mock"
)

type mockPdcFrontendHub struct {
	mock.Mock
}

func (m *mockPdcFrontendHub) SetServer(server shared.Server) {
	m.Called(server)
}

func (m *mockPdcFrontendHub) GetServer() shared.Server {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(shared.Server)
}

func (m *mockPdcFrontendHub) Broadcast(session int32, message frontend.OutgoingMessage) {
	m.Called(session, message)
}

func (m *mockPdcFrontendHub) Send(session int32, cid string, message frontend.OutgoingMessage) {
	m.Called(session, cid, message)
}

func (m *mockPdcFrontendHub) GetAtisCodes(session int32) (string, string) {
	for _, expected := range m.ExpectedCalls {
		if expected.Method == "GetAtisCodes" {
			args := m.Called(session)

			var arr string
			if args.Get(0) != nil {
				arr = args.String(0)
			}

			var dep string
			if args.Get(1) != nil {
				dep = args.String(1)
			}

			return arr, dep
		}
	}

	return "", ""
}

func (m *mockPdcFrontendHub) CidOnline(session int32, cid string) {
	m.Called(session, cid)
}

func (m *mockPdcFrontendHub) CidDisconnect(cid string) {
	m.Called(cid)
}

func (m *mockPdcFrontendHub) SendStripUpdate(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *mockPdcFrontendHub) SendControllerOnline(session int32, callsign string, position string, identifier string, ownedSectors []string) {
	m.Called(session, callsign, position, identifier, ownedSectors)
}

func (m *mockPdcFrontendHub) SendControllerUpdate(session int32, callsign string, position string, identifier string, ownedSectors []string) {
	m.Called(session, callsign, position, identifier, ownedSectors)
}

func (m *mockPdcFrontendHub) SendControllerOffline(session int32, callsign string, position string, identifier string) {
	m.Called(session, callsign, position, identifier)
}

func (m *mockPdcFrontendHub) SendAssignedSquawkEvent(session int32, callsign string, squawk string) {
	m.Called(session, callsign, squawk)
}

func (m *mockPdcFrontendHub) SendSquawkEvent(session int32, callsign string, squawk string) {
	m.Called(session, callsign, squawk)
}

func (m *mockPdcFrontendHub) SendRequestedAltitudeEvent(session int32, callsign string, altitude int32) {
	m.Called(session, callsign, altitude)
}

func (m *mockPdcFrontendHub) SendClearedAltitudeEvent(session int32, callsign string, altitude int32) {
	m.Called(session, callsign, altitude)
}

func (m *mockPdcFrontendHub) SendBayEvent(session int32, callsign string, bay string, sequence int32) {
	m.Called(session, callsign, bay, sequence)
}

func (m *mockPdcFrontendHub) SendBulkBayEvent(session int32, bay string, strips []frontend.BulkBayEntry) {
	m.Called(session, bay, strips)
}

func (m *mockPdcFrontendHub) SendAircraftDisconnect(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *mockPdcFrontendHub) SendStandEvent(session int32, callsign string, stand string) {
	m.Called(session, callsign, stand)
}

func (m *mockPdcFrontendHub) SendSetHeadingEvent(session int32, callsign string, heading int32) {
	m.Called(session, callsign, heading)
}

func (m *mockPdcFrontendHub) SendCommunicationTypeEvent(session int32, callsign string, communicationType string) {
	m.Called(session, callsign, communicationType)
}

func (m *mockPdcFrontendHub) SendCoordinationTransfer(session int32, callsign, from, to string) {
	m.Called(session, callsign, from, to)
}

func (m *mockPdcFrontendHub) SendCoordinationAssume(session int32, callsign, position string) {
	m.Called(session, callsign, position)
}

func (m *mockPdcFrontendHub) SendCoordinationReject(session int32, callsign, position string) {
	m.Called(session, callsign, position)
}

func (m *mockPdcFrontendHub) SendCoordinationFree(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *mockPdcFrontendHub) SendOwnersUpdate(session int32, callsign string, owner string, nextOwners []string, previousOwners []string, nextDisplay *internalModels.NextDisplay) {
	m.Called(session, callsign, owner, nextOwners, previousOwners, nextDisplay)
}

func (m *mockPdcFrontendHub) SendLayoutUpdates(session int32, layoutMap map[string]string) {
	m.Called(session, layoutMap)
}

func (m *mockPdcFrontendHub) SendCdmUpdate(session int32, event frontend.CdmDataEvent) {
	m.Called(session, event)
}

func (m *mockPdcFrontendHub) SendCdmUpdates(session int32, events []frontend.CdmDataEvent) {
	m.Called(session, events)
}

func (m *mockPdcFrontendHub) SendCdmWait(session int32, callsign string) {
	m.Called(session, callsign)
}

func (m *mockPdcFrontendHub) SendPdcStateChange(session int32, callsign, state, remarks string) {
	m.Called(session, callsign, state, remarks)
}

func (m *mockPdcFrontendHub) SendMessage(session int32, sender, text string, recipients []string) {
	m.Called(session, sender, text, recipients)
}

func (m *mockPdcFrontendHub) SendRunwayConfiguration(session int32, departure, arrival []string, status map[string]string) {
	m.Called(session, departure, arrival, status)
}

func (m *mockPdcFrontendHub) SendTacticalStripCreated(session int32, strip frontend.TacticalStripPayload) {
	m.Called(session, strip)
}

func (m *mockPdcFrontendHub) SendTacticalStripDeleted(session int32, id int64, bay string) {
	m.Called(session, id, bay)
}

func (m *mockPdcFrontendHub) SendTacticalStripUpdated(session int32, strip frontend.TacticalStripPayload) {
	m.Called(session, strip)
}

func (m *mockPdcFrontendHub) SendTacticalStripMoved(session int32, id int64, bay string, sequence int32) {
	m.Called(session, id, bay, sequence)
}

func (m *mockPdcFrontendHub) SendBroadcast(session int32, message string, from string) {
	m.Called(session, message, from)
}

func (m *mockPdcFrontendHub) SendCoordinationTagRequest(session int32, callsign, from, to string) {
	m.Called(session, callsign, from, to)
}

func (m *mockPdcFrontendHub) SendAvailableSids(session int32, sids pkgModels.AvailableSids) {}

var _ internalModels.TacticalStrip // ensure import used

type mockPdcEuroscopeHub struct {
	mock.Mock
}

func (m *mockPdcEuroscopeHub) HasActiveClientForAirport(airport string) bool {
	args := m.Called(airport)
	return args.Bool(0)
}

func (m *mockPdcEuroscopeHub) SetServer(server shared.Server) {
	m.Called(server)
}

func (m *mockPdcEuroscopeHub) GetServer() shared.Server {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(shared.Server)
}

func (m *mockPdcEuroscopeHub) Broadcast(session int32, message euroscope.OutgoingMessage) {
	m.Called(session, message)
}

func (m *mockPdcEuroscopeHub) BroadcastCdmUpdates(session int32, events []euroscope.CdmUpdateEvent) {
	m.Called(session, events)
}

func (m *mockPdcEuroscopeHub) Send(session int32, cid string, message euroscope.OutgoingMessage) {
	m.Called(session, cid, message)
}

func (m *mockPdcEuroscopeHub) SendGenerateSquawk(session int32, cid string, callsign string) {
	m.Called(session, cid, callsign)
}

func (m *mockPdcEuroscopeHub) SendGroundState(session int32, cid string, callsign string, state string) {
	m.Called(session, cid, callsign, state)
}

func (m *mockPdcEuroscopeHub) SendClearedFlag(session int32, cid string, callsign string, flag bool) {
	m.Called(session, cid, callsign, flag)
}

func (m *mockPdcEuroscopeHub) SendStand(session int32, cid string, callsign string, stand string) {
	m.Called(session, cid, callsign, stand)
}

func (m *mockPdcEuroscopeHub) SendEobt(session int32, cid string, callsign string, eobt string) {
	m.Called(session, cid, callsign, eobt)
}

func (m *mockPdcEuroscopeHub) SendRoute(session int32, cid string, callsign string, route string) {
	m.Called(session, cid, callsign, route)
}

func (m *mockPdcEuroscopeHub) SendRemarks(session int32, cid string, callsign string, remarks string) {
	m.Called(session, cid, callsign, remarks)
}

func (m *mockPdcEuroscopeHub) SendAircraftInfo(session int32, cid string, callsign string, aircraftType string) {
	m.Called(session, cid, callsign, aircraftType)
}

func (m *mockPdcEuroscopeHub) SendAircraftInfoAndRemarks(session int32, cid string, callsign string, aircraftType string, remarks string) {
	m.Called(session, cid, callsign, aircraftType, remarks)
}

func (m *mockPdcEuroscopeHub) SendSid(session int32, cid string, callsign string, sid string) {
	m.Called(session, cid, callsign, sid)
}

func (m *mockPdcEuroscopeHub) SendAssignedSquawk(session int32, cid string, callsign string, squawk string) {
	m.Called(session, cid, callsign, squawk)
}

func (m *mockPdcEuroscopeHub) SendRunway(session int32, cid string, callsign string, runway string) {
	m.Called(session, cid, callsign, runway)
}

func (m *mockPdcEuroscopeHub) SendClearedAltitude(session int32, cid string, callsign string, altitude int32) {
	m.Called(session, cid, callsign, altitude)
}

func (m *mockPdcEuroscopeHub) SendHeading(session int32, cid string, callsign string, heading int32) {
	m.Called(session, cid, callsign, heading)
}

func (m *mockPdcEuroscopeHub) SendCoordinationHandover(session int32, cid string, callsign string, targetCallsign string) {
	m.Called(session, cid, callsign, targetCallsign)
}

func (m *mockPdcEuroscopeHub) SendAssumeOnly(session int32, cid string, callsign string) {
	m.Called(session, cid, callsign)
}

func (m *mockPdcEuroscopeHub) SendAssumeAndDrop(session int32, cid string, callsign string) {
	m.Called(session, cid, callsign)
}

func (m *mockPdcEuroscopeHub) SendDropTracking(session int32, cid string, callsign string) {
	m.Called(session, cid, callsign)
}

func (m *mockPdcEuroscopeHub) GetMasterCallsign(session int32) string {
	return ""
}

func (m *mockPdcEuroscopeHub) GetMasterCid(session int32) string {
	args := m.Called(session)
	return args.String(0)
}

func (m *mockPdcEuroscopeHub) GetClientLocalIP(session int32, cid string) string {
	return ""
}

func (m *mockPdcEuroscopeHub) IsObserverCid(cid string) bool {
	return false
}

func (m *mockPdcEuroscopeHub) IsSessionSynced(sessionId int32) bool {
	return true
}

func (m *mockPdcEuroscopeHub) GetRunwayMismatchStatus(session int32, cid string) (bool, bool) {
	return false, false
}

func (m *mockPdcEuroscopeHub) SendCreateFPL(session int32, cid string, event euroscope.CreateFPLEvent) {
	m.Called(session, cid, event)
}

func (m *mockPdcEuroscopeHub) SendPdcStateChange(session int32, callsign, state, remarks string) {
	m.Called(session, callsign, state, remarks)
}
