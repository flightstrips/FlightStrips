package frontend

import (
	"FlightStrips/pkg/events"
	"encoding/json"
)

type EventType string

const HeartbeatEventPayload string = "heartbeat"

const (
	GoAround                   EventType = "go_around"
	AirportConfigurationChange EventType = "airport_configuration_change"
	RunWayConfiguration        EventType = "run_way_configuration"
	AtisUpdate                 EventType = "atis_update"

	Initial            EventType = "initial"
	StripUpdate        EventType = "strip_update"
	ControllerOnline   EventType = "controller_online"
	ControllerOffline  EventType = "controller_offline"
	AssignedSquawk     EventType = "assigned_squawk"
	Squawk             EventType = "squawk"
	RequestedAltitude  EventType = "requested_altitude"
	ClearedAltitude    EventType = "cleared_altitude"
	Bay                EventType = "bay"
	Disconnect         EventType = "disconnect"
	AircraftDisconnect EventType = "aircraft_disconnect"
	Stand              EventType = "stand"
	SetHeading         EventType = "heading"
	CommunicationType  EventType = "communication_type"

	CoordinationTransferRequestType EventType = "coordination_transfer_request"
	CoordinationAssumeRequestType   EventType = "coordination_assume_request"
	CoordinationRejectRequestType   EventType = "coordination_reject_request"
	CoordinationFreeRequestType     EventType = "coordination_free_request"

	Move                              EventType = "move"
	GenerateSquawk                    EventType = "generate_squawk"
	UpdateStripData                   EventType = "update_strip_data"
	CoordinationAssumeBroadcastType   EventType = "coordination_assume_broadcast"
	CoordinationRejectBroadcastType   EventType = "coordination_reject_broadcast"
	CoordinationTransferBroadcastType EventType = "coordination_transfer_broadcast"
	CoordinationFreeBroadcastType     EventType = "coordination_free_broadcast"

	OwnersUpdate EventType = "owners_update"
)

type OutgoingMessage interface {
	events.OutgoingMessage
	GetType() EventType
}

type RunwayConfiguration struct {
	Departure []string `json:"departure"`
	Arrival   []string `json:"arrival"`
}

type Strip struct {
	Callsign            string   `json:"callsign"`
	Origin              string   `json:"origin"`
	Destination         string   `json:"destination"`
	Alternate           string   `json:"alternate"`
	Route               string   `json:"route"`
	Remarks             string   `json:"remarks"`
	Runway              string   `json:"runway"`
	Squawk              string   `json:"squawk"`
	AssignedSquawk      string   `json:"assigned_squawk"`
	Sid                 string   `json:"sid"`
	ClearedAltitude     int      `json:"cleared_altitude"`
	RequestedAltitude   int      `json:"requested_altitude"`
	Heading             int      `json:"heading"`
	AircraftType        string   `json:"aircraft_type"`
	AircraftCategory    string   `json:"aircraft_category"`
	Stand               string   `json:"stand"`
	Capabilities        string   `json:"capabilities"`
	CommunicationType   string   `json:"communication_type"`
	Eobt                string   `json:"eobt"`
	Eldt                string   `json:"eldt"`
	Bay                 string   `json:"bay"`
	ReleasePoint        string   `json:"release_point"`
	Version             int      `json:"version"`
	Sequence            int      `json:"sequence"`
	NextControllers     []string `json:"next_controllers"`
	PreviousControllers []string `json:"previous_controllers"`
	Owner               string   `json:"owner"`
}

type Controller struct {
	Callsign   string `json:"callsign"`
	Position   string `json:"position"`
	Identifier string `json:"identifier"`
}

type InitialEvent struct {
	Contsollers []Controller        `json:"controllers"`
	Strips      []Strip             `json:"strips"`
	Me          Controller          `json:"me"`
	Airport     string              `json:"airport"`
	Callsign    string              `json:"callsign"`
	RunwaySetup RunwayConfiguration `json:"runway_setup"`
}

func (i InitialEvent) Marshal() ([]byte, error) {
	return marshall(i)
}

func (i InitialEvent) GetType() EventType {
	return Initial
}

type StripUpdateEvent struct {
	Strip
}

func (s StripUpdateEvent) Marshal() ([]byte, error) {
	return marshall(s)
}

