package euroscope

import (
	"FlightStrips/pkg/events"
	"FlightStrips/pkg/models"
	"encoding/json"
)

type EventType string

const (
	Authentication            EventType = "token"
	Login                     EventType = "login"
	ControllerOnline          EventType = "controller_online"
	ControllerOffline         EventType = "controller_offline"
	Sync                      EventType = "sync"
	AssignedSquawk            EventType = "assigned_squawk"
	Squawk                    EventType = "squawk"
	RequestedAltitude         EventType = "requested_altitude"
	ClearedAltitude           EventType = "cleared_altitude"
	CommunicationType         EventType = "communication_type"
	GroundState               EventType = "ground_state"
	ClearedFlag               EventType = "cleared_flag"
	PositionUpdate            EventType = "aircraft_position_update"
	SetHeading                EventType = "heading"
	AircraftDisconnected      EventType = "aircraft_disconnect"
	Stand                     EventType = "stand"
	StripUpdate               EventType = "strip_update"
	Runway                    EventType = "runway"
	AircraftRunway            EventType = "aircraft_runway"
	SessionInfo               EventType = "session_info"
	RunwayMismatchAlert       EventType = "runway_mismatch_alert"
	CdmUpdate                 EventType = "cdm_update"
	CdmTobtUpdate             EventType = "cdm_tobt_update"
	CdmDeiceUpdate            EventType = "cdm_deice_update"
	CdmManualCtot             EventType = "cdm_manual_ctot"
	CdmCtotRemove             EventType = "cdm_ctot_remove"
	CdmApproveReqTobt         EventType = "cdm_approve_req_tobt"
	CdmAsrtToggle             EventType = "cdm_asrt_toggle"
	CdmTsacUpdate             EventType = "cdm_tsac_update"
	CdmMasterToggle           EventType = "cdm_master_toggle"
	GenerateSquawk            EventType = "generate_squawk"
	Route                     EventType = "route"
	Remarks                   EventType = "remarks"
	Sid                       EventType = "sid"
	CoordinationHandover      EventType = "coordination_handover"
	TrackingControllerChanged EventType = "tracking_controller_changed"
	CoordinationReceived      EventType = "coordination_received"
	AssumeOnly                EventType = "assume_only"
	AssumeAndDrop             EventType = "assume_and_drop"
	DropTracking              EventType = "drop_tracking"
	BackendSync               EventType = "backend_sync"
	CreateFPL                 EventType = "create_fpl"
	PdcStateChange            EventType = "pdc_state_change"
	IssuePdcClearance         EventType = "issue_pdc_clearance"
	PdcRevertToVoice          EventType = "pdc_revert_to_voice"
)

const (
	GroundStateUnknown = ""
	GroundStateStartup = "ST-UP"
	GroundStatePush    = "PUSH"
	GroundStateTaxi    = "TAXI"
	GroundStateLineup  = "LINEUP"
	GroundStateDepart  = "DEPA"
	GroundStateParked  = "PARK"
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
	Stand              string `json:"stand"`
	Capabilities       string `json:"capabilities"`
	CommunicationType  string `json:"communication_type"`
	Eobt               string `json:"eobt"`
	Eldt               string `json:"eldt"`
	TrackingController string `json:"tracking_controller"`
	EngineType         string `json:"engine_type"`
	HasFP              bool   `json:"has_fp"`
}

type SyncRunway struct {
	Arrival   bool   `json:"arrival"`
	Departure bool   `json:"departure"`
	Name      string `json:"name"`
}

