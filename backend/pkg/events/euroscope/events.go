package euroscope

import (
	"FlightStrips/pkg/events"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"strings"
)

const (
	Authentication            = EventType_EVENT_TOKEN
	Login                     = EventType_EVENT_LOGIN
	ControllerOnline          = EventType_EVENT_CONTROLLER_ONLINE
	ControllerOffline         = EventType_EVENT_CONTROLLER_OFFLINE
	Sync                      = EventType_EVENT_SYNC
	AssignedSquawk            = EventType_EVENT_ASSIGNED_SQUAWK
	Squawk                    = EventType_EVENT_SQUAWK
	RequestedAltitude         = EventType_EVENT_REQUESTED_ALTITUDE
	ClearedAltitude           = EventType_EVENT_CLEARED_ALTITUDE
	CommunicationType         = EventType_EVENT_COMMUNICATION_TYPE
	GroundState               = EventType_EVENT_GROUND_STATE
	ClearedFlag               = EventType_EVENT_CLEARED_FLAG
	PositionUpdate            = EventType_EVENT_AIRCRAFT_POSITION_UPDATE
	SetHeading                = EventType_EVENT_HEADING
	AircraftDisconnected      = EventType_EVENT_AIRCRAFT_DISCONNECT
	Stand                     = EventType_EVENT_STAND
	TrackingControllerChanged = EventType_EVENT_TRACKING_CONTROLLER_CHANGED
	StripUpdate               = EventType_EVENT_STRIP_UPDATE
	RunwayType                = EventType_EVENT_RUNWAY
	SessionInfo               = EventType_EVENT_SESSION_INFO
	RunwayMismatchAlert       = EventType_EVENT_RUNWAY_MISMATCH_ALERT
	GenerateSquawk            = EventType_EVENT_GENERATE_SQUAWK
	Eobt                      = EventType_EVENT_EOBT
	Route                     = EventType_EVENT_ROUTE
	Remarks                   = EventType_EVENT_REMARKS
	AircraftInfo              = EventType_EVENT_AIRCRAFT_INFO
	AircraftInfoRemarks       = EventType_EVENT_AIRCRAFT_INFO_REMARKS
	Sid                       = EventType_EVENT_SID
	AircraftRunway            = EventType_EVENT_AIRCRAFT_RUNWAY
	CoordinationHandover      = EventType_EVENT_COORDINATION_HANDOVER
	CoordinationReceived      = EventType_EVENT_COORDINATION_RECEIVED
	AssumeOnly                = EventType_EVENT_ASSUME_ONLY
	AssumeAndDrop             = EventType_EVENT_ASSUME_AND_DROP
	DropTracking              = EventType_EVENT_DROP_TRACKING
	BackendSync               = EventType_EVENT_BACKEND_SYNC
	CreateFPL                 = EventType_EVENT_CREATE_FPL
	CdmUpdate                 = EventType_EVENT_CDM_UPDATE
	CdmUpdateBatch            = EventType_EVENT_CDM_UPDATE_BATCH
	CdmTobtUpdate             = EventType_EVENT_CDM_TOBT_UPDATE
	CdmAsrtToggle             = EventType_EVENT_CDM_ASRT_TOGGLE
	CdmTsacUpdate             = EventType_EVENT_CDM_TSAC_UPDATE
	CdmDeiceUpdate            = EventType_EVENT_CDM_DEICE_UPDATE
	CdmManualCtot             = EventType_EVENT_CDM_MANUAL_CTOT
	CdmCtotRemove             = EventType_EVENT_CDM_CTOT_REMOVE
	CdmReady                  = EventType_EVENT_CDM_READY
	PdcStateChange            = EventType_EVENT_PDC_STATE_CHANGE
	IssuePdcClearance         = EventType_EVENT_ISSUE_PDC_CLEARANCE
	PdcRevertToVoice          = EventType_EVENT_PDC_REVERT_TO_VOICE
	SendPrivateMessage        = EventType_EVENT_SEND_PRIVATE_MESSAGE
	Hold                      = EventType_EVENT_HOLD
	AMANGainLoss              = EventType_EVENT_AMAN_GAIN_LOSS
	AMANRouteFact             = EventType_EVENT_AMAN_ROUTE_FACT
)

type SyncRunway = Runway

const (
	GroundStateUnknown  = ""
	GroundStateStartup  = "ST-UP"
	GroundStatePush     = "PUSH"
	GroundStateTaxi     = "TAXI"
	GroundStateLineup   = "LINEUP"
	GroundStateDepart   = "DEPA"
	GroundStateParked   = "PARK"
	SessionInfoMaster   = "master"
	SessionInfoSlave    = "slave"
	SessionInfoObserver = "observer"
)

type OutgoingMessage interface {
	events.OutgoingMessage
	GetType() EventType
}

type typedProtoMessage interface {
	proto.Message
	GetType() EventType
}

func marshalMessage(message typedProtoMessage) ([]byte, error) {
	return MarshalEnvelope(message, message.GetType())
}

func MarshalEnvelope(message proto.Message, eventType EventType) ([]byte, error) {
	envelope := &Envelope{}
	oneof := envelope.ProtoReflect().Descriptor().Oneofs().ByName("event")
	field := oneof.Fields().ByNumber(protoreflect.FieldNumber(eventType))
	if field == nil || field.Message().FullName() != message.ProtoReflect().Descriptor().FullName() {
		return nil, fmt.Errorf("protobuf event %s does not match envelope field %d", message.ProtoReflect().Descriptor().FullName(), eventType)
	}

	envelope.ProtoReflect().Set(field, protoreflect.ValueOfMessage(message.ProtoReflect()))
	return proto.Marshal(envelope)
}

