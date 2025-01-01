package main

import "time"

type EventType string

const HeartbeatEventPayload string = "heartbeat"

const (
	Message    EventType = "message"
	LogMessage EventType = "log_message"
	Heartbeat  EventType = "heartbeat"

	CloseConnection   EventType = "close_connection"
	InitialConnection EventType = "initial_connection"

	GoAround                   EventType = "go_around"
	AirportConfigurationChange EventType = "airport_configuration_change"
	RunWayConfiguration        EventType = "run_way_configuration"
	AtisUpdate                 EventType = "atis_update"

	CoordinationRequestInit   EventType = "coordination_request_init"
	CoordinationRequestAccept EventType = "coordination_request_accept"
	CoordinationRequestReject EventType = "coordination_request_reject"

	PositionOnline  EventType = "position_online"
	PositionOffline EventType = "position_offline"

	// TODO: Strip Management Requests and such

	StripUpdate                EventType = "strip_update"
	StripAssumeRequestInit     EventType = "strip_assume_request"
	StripAssumeRequestReject   EventType = "strip_assume_request_reject"
	StripTransferRequestInit   EventType = "strip_transfer_request"
	StripTransferRequestReject EventType = "strip_transfer_request_reject"
	StripMoveRequest           EventType = "strip_move_request"
)

type Event struct {
	Type      EventType
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
		TimeStamp: time.Now(),
		Payload:   &HeartBeatEventPayload{Payload: content},
	}
}

// This event is from the frontend to the backend
type InitiateConnectionEvent struct {
	CID string
}

type InitialConnectionEvent struct {
	Strips                []Strip
	Positions             []Position
	AirportConfigurations []AirportConfiguration
}

func NewInitialConnectionEvent(strips []Strip, positions []Position, airportConfigurations []AirportConfiguration) *Event {
	return &Event{
		Type:      InitialConnection,
		Source:    "FlightStrips",
		TimeStamp: time.Now(),
		Payload: &InitialConnectionEvent{
			Strips:                strips,
			Positions:             positions,
			AirportConfigurations: airportConfigurations,
		},
	}
}
