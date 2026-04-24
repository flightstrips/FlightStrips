package testutil

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events/euroscope"
)

// ClearedFlagCall records arguments to SendClearedFlag.
type ClearedFlagCall struct {
	Session  int32
	Cid      string
	Callsign string
	Flag     bool
}

// GroundStateCall records arguments to SendGroundState.
type GroundStateCall struct {
	Session     int32
	Cid         string
	Callsign    string
	GroundState string
}

// CoordinationHandoverCall records arguments to SendCoordinationHandover.
type CoordinationHandoverCall struct {
	Session        int32
	Cid            string
	Callsign       string
	TargetCallsign string
}

type AssumeOnlyCall struct {
	Session  int32
	Cid      string
	Callsign string
}

type AssumeAndDropCall struct {
	Session  int32
	Cid      string
	Callsign string
}

type DropTrackingCall struct {
	Session  int32
	Cid      string
	Callsign string
}

type CdmReadyRequestCall struct {
	Session  int32
	Cid      string
	Callsign string
}

type GenerateSquawkCall struct {
	Session  int32
	Cid      string
	Callsign string
}

// ClearedAltitudeCall records arguments to SendClearedAltitude.
type ClearedAltitudeCall struct {
	Session  int32
	Cid      string
	Callsign string
	Altitude int32
}

// MockEuroscopeHub is a configurable mock for shared.EuroscopeHub.
// It records calls for assertion in tests.
type MockEuroscopeHub struct {
	server shared.Server

	HasActiveClientForAirportFn func(airport string) bool
	GetMasterCallsignFn         func(session int32) string

	ClearedFlags          []ClearedFlagCall
	GroundStates          []GroundStateCall
	CoordinationHandovers []CoordinationHandoverCall
	AssumeOnlys           []AssumeOnlyCall
	AssumeAndDrops        []AssumeAndDropCall
	DropTrackings         []DropTrackingCall
	CdmReadyRequests      []CdmReadyRequestCall
	GenerateSquawks       []GenerateSquawkCall
	ClearedAltitudes      []ClearedAltitudeCall
	Broadcasts            []euroscope.OutgoingMessage
	CreateFPLCalls        []CreateFPLCall
}

func (m *MockEuroscopeHub) GetServer() shared.Server { return m.server }

func (m *MockEuroscopeHub) SetServer(server shared.Server) { m.server = server }

func (m *MockEuroscopeHub) HasActiveClientForAirport(airport string) bool {
	if m.HasActiveClientForAirportFn != nil {
		return m.HasActiveClientForAirportFn(airport)
	}
	return true // default: assume ES client is present so existing tests are not affected
}

func (m *MockEuroscopeHub) GetMasterCallsign(session int32) string {
	if m.GetMasterCallsignFn != nil {
		return m.GetMasterCallsignFn(session)
	}
	return ""
}

func (m *MockEuroscopeHub) GetRunwayMismatchStatus(session int32, cid string) (bool, bool) {
	return false, false
}

func (m *MockEuroscopeHub) Broadcast(session int32, message euroscope.OutgoingMessage) {
	m.Broadcasts = append(m.Broadcasts, message)
}

func (m *MockEuroscopeHub) Send(session int32, cid string, message euroscope.OutgoingMessage) {}

func (m *MockEuroscopeHub) SendCdmReadyRequest(session int32, cid string, callsign string) {
	m.CdmReadyRequests = append(m.CdmReadyRequests, CdmReadyRequestCall{Session: session, Cid: cid, Callsign: callsign})
}

func (m *MockEuroscopeHub) SendGenerateSquawk(session int32, cid string, callsign string) {
	m.GenerateSquawks = append(m.GenerateSquawks, GenerateSquawkCall{Session: session, Cid: cid, Callsign: callsign})
}

func (m *MockEuroscopeHub) SendGroundState(session int32, cid string, callsign string, state string) {
	m.GroundStates = append(m.GroundStates, GroundStateCall{session, cid, callsign, state})
}

func (m *MockEuroscopeHub) SendClearedFlag(session int32, cid string, callsign string, flag bool) {
	m.ClearedFlags = append(m.ClearedFlags, ClearedFlagCall{session, cid, callsign, flag})
}

func (m *MockEuroscopeHub) SendStand(session int32, cid string, callsign string, stand string) {}

func (m *MockEuroscopeHub) SendRoute(session int32, cid string, callsign string, route string) {}

func (m *MockEuroscopeHub) SendRemarks(session int32, cid string, callsign string, remarks string) {}

func (m *MockEuroscopeHub) SendSid(session int32, cid string, callsign string, sid string) {}

func (m *MockEuroscopeHub) SendAssignedSquawk(session int32, cid string, callsign string, squawk string) {
}

func (m *MockEuroscopeHub) SendRunway(session int32, cid string, callsign string, runway string) {}

func (m *MockEuroscopeHub) SendClearedAltitude(session int32, cid string, callsign string, altitude int32) {
	m.ClearedAltitudes = append(m.ClearedAltitudes, ClearedAltitudeCall{session, cid, callsign, altitude})
}

func (m *MockEuroscopeHub) SendHeading(session int32, cid string, callsign string, heading int32) {}

func (m *MockEuroscopeHub) SendCoordinationHandover(session int32, cid string, callsign string, targetCallsign string) {
	m.CoordinationHandovers = append(m.CoordinationHandovers, CoordinationHandoverCall{session, cid, callsign, targetCallsign})
}

func (m *MockEuroscopeHub) SendAssumeOnly(session int32, cid string, callsign string) {
	m.AssumeOnlys = append(m.AssumeOnlys, AssumeOnlyCall{Session: session, Cid: cid, Callsign: callsign})
}

func (m *MockEuroscopeHub) SendAssumeAndDrop(session int32, cid string, callsign string) {
	m.AssumeAndDrops = append(m.AssumeAndDrops, AssumeAndDropCall{Session: session, Cid: cid, Callsign: callsign})
}

func (m *MockEuroscopeHub) SendDropTracking(session int32, cid string, callsign string) {
	m.DropTrackings = append(m.DropTrackings, DropTrackingCall{Session: session, Cid: cid, Callsign: callsign})
}

// CreateFPLCall records arguments to SendCreateFPL.
type CreateFPLCall struct {
	Session int32
	Cid     string
	Event   euroscope.CreateFPLEvent
}

func (m *MockEuroscopeHub) SendCreateFPL(session int32, cid string, event euroscope.CreateFPLEvent) {
	m.CreateFPLCalls = append(m.CreateFPLCalls, CreateFPLCall{session, cid, event})
}

func (m *MockEuroscopeHub) SendPdcStateChange(session int32, callsign, state, remarks string) {}
