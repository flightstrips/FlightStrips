package frontend

import (
	"FlightStrips/pkg/events"
	"encoding/json"
	"time"
)

type EventType string

const HeartbeatEventPayload string = "heartbeat"

const (
	GoAround                   EventType = "go_around"
	AirportConfigurationChange EventType = "airport_configuration_change"
	RunWayConfiguration        EventType = "run_way_configuration"
	AtisUpdate                 EventType = "atis_update"

	Token EventType = "token"

	Initial            EventType = "initial"
	StripUpdate        EventType = "strip_update"
	ControllerOnline   EventType = "controller_online"
	ControllerOffline  EventType = "controller_offline"
	AssignedSquawk     EventType = "assigned_squawk"
	Squawk             EventType = "squawk"
	RequestedAltitude  EventType = "requested_altitude"
	ClearedAltitude    EventType = "cleared_altitude"
	Bay                EventType = "bay"
	BulkBay            EventType = "bulk_bay"
	Disconnect         EventType = "disconnect"
	AircraftDisconnect EventType = "aircraft_disconnect"
	Stand              EventType = "stand"
	SetHeading         EventType = "heading"
	CommunicationType  EventType = "communication_type"

	CoordinationTransferRequestType    EventType = "coordination_transfer_request"
	CoordinationAssumeRequestType      EventType = "coordination_assume_request"
	CoordinationRejectRequestType      EventType = "coordination_reject_request"
	CoordinationFreeRequestType        EventType = "coordination_free_request"
	CoordinationCancelTransferRequest  EventType = "coordination_cancel_transfer_request"
	CoordinationForceAssumeRequestType EventType = "coordination_force_assume_request"
	CoordinationTagRequestType         EventType = "coordination_tag_request"
	CoordinationAcceptTagRequestType   EventType = "coordination_accept_tag_request"

	Move                              EventType = "move"
	GenerateSquawk                    EventType = "generate_squawk"
	UpdateStripData                   EventType = "update_strip_data"
	AcknowledgeUnexpectedChange       EventType = "acknowledge_unexpected_change"
	CoordinationAssumeBroadcastType   EventType = "coordination_assume_broadcast"
	CoordinationRejectBroadcastType   EventType = "coordination_reject_broadcast"
	CoordinationTransferBroadcastType EventType = "coordination_transfer_broadcast"
	CoordinationFreeBroadcastType     EventType = "coordination_free_broadcast"
	CoordinationTagRequestBroadcastType EventType = "coordination_tag_request_broadcast"

	OwnersUpdate EventType = "owners_update"

	UpdateOrder EventType = "update_order"

	LayoutUpdate = "layout_update"

	Broadcast       EventType = "broadcast"
	SendMessage     EventType = "send_message"
	MessageReceived EventType = "message_received"

	CdmWait  EventType = "cdm_wait"
	CdmData  EventType = "cdm_data"
	CdmReady EventType = "cdm_ready"

	ReleasePoint EventType = "release_point"

	Marked EventType = "marked"

	RunwayClearance EventType = "runway_clearance"

	PdcManualStateChange EventType = "pdc_manual_state_change"
	PdcStateChange       EventType = "pdc_state_change"
	IssuePdcClearance    EventType = "issue_pdc_clearance"
	RevertToVoice        EventType = "revert_to_voice"

	// Tactical strip events (broadcast to frontend)
	TacticalStripCreated EventType = "tactical_strip_created"
	TacticalStripDeleted EventType = "tactical_strip_deleted"
	TacticalStripUpdated EventType = "tactical_strip_updated"
	TacticalStripMoved   EventType = "tactical_strip_moved"

	// Sent to the originating client when a frontend action is rejected by the backend
	ActionRejected EventType = "action_rejected"

	// AvailableSids is broadcast to all frontend clients when the master EuroScope client
	// sends a sync event containing SIDs, and on new frontend connects.
	AvailableSids EventType = "available_sids"
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
	ClearedAltitude     int32    `json:"cleared_altitude"`
	RequestedAltitude   int32    `json:"requested_altitude"`
	Heading             int32    `json:"heading"`
	AircraftType        string   `json:"aircraft_type"`
	AircraftCategory    string   `json:"aircraft_category"`
	Stand               string   `json:"stand"`
	Capabilities        string   `json:"capabilities"`
	CommunicationType   string   `json:"communication_type"`
	Eobt                string   `json:"eobt"`
	Eldt                string   `json:"eldt"`
	Bay                 string   `json:"bay"`
	ReleasePoint        string   `json:"release_point"`
	Version             int32    `json:"version"`
	Sequence            int32    `json:"sequence"`
	NextControllers     []string `json:"next_controllers"`
	PreviousControllers []string `json:"previous_controllers"`
	Owner               string   `json:"owner"`
	Tobt                string   `json:"tobt"`
	Tsat                string   `json:"tsat"`
	Ctot                string   `json:"ctot"`
	PdcState            string   `json:"pdc_state"`
	Marked                 bool     `json:"marked"`
	Registration           string   `json:"registration"`
	TrackingController     string   `json:"tracking_controller"`
	RunwayCleared          bool     `json:"runway_cleared"`
	UnexpectedChangeFields  []string `json:"unexpected_change_fields"`
	ControllerModifiedFields []string `json:"controller_modified_fields"`
}

