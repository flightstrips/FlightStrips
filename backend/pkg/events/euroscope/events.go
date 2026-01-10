package euroscope

import (
	"FlightStrips/pkg/events"
	"encoding/json"
)

type EventType string

const (
	Authentication       EventType = "token"
	Login                EventType = "login"
	ControllerOnline     EventType = "controller_online"
	ControllerOffline    EventType = "controller_offline"
	Sync                 EventType = "sync"
	AssignedSquawk       EventType = "assigned_squawk"
	Squawk               EventType = "squawk"
	RequestedAltitude    EventType = "requested_altitude"
	ClearedAltitude      EventType = "cleared_altitude"
	CommunicationType    EventType = "communication_type"
	GroundState          EventType = "ground_state"
	ClearedFlag          EventType = "cleared_flag"
	PositionUpdate       EventType = "aircraft_position_update"
	SetHeading           EventType = "heading"
	AircraftDisconnected EventType = "aircraft_disconnect"
	Stand                EventType = "stand"
	StripUpdate          EventType = "strip_update"
	Runway               EventType = "runway"
	AircraftRunway       EventType = "aircraft_runway"
	SessionInfo          EventType = "session_info"
	GenerateSquawk       EventType = "generate_squawk"
	Route                EventType = "route"
	Remarks              EventType = "remarks"
	Sid                  EventType = "sid"
)

const (
	GroundStateUnknown = ""
	GroundStateStartup = "ST-UP"
	GroundStatePush    = "PUSH"
	GroundStateTaxi    = "TAXI"
	GroundStateDepart  = "DEPA"
)

type OutgoingMessage interface {
	events.OutgoingMessage
	GetType() EventType
}

func marshall[T OutgoingMessage](message T) (result []byte, err error) {
	// This is really hacky
	original, err := json.Marshal(message)
	if err != nil {
		return
	}

	var properties map[string]interface{}
	err = json.Unmarshal(original, &properties)
	if err != nil {
		return
	}

	properties["type"] = message.GetType()
	return json.Marshal(properties)
}

type LoginEvent struct {
	Type       EventType `json:"type"`
	Connection string    `json:"connection"`
	Airport    string    `json:"airport"`
	Position   string    `json:"position"`
	Callsign   string    `json:"callsign"`
	Range      int32     `json:"range"`
}

type ControllerOnlineEvent struct {
	Type     EventType `json:"type"`
	Position string    `json:"position"`
	Callsign string    `json:"callsign"`
}

type ControllerOfflineEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type Strip struct {
	Callsign          string `json:"callsign"`
	Origin            string `json:"origin"`
	Destination       string `json:"destination"`
	Alternate         string `json:"alternate"`
	Route             string `json:"route"`
	Remarks           string `json:"remarks"`
	Runway            string `json:"runway"`
	Squawk            string `json:"squawk"`
	AssignedSquawk    string `json:"assigned_squawk"`
	Sid               string `json:"sid"`
	Cleared           bool   `json:"cleared"`
	GroundState       string `json:"ground_state"`
	ClearedAltitude   int32  `json:"cleared_altitude"`
	RequestedAltitude int32  `json:"requested_altitude"`
	Heading           int32  `json:"heading"`
	AircraftType      string `json:"aircraft_type"`
	AircraftCategory  string `json:"aircraft_category"`
	Position          struct {
		Lat      float64 `json:"lat"`
		Lon      float64 `json:"lon"`
		Altitude int32   `json:"altitude"`
	} `json:"position"`
	Stand             string `json:"stand"`
	Capabilities      string `json:"capabilities"`
	CommunicationType string `json:"communication_type"`
	Eobt              string `json:"eobt"`
	Eldt              string `json:"eldt"`
}

type SyncEvent struct {
	Type        EventType `json:"type"`
	Controllers []struct {
		Position string `json:"position"`
		Callsign string `json:"callsign"`
	} `json:"controllers"`
	Strips []Strip `json:"strips"`
}

type AssignedSquawkEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Squawk   string    `json:"squawk"`
}

type SquawkEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Squawk   string    `json:"squawk"`
}

type ClearedAltitudeEvent struct {
	Type     EventType `json:"type"`
	Altitude int32     `json:"altitude"`
	Callsign string    `json:"callsign"`
}

type RequestedAltitudeEvent struct {
	Type     EventType `json:"type"`
	Altitude int32     `json:"altitude"`
	Callsign string    `json:"callsign"`
}

type CommunicationTypeEvent struct {
	Type              EventType `json:"type"`
	Callsign          string    `json:"callsign"`
	CommunicationType string    `json:"communication_type"`
}

type GroundStateEvent struct {
	Type        EventType `json:"type"`
	Callsign    string    `json:"callsign"`
	GroundState string    `json:"ground_state"`
}

type ClearedFlagEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Cleared  bool      `json:"cleared"`
}

type AircraftPositionUpdateEvent struct {
	Type     EventType `json:"type"`
	Altitude int64     `json:"altitude"`
	Callsign string    `json:"callsign"`
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
}

type HeadingEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Heading  int32     `json:"heading"`
}

type AircraftDisconnectEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type StandEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Stand    string    `json:"stand"`
}

type StripUpdateEvent struct {
	Type EventType `json:"type"`
	Strip
}

type RunwayEvent struct {
	Type    EventType `json:"type"`
	Runways []struct {
		Arrival   bool   `json:"arrival"`
		Departure bool   `json:"departure"`
		Name      string `json:"name"`
	} `json:"runways"`
}

type SessionInfoRole string

const (
	SessionInfoMaster SessionInfoRole = "master"
	SessionInfoSlave  SessionInfoRole = "slave"
)

type SessionInfoEvent struct {
	Role SessionInfoRole `json:"role"`
}

func (e SessionInfoEvent) GetType() EventType {
	return SessionInfo
}

func (e SessionInfoEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

type GenerateSquawkEvent struct {
	Callsign string `json:"callsign"`
}

func (e GenerateSquawkEvent) GetType() EventType {
	return GenerateSquawk
}

func (e GenerateSquawkEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e GroundStateEvent) GetType() EventType {
	return GroundState
}

func (e GroundStateEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e ClearedFlagEvent) GetType() EventType {
	return ClearedFlag
}

func (e ClearedFlagEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e AssignedSquawkEvent) GetType() EventType {
	return AssignedSquawk
}

func (e AssignedSquawkEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e RequestedAltitudeEvent) GetType() EventType {
	return RequestedAltitude
}

func (e RequestedAltitudeEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e ClearedAltitudeEvent) GetType() EventType {
	return ClearedAltitude
}

func (e ClearedAltitudeEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CommunicationTypeEvent) GetType() EventType {
	return CommunicationType
}

func (e CommunicationTypeEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e HeadingEvent) GetType() EventType {
	return SetHeading
}

func (e HeadingEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e StandEvent) GetType() EventType {
	return Stand
}

func (e StandEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

type RouteEvent struct {
	Callsign string `json:"callsign"`
	Route    string `json:"route"`
}

type RemarksEvent struct {
	Callsign string `json:"callsign"`
	Remarks  string `json:"remarks"`
}

type SidEvent struct {
	Callsign string `json:"callsign"`
	Sid      string `json:"sid"`
}

type AircraftRunwayEvent struct {
	Callsign string `json:"callsign"`
	Runway   string `json:"runway"`
}

func (e RouteEvent) GetType() EventType {
	return Route
}

func (e RouteEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e RemarksEvent) GetType() EventType {
	return Remarks
}

func (e RemarksEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e SidEvent) GetType() EventType {
	return Sid
}

func (e SidEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e AircraftRunwayEvent) GetType() EventType {
	return AircraftRunway
}

func (e AircraftRunwayEvent) Marshal() ([]byte, error) {
	return marshall(e)
}