func UnmarshalEnvelope(data []byte) (EventType, []byte, error) {
	var envelope Envelope
	if err := proto.Unmarshal(data, &envelope); err != nil {
		return EventType_EVENT_UNKNOWN, nil, err
	}

	reflection := envelope.ProtoReflect()
	field := reflection.WhichOneof(reflection.Descriptor().Oneofs().ByName("event"))
	if field == nil {
		return EventType_EVENT_UNKNOWN, nil, fmt.Errorf("protobuf envelope contains no event")
	}
	payload, err := proto.Marshal(reflection.Get(field).Message().Interface())
	if err != nil {
		return EventType_EVENT_UNKNOWN, nil, err
	}
	return EventType(field.Number()), payload, nil
}

func UnmarshalEvent(data []byte, expected EventType, target proto.Message) error {
	eventType, payload, err := UnmarshalEnvelope(data)
	if err != nil {
		return err
	}
	if eventType != expected {
		return fmt.Errorf("unexpected protobuf event %s, expected %s", eventType, expected)
	}
	return proto.Unmarshal(payload, target)
}

func EventName(eventType EventType) string {
	return strings.ToLower(strings.TrimPrefix(eventType.String(), "EVENT_"))
}

func (e SessionInfoEvent) GetType() EventType { return SessionInfo }
func (e SessionInfoEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e RunwayMismatchAlertEvent) GetType() EventType { return RunwayMismatchAlert }
func (e RunwayMismatchAlertEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmAsrtToggleEvent) GetType() EventType { return CdmAsrtToggle }
func (e CdmAsrtToggleEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmTsacUpdateEvent) GetType() EventType { return CdmTsacUpdate }
func (e CdmTsacUpdateEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmUpdateEvent) GetType() EventType { return CdmUpdate }
func (e CdmUpdateEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmUpdateBatchEvent) GetType() EventType { return CdmUpdateBatch }
func (e CdmUpdateBatchEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmTobtUpdateEvent) GetType() EventType { return CdmTobtUpdate }
func (e CdmTobtUpdateEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmDeiceUpdateEvent) GetType() EventType { return CdmDeiceUpdate }
func (e CdmDeiceUpdateEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmManualCtotEvent) GetType() EventType { return CdmManualCtot }
func (e CdmManualCtotEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmCtotRemoveEvent) GetType() EventType { return CdmCtotRemove }
func (e CdmCtotRemoveEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CdmReadyEvent) GetType() EventType { return CdmReady }
func (e CdmReadyEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e GenerateSquawkEvent) GetType() EventType { return GenerateSquawk }
func (e GenerateSquawkEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e EobtEvent) GetType() EventType { return Eobt }
func (e EobtEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e GroundStateEvent) GetType() EventType { return GroundState }
func (e GroundStateEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e ClearedFlagEvent) GetType() EventType { return ClearedFlag }
func (e ClearedFlagEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AssignedSquawkEvent) GetType() EventType { return AssignedSquawk }
func (e AssignedSquawkEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e RequestedAltitudeEvent) GetType() EventType { return RequestedAltitude }
func (e RequestedAltitudeEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e ClearedAltitudeEvent) GetType() EventType { return ClearedAltitude }
func (e ClearedAltitudeEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CommunicationTypeEvent) GetType() EventType { return CommunicationType }
func (e CommunicationTypeEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e HeadingEvent) GetType() EventType { return SetHeading }
func (e HeadingEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e StandEvent) GetType() EventType { return Stand }
func (e StandEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e RouteEvent) GetType() EventType { return Route }
func (e RouteEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e RemarksEvent) GetType() EventType { return Remarks }
func (e RemarksEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AircraftInfoEvent) GetType() EventType { return AircraftInfo }
func (e AircraftInfoEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AircraftInfoRemarksEvent) GetType() EventType { return AircraftInfoRemarks }
func (e AircraftInfoRemarksEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e SidEvent) GetType() EventType { return Sid }
func (e SidEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AircraftRunwayEvent) GetType() EventType { return AircraftRunway }
func (e AircraftRunwayEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e TrackingControllerChangedEvent) GetType() EventType { return TrackingControllerChanged }
func (e TrackingControllerChangedEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AssumeOnlyEvent) GetType() EventType { return AssumeOnly }
func (e AssumeOnlyEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e AssumeAndDropEvent) GetType() EventType { return AssumeAndDrop }
func (e AssumeAndDropEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e DropTrackingEvent) GetType() EventType { return DropTracking }
func (e DropTrackingEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CoordinationHandoverEvent) GetType() EventType { return CoordinationHandover }
func (e CoordinationHandoverEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e BackendSyncEvent) GetType() EventType { return BackendSync }
func (e BackendSyncEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e CreateFPLEvent) GetType() EventType { return CreateFPL }
func (e CreateFPLEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e PdcStateChangeEvent) GetType() EventType { return PdcStateChange }
func (e PdcStateChangeEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}

func (e SendPrivateMessageEvent) GetType() EventType { return SendPrivateMessage }
func (e SendPrivateMessageEvent) Marshal() ([]byte, error) {
	return marshalMessage(&e)
}
