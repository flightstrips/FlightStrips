package testutil

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"
)

// ControllerOnlineCall records arguments to SendControllerOnline.
type ControllerOnlineCall struct {
	Session    int32
	Callsign   string
	Position   string
	Identifier string
}

// ControllerOfflineCall records arguments to SendControllerOffline.
type ControllerOfflineCall struct {
	Session    int32
	Callsign   string
	Position   string
	Identifier string
}

// BayEventCall records arguments to SendBayEvent.
type BayEventCall struct {
	Session  int32
	Callsign string
	Bay      string
	Sequence int32
}

// OwnersUpdateCall records arguments to SendOwnersUpdate.
type OwnersUpdateCall struct {
	Session        int32
	Callsign       string
	Owner          string
	NextOwners     []string
	PreviousOwners []string
}

// CoordinationTransferCall records arguments to SendCoordinationTransfer.
type CoordinationTransferCall struct {
	Session  int32
	Callsign string
	From     string
	To       string
}

// CoordinationAssumeCall records arguments to SendCoordinationAssume.
type CoordinationAssumeCall struct {
	Session  int32
	Callsign string
	Position string
}

// CoordinationRejectCall records arguments to SendCoordinationReject.
type CoordinationRejectCall struct {
	Session  int32
	Callsign string
	Position string
}

// CoordinationFreeCall records arguments to SendCoordinationFree.
type CoordinationFreeCall struct {
	Session  int32
	Callsign string
}

// CoordinationTagRequestCall records arguments to SendCoordinationTagRequest.
type CoordinationTagRequestCall struct {
	Session  int32
	Callsign string
	From     string
	To       string
}

// AircraftDisconnectCall records arguments to SendAircraftDisconnect.
type AircraftDisconnectCall struct {
	Session  int32
	Callsign string
}

// StripUpdateCall records arguments to SendStripUpdate.
type StripUpdateCall struct {
	Session  int32
	Callsign string
}

type CdmWaitCall struct {
	Session  int32
	Callsign string
}

type CdmUpdateCall struct {
	Session  int32
	Callsign string
	Eobt     string
	Tobt     string
	Tsat     string
	Ctot     string
}

// BulkBayCall records arguments to SendBulkBayEvent.
type BulkBayCall struct {
	Session int32
	Bay     string
	Strips  []frontend.BulkBayEntry
}

type TacticalStripMovedCall struct {
	Session  int32
	ID       int64
	Bay      string
	Sequence int32
}

// MockFrontendHub is a configurable mock for shared.FrontendHub.
// It records calls for assertion in tests.
type MockFrontendHub struct {
	server shared.Server

	BayEvents               []BayEventCall
	BulkBayEvents           []BulkBayCall
	OwnersUpdates           []OwnersUpdateCall
	CoordinationTransfers   []CoordinationTransferCall
	CoordinationAssumes     []CoordinationAssumeCall
	CoordinationRejects     []CoordinationRejectCall
	CoordinationFrees       []CoordinationFreeCall
	CoordinationTagRequests []CoordinationTagRequestCall
	AircraftDisconnects     []AircraftDisconnectCall
	StripUpdates            []StripUpdateCall
	ControllerOnlines       []ControllerOnlineCall
	ControllerOfflines      []ControllerOfflineCall
	CdmWaits                []CdmWaitCall
	CdmUpdates              []CdmUpdateCall
	TacticalStripMoves      []TacticalStripMovedCall
}

func (m *MockFrontendHub) GetServer() shared.Server {
	return m.server
}

func (m *MockFrontendHub) SetServer(server shared.Server) {
	m.server = server
}

func (m *MockFrontendHub) Broadcast(session int32, message frontend.OutgoingMessage) {}

func (m *MockFrontendHub) Send(session int32, cid string, message frontend.OutgoingMessage) {}

func (m *MockFrontendHub) CidOnline(session int32, cid string) {}

func (m *MockFrontendHub) CidDisconnect(cid string) {}

func (m *MockFrontendHub) SendStripUpdate(session int32, callsign string) {
	m.StripUpdates = append(m.StripUpdates, StripUpdateCall{session, callsign})
}

func (m *MockFrontendHub) SendControllerOnline(session int32, callsign string, position string, identifier string) {
	m.ControllerOnlines = append(m.ControllerOnlines, ControllerOnlineCall{session, callsign, position, identifier})
}

func (m *MockFrontendHub) SendControllerOffline(session int32, callsign string, position string, identifier string) {
	m.ControllerOfflines = append(m.ControllerOfflines, ControllerOfflineCall{session, callsign, position, identifier})
}

