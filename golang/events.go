package main

import (
	"FlightStrips/data"
	"time"
)

type EventType string

const HeartbeatEventPayload string = "heartbeat"

const (
	// For each event type we need to define the following:
	// Whether the event is sent to all other FrontEnd Clients
	// Whether the event is sent to a specific FrontEnd Client
	// Whether the event is sent to a specific Euroscope Client

	Message    EventType = "message"
	LogMessage EventType = "log_message"
	Heartbeat  EventType = "heartbeat"

	InitiateConnection EventType = "initiate_connection"
	CloseConnection    EventType = "close_connection"
	InitialConnection  EventType = "initial_connection"

	// GoAround - Sent to all FrontEnd Clients
	// AirportConfigurationChange - Sent to all FrontEnd Clients
	// RunWayConfiguration - Sent to all FrontEnd Clients
	// AtisUpdate - Sent to all FrontEnd Clients
	GoAround                   EventType = "go_around"
	AirportConfigurationChange EventType = "airport_configuration_change"
	RunWayConfiguration        EventType = "run_way_configuration"
	AtisUpdate                 EventType = "atis_update"

	// CoordinationRequestInit - Sent to a specific FrontEnd Client
	// CoordinationRequestAccept - Sent to a specific FrontEnd Client
	// CoordinationRequestReject - Sent to a specific FrontEnd Client
	CoordinationRequestInit   EventType = "coordination_request_init"
	CoordinationRequestAccept EventType = "coordination_request_accept"
	CoordinationRequestReject EventType = "coordination_request_reject"

	// PositionOnline - Sent to all FrontEnd Clients
	// PositionOffline - Sent to all FrontEnd Clients
	PositionOnline  EventType = "position_online"
	PositionOffline EventType = "position_offline"

	// StripUpdate - Sent to all FrontEnd Clients && One Euroscope Client (The one who made the change)
	// StripAssumeRequestInit - Sent to a specific FrontEnd Client
	// StripAssumeRequestReject - Sent to a specific FrontEnd Client
	// StripTransferRequestInit - Sent to a specific FrontEnd Client
	// StripTransferRequestReject - Sent to a specific FrontEnd Client
	// StripMoveRequest - Sent to a specific FrontEnd Client
	StripUpdate                EventType = "strip_update"
	StripAssumeRequestInit     EventType = "strip_assume_request"
	StripAssumeRequestReject   EventType = "strip_assume_request_reject"
	StripTransferRequestInit   EventType = "strip_transfer_request"
	StripTransferRequestReject EventType = "strip_transfer_request_reject"
	StripMoveRequest           EventType = "strip_move_request"
)

type Event struct {
	Type      EventType
	Airport   string
	Source    string
	TimeStamp time.Time
	Payload   interface{}
}

// TODO: Work out if this would be ever used

type MessageEvent struct {
	contents string
}

type HeartBeatEventPayload struct {
	Payload string
}

func NewHeartBeatEvent(content string) *Event {
	return &Event{
		Type:      Heartbeat,
		Source:    "FlightStrips",
		Airport:   "All",
		TimeStamp: time.Now(),
		Payload:   &HeartBeatEventPayload{Payload: content},
	}
}

// InitiateConnectionEvent This event is from the frontend to the backend
type InitiateConnectionEvent struct {
	CID       string
	AuthToken string
}

type InitialConnectionEventResponsePayload struct {
	Strips                []data.Strip
	Controllers           []data.Controller
	AirportConfigurations []AirportConfiguration
}

func NewInitialConnectionEvent(airport string, strips []data.Strip, controllers []data.Controller, airportConfigurations []AirportConfiguration) *Event {
	return &Event{
		Type:      InitialConnection,
		Source:    "FlightStrips",
		Airport:   airport,
		TimeStamp: time.Now(),
		Payload: &InitialConnectionEventResponsePayload{
			Strips:                strips,
			Controllers:           controllers,
			AirportConfigurations: airportConfigurations,
		},
	}
}