type Controller struct {
	Callsign   string `json:"callsign"`
	Position   string `json:"position"`
	Identifier string `json:"identifier"`
	Section    string `json:"section"`
}

type SyncCoordination struct {
	Callsign     string `json:"callsign"`
	From         string `json:"from"`
	To           string `json:"to"`
	IsTagRequest bool   `json:"is_tag_request"`
}

type InitialEvent struct {
	Contsollers    []Controller              `json:"controllers"`
	Strips         []Strip                   `json:"strips"`
	TacticalStrips []TacticalStripPayload    `json:"tactical_strips"`
	Me             Controller                `json:"me"`
	Layout         string                    `json:"layout"`
	Airport        string                    `json:"airport"`
	Callsign       string                    `json:"callsign"`
	RunwaySetup    RunwayConfiguration       `json:"runway_setup"`
	Coordinations  []SyncCoordination        `json:"coordinations"`
	Messages       []MessageReceivedEvent    `json:"messages"`
	AvailableSids  []string                  `json:"available_sids"`
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
	Altitude int32  `json:"altitude"`
}

func (r RequestedAltitudeEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r RequestedAltitudeEvent) GetType() EventType {
	return RequestedAltitude
}

type ClearedAltitudeEvent struct {
	Callsign string `json:"callsign"`
	Altitude int32  `json:"altitude"`
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

// BulkBayEntry holds the sequence update for a single flight strip in a BulkBayEvent.
type BulkBayEntry struct {
	Callsign string `json:"callsign"`
	Sequence int32  `json:"sequence"`
}

// BulkBayEvent broadcasts a batch of sequence updates for strips in a single bay atomically,
// preventing temporary ordering inconsistencies on the frontend when many strips are recalculated.
type BulkBayEvent struct {
	Bay    string         `json:"bay"`
	Strips []BulkBayEntry `json:"strips"`
}

func (b BulkBayEvent) Marshal() ([]byte, error) {
	return marshall(b)
}

func (b BulkBayEvent) GetType() EventType {
	return BulkBay
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
	Heading  int32  `json:"heading"`
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

type AcknowledgeUnexpectedChangeEvent struct {
	Type      EventType `json:"type"`
	Callsign  string    `json:"callsign"`
	FieldName string    `json:"field_name"`
}

type UpdateStripDataEvent struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Sid      *string   `json:"sid"`
	Eobt     *string   `json:"eobt"`
	Route    *string   `json:"route"`
	Heading  *int32    `json:"heading"`
	Altitude *int32    `json:"altitude"`
	Stand    *string   `json:"stand"`
	Runway   *string   `json:"runway,omitempty"`
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

// ---------- CANCEL TRANSFER ----------

type CoordinationCancelTransferRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationForceAssumeRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

// ---------- TAG REQUEST ----------

type CoordinationTagRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationAcceptTagRequestEvent struct {
	Type     string `json:"type"`
	Callsign string `json:"callsign"`
}

type CoordinationTagRequestBroadcastEvent struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Callsign string `json:"callsign"`
}

func (c CoordinationTagRequestBroadcastEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CoordinationTagRequestBroadcastEvent) GetType() EventType {
	return CoordinationTagRequestBroadcastType
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
	Owner          string   `json:"owner"`
	NextOwners     []string `json:"next_owners"`
	PreviousOwners []string `json:"previous_owners"`
}

func (o OwnersUpdateEvent) Marshal() ([]byte, error) {
	return marshall(o)
}

func (o OwnersUpdateEvent) GetType() EventType {
	return OwnersUpdate
}

type UpdateOrderEvent struct {
	Callsign    string    `json:"callsign"`
	InsertAfter *StripRef `json:"insert_after"`
}

func (o UpdateOrderEvent) Marshal() ([]byte, error) {
	return marshall(o)
}

func (o UpdateOrderEvent) GetType() EventType {
	return UpdateOrder
}

type LayoutUpdateEvent struct {
	Layout string `json:"layout"`
}

func (l LayoutUpdateEvent) Marshal() ([]byte, error) {
	return marshall(l)
}

func (l LayoutUpdateEvent) GetType() EventType {
	return LayoutUpdate
}

type BroadcastEvent struct {
	Message string `json:"message"`
	From    string `json:"from"`
}

func (l BroadcastEvent) Marshal() ([]byte, error) {
	return marshall(l)
}

func (l BroadcastEvent) GetType() EventType {
	return Broadcast
}

type SendMessageEvent struct {
	Text       string   `json:"text"`
	Recipients []string `json:"recipients"`
}

type MessageReceivedEvent struct {
	ID          int64    `json:"id"`
	Sender      string   `json:"sender"`
	Text        string   `json:"text"`
	IsBroadcast bool     `json:"is_broadcast"`
	Recipients  []string `json:"recipients"`
}

func (m MessageReceivedEvent) Marshal() ([]byte, error) {
	return marshall(m)
}

func (m MessageReceivedEvent) GetType() EventType {
	return MessageReceived
}

type CdmWaitEvent struct {
	Callsign string `json:"callsign"`
}

func (c CdmWaitEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CdmWaitEvent) GetType() EventType {
	return CdmWait
}

type CdmDataEvent struct {
	Callsign string `json:"callsign"`
	Eobt     string `json:"eobt"`
	Tobt     string `json:"tobt"`
	Tsat     string `json:"tsat"`
	Ctot     string `json:"ctot"`
}

func (c CdmDataEvent) Marshal() ([]byte, error) {
	return marshall(c)
}

func (c CdmDataEvent) GetType() EventType {
	return CdmData
}

type CdmReadyEvent struct {
	Callsign string `json:"callsign"`
}

type ReleasePointEvent struct {
	Callsign     string `json:"callsign"`
	ReleasePoint string `json:"release_point"`
}

func (r ReleasePointEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r ReleasePointEvent) GetType() EventType {
	return ReleasePoint
}

type MarkedEvent struct {
	Callsign string `json:"callsign"`
	Marked   bool   `json:"marked"`
}

func (m MarkedEvent) Marshal() ([]byte, error) {
	return marshall(m)
}

func (m MarkedEvent) GetType() EventType {
	return Marked
}

type RunwayClearanceEvent struct {
	Callsign string `json:"callsign"`
}

func (r RunwayClearanceEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r RunwayClearanceEvent) GetType() EventType {
	return RunwayClearance
}

// PDC Events

type PdcStateChangeEvent struct {
	Callsign string `json:"callsign"`
	State    string `json:"state"`
}

func (p PdcStateChangeEvent) Marshal() ([]byte, error) {
	return marshall(p)
}

func (p PdcStateChangeEvent) GetType() EventType {
	return PdcStateChange
}

// PDC Incoming Events

type IssuePdcClearanceRequest struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	Remarks  string    `json:"remarks"`
}

