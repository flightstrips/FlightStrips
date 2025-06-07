package main

import (
	"time"
)

type EventType string

const HeartbeatEventPayload string = "heartbeat"

const (
	// For each event type we need to define the following:
	// Whether the event is sent to all other FrontEnd Clients
	// Whether the event is sent to a specific FrontEnd Client
	// Whether the event is sent to a specific Euroscope Client
	Authentication EventType = "token"

	// Euroscope Specific Events
	EuroscopeLogin                EventType = "login"
	EuroscopeControllerOnline     EventType = "controller_online"
	EuroscopeControllerOffline    EventType = "controller_offline"
	EuroscopeSync                 EventType = "sync"
	EuroscopeAssignedSquawk       EventType = "assigned_squawk"
	EuroscopeSquawk               EventType = "squawk"
	EuroscopeRequestedAltitude    EventType = "requested_altitude"
	EuroscopeClearedAltitude      EventType = "cleared_altitude"
	EuroscopeCommunicationType    EventType = "communication_type"
	EuroscopeGroundState          EventType = "ground_state"
	EuroscopeClearedFlag          EventType = "cleared_flag"
	EuroscopePositionUpdate       EventType = "aircraft_position_update"
	EuroscopeSetHeading           EventType = "heading"
	EuroscopeAircraftDisconnected EventType = "aircraft_disconnect"
	EuroscopeStand                EventType = "stand"
	EuroscopeStripUpdate          EventType = "strip_update"
	EuroscopeRunway               EventType = "runway"
	EuroscopeSessionInfo          EventType = "session_info"
	EuroscopeGenerateSquawk       EventType = "generate_squawk"
	EuroscopeRoute                EventType = "route"
	EuroscopeRemarks              EventType = "remarks"
	EuroscopeSID                  EventType = "sid"
	EuroscopeAircraftRunway       EventType = "aircraft_runway"

	// GoAround - Sent to all FrontEnd Clients
	// AirportConfigurationChange - Sent to all FrontEnd Clients
	// RunWayConfiguration - Sent to all FrontEnd Clients
	// AtisUpdate - Sent to all FrontEnd Clients
	GoAround                   EventType = "go_around"
	AirportConfigurationChange EventType = "airport_configuration_change"
	RunWayConfiguration        EventType = "run_way_configuration"
	AtisUpdate                 EventType = "atis_update"

	// PositionOnline - Sent to all FrontEnd Clients
	// PositionOffline - Sent to all FrontEnd Clients
	PositionOnline  EventType = "position_online"
	PositionOffline EventType = "position_offline"

	// StripUpdate - Sent to all FrontEnd Clients && One Euroscope Client (The one who made the change)
	// StripTransferRequestInit - Sent to a specific FrontEnd Client
	// StripTransferRequestReject - Sent to a specific FrontEnd Client
	// StripMoveRequest - Sent to a specific FrontEnd Client
	StripUpdate                EventType = "strip_update"
	StripUpdateCleared         EventType = "strip_update_cleared"
	StripUpdateBay             EventType = "strip_update_bay"
	StripUpdateBayPosition     EventType = "strip_update_bay_position"
	StripUpdateRunwayChange    EventType = "strip_update_runway_change"
	StripUpdateDeparture       EventType = "strip_update_departure"
	StripTransferRequestInit   EventType = "strip_transfer_request"
	StripTransferRequestReject EventType = "strip_transfer_request_reject"
	StripMoveRequest           EventType = "strip_move_request"

	FrontendInitial           EventType = "initial"
	FrontendStripUpdate       EventType = "strip_update"
	FrontendControllerOnline  EventType = "controller_online"
	FrontendControllerOffline EventType = "controller_offline"
	FrontendAssignedSquawk    EventType = "assigned_squawk"
	FrontendSquawk            EventType = "squawk"
	FrontendRequestedAltitude EventType = "requested_altitude"
	FrontendClearedAltitude   EventType = "cleared_altitude"
	FrontendBay               EventType = "bay"
)

type AuthenticationEvent struct {
	Type  EventType
	Token string
}

type Event struct {
	Type      EventType
	Airport   string
	Source    string
	Cid       string
	TimeStamp time.Time
	Payload   interface{}
}

// TODO: Work out if this would be ever used

type GoAroundEventPayload struct {
	ControllerID string
}

type PositionOnlinePayload struct {
	Position string
}

type PositionOfflinePayload struct {
	Position string
}

type StripUpdatePayload struct{}

type StripUpdateClearedPayload struct{}

type StripUpdateBayPayload struct{}

type StripUpdateBayPositionPayload struct{}

type StripUpdateRunwayChangePayload struct{}

type StripUpdateDeparturePayload struct{}

type StripTransferRequestInitPayload struct{}

type StripTransferRequestRejectPayload struct{}

type StripMoveRequestPayload struct{}