func (m *MockFrontendHub) SendAssignedSquawkEvent(session int32, callsign string, squawk string) {}

func (m *MockFrontendHub) SendSquawkEvent(session int32, callsign string, squawk string) {}

func (m *MockFrontendHub) SendRequestedAltitudeEvent(session int32, callsign string, altitude int32) {
}

func (m *MockFrontendHub) SendClearedAltitudeEvent(session int32, callsign string, altitude int32) {}

func (m *MockFrontendHub) SendBayEvent(session int32, callsign string, bay string, sequence int32) {
	m.BayEvents = append(m.BayEvents, BayEventCall{session, callsign, bay, sequence})
}

func (m *MockFrontendHub) SendBulkBayEvent(session int32, bay string, strips []frontend.BulkBayEntry) {
	m.BulkBayEvents = append(m.BulkBayEvents, BulkBayCall{session, bay, strips})
}

func (m *MockFrontendHub) SendAircraftDisconnect(session int32, callsign string) {
	m.AircraftDisconnects = append(m.AircraftDisconnects, AircraftDisconnectCall{session, callsign})
}

func (m *MockFrontendHub) SendStandEvent(session int32, callsign string, stand string) {}

func (m *MockFrontendHub) SendSetHeadingEvent(session int32, callsign string, heading int32) {}

func (m *MockFrontendHub) SendCommunicationTypeEvent(session int32, callsign string, communicationType string) {
}

func (m *MockFrontendHub) SendCoordinationTransfer(session int32, callsign, from, to string) {
	m.CoordinationTransfers = append(m.CoordinationTransfers, CoordinationTransferCall{session, callsign, from, to})
}

func (m *MockFrontendHub) SendCoordinationAssume(session int32, callsign, position string) {
	m.CoordinationAssumes = append(m.CoordinationAssumes, CoordinationAssumeCall{session, callsign, position})
}

func (m *MockFrontendHub) SendCoordinationReject(session int32, callsign, position string) {
	m.CoordinationRejects = append(m.CoordinationRejects, CoordinationRejectCall{session, callsign, position})
}

func (m *MockFrontendHub) SendCoordinationFree(session int32, callsign string) {
	m.CoordinationFrees = append(m.CoordinationFrees, CoordinationFreeCall{session, callsign})
}

func (m *MockFrontendHub) SendOwnersUpdate(session int32, callsign string, owner string, nextOwners []string, previousOwners []string) {
	m.OwnersUpdates = append(m.OwnersUpdates, OwnersUpdateCall{session, callsign, owner, nextOwners, previousOwners})
}

func (m *MockFrontendHub) SendLayoutUpdates(session int32, layoutMap map[string]string) {}

func (m *MockFrontendHub) SendCdmUpdate(session int32, callsign, eobt, tobt, tsat, ctot string) {
	m.CdmUpdates = append(m.CdmUpdates, CdmUpdateCall{
		Session:  session,
		Callsign: callsign,
		Eobt:     eobt,
		Tobt:     tobt,
		Tsat:     tsat,
		Ctot:     ctot,
	})
}

func (m *MockFrontendHub) SendCdmWait(session int32, callsign string) {
	m.CdmWaits = append(m.CdmWaits, CdmWaitCall{Session: session, Callsign: callsign})
}

func (m *MockFrontendHub) SendPdcStateChange(session int32, callsign, state, remarks string) {}

func (m *MockFrontendHub) SendRunwayConfiguration(session int32, departure, arrival []string, status map[string]string) {
}

func (m *MockFrontendHub) SendTacticalStripCreated(session int32, strip frontend.TacticalStripPayload) {
}

func (m *MockFrontendHub) SendTacticalStripDeleted(session int32, id int64, bay string) {}

func (m *MockFrontendHub) SendTacticalStripUpdated(session int32, strip frontend.TacticalStripPayload) {
}

func (m *MockFrontendHub) SendTacticalStripMoved(session int32, id int64, bay string, sequence int32) {
	m.TacticalStripMoves = append(m.TacticalStripMoves, TacticalStripMovedCall{
		Session:  session,
		ID:       id,
		Bay:      bay,
		Sequence: sequence,
	})
}

func (m *MockFrontendHub) SendBroadcast(session int32, message string, from string) {}

func (m *MockFrontendHub) SendCoordinationTagRequest(session int32, callsign, from, to string) {
	m.CoordinationTagRequests = append(m.CoordinationTagRequests, CoordinationTagRequestCall{session, callsign, from, to})
}

func (m *MockFrontendHub) SendAvailableSids(session int32, sids pkgModels.AvailableSids) {}