type RevertToVoiceRequest struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
}

type PdcManualStateChangeRequest struct {
	Type     EventType `json:"type"`
	Callsign string    `json:"callsign"`
	State    string    `json:"state"`
}

type RunwayConfigurationEvent struct {
	RunwaySetup RunwayConfiguration `json:"runway_setup"`
}

func (r RunwayConfigurationEvent) Marshal() ([]byte, error) {
	return marshall(r)
}

func (r RunwayConfigurationEvent) GetType() EventType {
	return RunWayConfiguration
}

// ---------- StripRef (shared type for move actions) ----------

// StripRef identifies a single strip of any type, used as a neighbour reference in move actions.
// Exactly one of Callsign or ID must be set depending on Kind.
type StripRef struct {
	Kind     string  `json:"kind"`               // "flight" | "tactical"
	Callsign *string `json:"callsign,omitempty"` // set when Kind = "flight"
	ID       *int64  `json:"id,omitempty"`       // set when Kind = "tactical"
}

// ---------- Tactical strip payload model ----------

type TacticalStripPayload struct {
	ID          int64      `json:"id"`
	SessionID   int32      `json:"session_id"`
	Type        string     `json:"type"`
	Bay         string     `json:"bay"`
	Label       string     `json:"label"`
	Aircraft    string     `json:"aircraft"`
	ProducedBy  string     `json:"produced_by"`
	Sequence    int32      `json:"sequence"`
	TimerStart  *time.Time `json:"timer_start,omitempty"`
	Confirmed   bool       `json:"confirmed"`
	ConfirmedBy string     `json:"confirmed_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ---------- Tactical strip outgoing events ----------

type TacticalStripCreatedEvent struct {
	Strip TacticalStripPayload `json:"strip"`
}

func (e TacticalStripCreatedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e TacticalStripCreatedEvent) GetType() EventType       { return TacticalStripCreated }

type TacticalStripDeletedEvent struct {
	ID        int64  `json:"id"`
	SessionID int32  `json:"session_id"`
	Bay       string `json:"bay"`
}

func (e TacticalStripDeletedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e TacticalStripDeletedEvent) GetType() EventType       { return TacticalStripDeleted }

type TacticalStripUpdatedEvent struct {
	Strip TacticalStripPayload `json:"strip"`
}

func (e TacticalStripUpdatedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e TacticalStripUpdatedEvent) GetType() EventType       { return TacticalStripUpdated }

type TacticalStripMovedEvent struct {
	ID        int64  `json:"id"`
	SessionID int32  `json:"session_id"`
	Bay       string `json:"bay"`
	Sequence  int32  `json:"sequence"`
}

func (e TacticalStripMovedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e TacticalStripMovedEvent) GetType() EventType       { return TacticalStripMoved }

type AtisUpdateEvent struct {
	Metar string `json:"metar"`
}

func (a AtisUpdateEvent) Marshal() ([]byte, error) { return marshall(a) }
func (a AtisUpdateEvent) GetType() EventType       { return AtisUpdate }

type ActionRejectedEvent struct {
	Action string `json:"action"` // the action type string that was rejected
	Reason string `json:"reason"` // human-readable reason
}

func (e ActionRejectedEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e ActionRejectedEvent) GetType() EventType       { return ActionRejected }

type AvailableSidsEvent struct {
	Sids []string `json:"sids"`
}

func (e AvailableSidsEvent) Marshal() ([]byte, error) { return marshall(e) }
func (e AvailableSidsEvent) GetType() EventType       { return AvailableSids }
