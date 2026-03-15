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
	Session    int32
	Cid        string
	Callsign   string
	GroundState string
}

// MockEuroscopeHub is a configurable mock for shared.EuroscopeHub.
// It records calls for assertion in tests.
type MockEuroscopeHub struct {
	server shared.Server

	HasActiveClientForAirportFn func(airport string) bool

	ClearedFlags  []ClearedFlagCall
	GroundStates  []GroundStateCall
}

func (m *MockEuroscopeHub) GetServer() shared.Server { return m.server }

func (m *MockEuroscopeHub) SetServer(server shared.Server) { m.server = server }

func (m *MockEuroscopeHub) HasActiveClientForAirport(airport string) bool {
	if m.HasActiveClientForAirportFn != nil {
		return m.HasActiveClientForAirportFn(airport)
	}
	return true // default: assume ES client is present so existing tests are not affected
}

func (m *MockEuroscopeHub) Broadcast(session int32, message euroscope.OutgoingMessage) {}

func (m *MockEuroscopeHub) Send(session int32, cid string, message euroscope.OutgoingMessage) {}

func (m *MockEuroscopeHub) SendGenerateSquawk(session int32, cid string, callsign string) {}

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
}

func (m *MockEuroscopeHub) SendHeading(session int32, cid string, callsign string, heading int32) {}

func (m *MockEuroscopeHub) SendCoordinationHandover(session int32, cid string, callsign string, targetCallsign string) {
}

func (m *MockEuroscopeHub) SendAssumeAndDrop(session int32, cid string, callsign string) {}