func (s StripUpdateEvent) GetType() EventType {
	return StripUpdate
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

type ControllerOnlineEvent struct {
	Controller
}

func (c ControllerOnlineEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c ControllerOnlineEvent) GetType() EventType {
	return ControllerOnline
}

type ControllerOfflineEvent struct {
	Controller
}

func (c ControllerOfflineEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c ControllerOfflineEvent) GetType() EventType {
	return ControllerOffline
}

type AssignedSquawkEvent struct {
	Callsign string `json:"callsign"`
	Squawk   string `json:"squawk"`
}

func (a AssignedSquawkEvent) Marshal() ([]byte, error) {
	return marshall(a)
}

func (a AssignedSquawkEvent) GetType() EventType {
	return AssignedSquawk
}

type SquawkEvent struct {
	Callsign string `json:"callsign"`
	Squawk   string `json:"squawk"`
}

func (s SquawkEvent) Marshal() ([]byte, error) {
	return marshall(s)
}

func (s SquawkEvent) GetType() EventType {
	return Squawk
}

type RequestedAltitudeEvent struct {
	Callsign string `json:"callsign"`
	Altitude int    `json:"altitude"`
}

func (r RequestedAltitudeEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r RequestedAltitudeEvent) GetType() EventType {
	return RequestedAltitude
}

type ClearedAltitudeEvent struct {
	Callsign string `json:"callsign"`
	Altitude int    `json:"altitude"`
}

func (r ClearedAltitudeEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r ClearedAltitudeEvent) GetType() EventType {
	return ClearedAltitude
}

type BayEvent struct {
	Callsign string `json:"callsign"`
	Bay      string `json:"bay"`
	Sequence int32  `json:"sequence"`
}

func (b BayEvent) Marshal() ([]byte, error) {
	return marshall(b)
}

func (b BayEvent) GetType() EventType {
	return Bay
}

type DisconnectEvent struct{}

func (d DisconnectEvent) Marshal() ([]byte, error) {
	return marshall(d)
}

func (d DisconnectEvent) GetType() EventType {
	return Disconnect
}

type AircraftDisconnectEvent struct {
	Callsign string `json:"callsign"`
}

func (a AircraftDisconnectEvent) Marshal() ([]byte, error) {
	return marshall(a)
}

func (a AircraftDisconnectEvent) GetType() EventType {
	return AircraftDisconnect
}

type StandEvent struct {
	Callsign string `json:"callsign"`
	Stand    string `json:"stand"`
}

func (s StandEvent) Marshal() ([]byte, error) {
	return marshall(s)
}

func (s StandEvent) GetType() EventType {
	return Stand
}

type SetHeadingEvent struct {
	Callsign string `json:"callsign"`
	Heading  int    `json:"heading"`
}

func (s SetHeadingEvent) Marshal() ([]byte, error) {
	return marshall(s)
}

func (s SetHeadingEvent) GetType() EventType {
	return SetHeading
}

type CommunicationTypeEvent struct {
	Callsign          string `json:"callsign"`
	CommunicationType string `json:"communication_type"`
}

func (c CommunicationTypeEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CommunicationTypeEvent) GetType() EventType {
	return CommunicationType
}

type MoveEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Bay      string    `json:"bay"`
}

type GenerateSquawkRequest struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type UpdateStripDataEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Sid      *string   `json:"sid"`
	Eobt     *string   `json:"eobt"`
	Route    *string   `json:"route"`
	Heading  *int      `json:"heading"`
	Altitude *int      `json:"altitude"`
	Stand    *string   `json:"stand"`
}

// ---------- TRANSFER ----------

type CoordinationTransferRequestEvent struct {
	Type     string `json:"type"`
	To       string `json:"to"`
	Callsign string `json:"callsign"`
}

type CoordinationTransferBroadcastEvent struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Callsign string `json:"callsign"`
}

func (c CoordinationTransferBroadcastEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CoordinationTransferBroadcastEvent) GetType() EventType {
	return CoordinationTransferBroadcastType
}

// ---------- ASSUME ----------

type CoordinationAssumeRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationAssumeBroadcastEvent struct {
	Position string `json:"position"`
	Callsign string `json:"callsign"`
}

func (c CoordinationAssumeBroadcastEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CoordinationAssumeBroadcastEvent) GetType() EventType {
	return CoordinationAssumeBroadcastType
}

// ---------- REJECT ----------

type CoordinationRejectRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationRejectBroadcastEvent struct {
	Position string `json:"position"`
	Callsign string `json:"callsign"`
}

func (c CoordinationRejectBroadcastEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CoordinationRejectBroadcastEvent) GetType() EventType {
	return CoordinationRejectBroadcastType
}

// ---------- FREE ------------

type CoordinationFreeRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationFreeBroadcastEvent struct {
	Callsign string `json:"callsign"`
}

func (c CoordinationFreeBroadcastEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CoordinationFreeBroadcastEvent) GetType() EventType {
	return CoordinationFreeBroadcastType
}

type OwnersUpdateEvent struct {
	Callsign       string   `json:"callsign"`
	NextOwners     []string `json:"next_owners"`
	PreviousOwners []string `json:"previous_owners"`
}

func (o OwnersUpdateEvent) Marshal() ([]byte, error) {
	return json.Marshal(o)
}

func (o OwnersUpdateEvent) GetType() EventType {
	return OwnersUpdate
}