type SyncEvent struct {
	Type        EventType `json:"type"`
	Controllers []struct {
		Position string `json:"position"`
		Callsign string `json:"callsign"`
	} `json:"controllers"`
	Strips  []Strip          `json:"strips"`
	Runways []SyncRunway     `json:"runways"`
	Sids    []models.SidInfo `json:"sids"`
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

type TrackingControllerChangedEvent struct {
	Type               EventType `json:"type"`
	Callsign           string    `json:"callsign"`
	TrackingController string    `json:"tracking_controller"`
}

type CoordinationReceivedEvent struct {
	Type               EventType `json:"type"`
	Callsign           string    `json:"callsign"`
	ControllerCallsign string    `json:"controller_callsign"`
}

type AssumeOnlyEvent struct {
	Callsign string `json:"callsign"`
}

type AssumeAndDropEvent struct {
	Callsign string `json:"callsign"`
}

type DropTrackingEvent struct {
	Callsign string `json:"callsign"`
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
	Type    EventType    `json:"type"`
	Runways []SyncRunway `json:"runways"`
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

type RunwayMismatchAlertEvent struct {
	ExpectedDeparture []string `json:"expected_departure"`
	ExpectedArrival   []string `json:"expected_arrival"`
	CurrentDeparture  []string `json:"current_departure"`
	CurrentArrival    []string `json:"current_arrival"`
}

func (e RunwayMismatchAlertEvent) GetType() EventType {
	return RunwayMismatchAlert
}

func (e RunwayMismatchAlertEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

type GenerateSquawkEvent struct {
	Callsign string `json:"callsign"`
}

type CdmUpdateEvent struct {
	Callsign        string `json:"callsign"`
	Eobt            string `json:"eobt,omitempty"`
	Tobt            string `json:"tobt,omitempty"`
	TobtSetBy       string `json:"tobt_set_by,omitempty"`
	TobtConfirmedBy string `json:"tobt_confirmed_by,omitempty"`
	ReqTobt         string `json:"req_tobt,omitempty"`
	Tsat            string `json:"tsat,omitempty"`
	Ttot            string `json:"ttot,omitempty"`
	Ctot            string `json:"ctot,omitempty"`
	CtotSource      string `json:"ctot_source,omitempty"`
	Asat            string `json:"asat,omitempty"`
	Asrt            string `json:"asrt,omitempty"`
	Tsac            string `json:"tsac,omitempty"`
	Status          string `json:"status,omitempty"`
	EcfmpID         string `json:"ecfmp_id,omitempty"`
	Phase           string `json:"phase,omitempty"`
}

type CdmTobtUpdateEvent struct {
	Callsign string `json:"callsign"`
	Tobt     string `json:"tobt"`
}

type CdmDeiceUpdateEvent struct {
	Callsign  string `json:"callsign"`
	DeiceType string `json:"deice_type"`
}

type CdmManualCtotEvent struct {
	Callsign string `json:"callsign"`
	Ctot     string `json:"ctot"`
}

type CdmCtotRemoveEvent struct {
	Callsign string `json:"callsign"`
}

type CdmApproveReqTobtEvent struct {
	Callsign string `json:"callsign"`
}

type CdmMasterToggleEvent struct {
	Master bool `json:"master"`
}

type BackendSyncCdmData struct {
	Eobt            string `json:"eobt,omitempty"`
	Tobt            string `json:"tobt,omitempty"`
	TobtSetBy       string `json:"tobt_set_by,omitempty"`
	TobtConfirmedBy string `json:"tobt_confirmed_by,omitempty"`
	ReqTobt         string `json:"req_tobt,omitempty"`
	Tsat            string `json:"tsat,omitempty"`
	Ttot            string `json:"ttot,omitempty"`
	Ctot            string `json:"ctot,omitempty"`
	CtotSource      string `json:"ctot_source,omitempty"`
	Asat            string `json:"asat,omitempty"`
	Asrt            string `json:"asrt,omitempty"`
	Tsac            string `json:"tsac,omitempty"`
	Status          string `json:"status,omitempty"`
	EcfmpID         string `json:"ecfmp_id,omitempty"`
	Phase           string `json:"phase,omitempty"`
}

type CdmAsrtToggleEvent struct {
	Callsign string `json:"callsign"`
	Asrt     string `json:"asrt"`
}

type CdmTsacUpdateEvent struct {
	Callsign string `json:"callsign"`
	Tsac     string `json:"tsac"`
}

func (e CdmAsrtToggleEvent) GetType() EventType {
	return CdmAsrtToggle
}

func (e CdmAsrtToggleEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmTsacUpdateEvent) GetType() EventType {
	return CdmTsacUpdate
}

func (e CdmTsacUpdateEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmUpdateEvent) GetType() EventType {
	return CdmUpdate
}

func (e CdmUpdateEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmTobtUpdateEvent) GetType() EventType {
	return CdmTobtUpdate
}

func (e CdmTobtUpdateEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmDeiceUpdateEvent) GetType() EventType {
	return CdmDeiceUpdate
}

func (e CdmDeiceUpdateEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmManualCtotEvent) GetType() EventType {
	return CdmManualCtot
}

func (e CdmManualCtotEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmCtotRemoveEvent) GetType() EventType {
	return CdmCtotRemove
}

func (e CdmCtotRemoveEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmApproveReqTobtEvent) GetType() EventType {
	return CdmApproveReqTobt
}

func (e CdmApproveReqTobtEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CdmMasterToggleEvent) GetType() EventType {
	return CdmMasterToggle
}

func (e CdmMasterToggleEvent) Marshal() ([]byte, error) {
	return marshall(e)
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

type CoordinationHandoverEvent struct {
	Callsign       string `json:"callsign"`
	TargetCallsign string `json:"target_callsign"`
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

func (e TrackingControllerChangedEvent) GetType() EventType {
	return TrackingControllerChanged
}

func (e TrackingControllerChangedEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e AssumeOnlyEvent) GetType() EventType {
	return AssumeOnly
}

func (e AssumeOnlyEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e AssumeAndDropEvent) GetType() EventType {
	return AssumeAndDrop
}

func (e AssumeAndDropEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e DropTrackingEvent) GetType() EventType {
	return DropTracking
}

func (e DropTrackingEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

func (e CoordinationHandoverEvent) GetType() EventType {
	return CoordinationHandover
}

func (e CoordinationHandoverEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

// BackendSyncStrip holds the backend-authoritative state for a single aircraft
// that the connecting EuroScope client must apply locally.
type BackendSyncStrip struct {
	Callsign          string             `json:"callsign"`
	AssignedSquawk    string             `json:"assigned_squawk"`
	Cleared           bool               `json:"cleared"`
	GroundState       string             `json:"ground_state"`
	Stand             string             `json:"stand"`
	Cdm               BackendSyncCdmData `json:"cdm"`
	PdcState          string             `json:"pdc_state,omitempty"`
	PdcRequestRemarks string             `json:"pdc_request_remarks,omitempty"`
}

// BackendSyncEvent is sent by the backend to every connecting EuroScope client
// immediately before the session_info event. It contains all strips in the session
// with the state fields that EuroScope must reflect locally.
type BackendSyncEvent struct {
	Strips    []BackendSyncStrip `json:"strips"`
	Latitude  float64            `json:"latitude"`
	Longitude float64            `json:"longitude"`
}

func (e BackendSyncEvent) GetType() EventType {
	return BackendSync
}

func (e BackendSyncEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

// CreateFPLEvent instructs EuroScope to create a flight plan in its session.
type CreateFPLEvent struct {
	Callsign          string `json:"callsign"`
	Origin            string `json:"origin"`
	Destination       string `json:"destination"`
	AlternateAD       string `json:"alternate_ad"`
	Sid               string `json:"sid"`
	AssignedSquawk    string `json:"assigned_squawk"`
	Eobt              string `json:"eobt"`
	AircraftType      string `json:"aircraft_type"`
	RequestedAltitude int32  `json:"requested_altitude"`
	Route             string `json:"route"`
	Stand             string `json:"stand"`
	Runway            string `json:"runway"`
	Remarks           string `json:"remarks"`
	PersonsOnBoard    int    `json:"persons_on_board"`
	FplType           string `json:"fpl_type"`
	Language          string `json:"language"`
}

func (e CreateFPLEvent) GetType() EventType {
	return CreateFPL
}

func (e CreateFPLEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

// PdcStateChangeEvent is sent by the backend to EuroScope clients when PDC state changes.
type PdcStateChangeEvent struct {
	Callsign          string `json:"callsign"`
	State             string `json:"state"`
	PdcRequestRemarks string `json:"pdc_request_remarks,omitempty"`
}

func (e PdcStateChangeEvent) GetType() EventType {
	return PdcStateChange
}

func (e PdcStateChangeEvent) Marshal() ([]byte, error) {
	return marshall(e)
}

// IssuePdcClearanceEvent is sent by the EuroScope plugin to issue a PDC clearance.
type IssuePdcClearanceEvent struct {
	Callsign string `json:"callsign"`
	Remarks  string `json:"remarks"`
}

// PdcRevertToVoiceEvent is sent by the EuroScope plugin to revert PDC to voice.
type PdcRevertToVoiceEvent struct {
	Callsign string `json:"callsign"`
}
