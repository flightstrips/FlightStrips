export enum EventType {
  FrontendInitial = "initial",
  FrontendStripUpdate = "strip_update",
  FrontendControllerOnline = "controller_online",
  FrontendControllerOffline = "controller_offline",
  FrontendAssignedSquawk = "assigned_squawk",
  FrontendSquawk = "squawk",
  FrontendRequestedAltitude = "requested_altitude",
  FrontendClearedAltitude = "cleared_altitude",
  FrontendBay = "bay",
  FrontendBulkBay = "bulk_bay",
  FrontendDisconnect = "disconnect",
  FrontendAircraftDisconnect = "aircraft_disconnect",
  FrontendStand = "stand",
  FrontendSetHeading = "set_heading",
  FrontendCommunicationType = "communication_type",
  FrontendOwnersUpdate = "owners_update",
  FrontendLayoutUpdate = "layout_update",
  FrontendBroadcast = "broadcast",
  FrontendCdmData = "cdm_data",
  FrontendCdmWait = "cdm_wait",
  FrontendReleasePoint = "release_point",
  FrontendPdcStateChange = "pdc_state_change",
  FrontendMarked = "marked",
  FrontendCoordinationTransferBroadcast = "coordination_transfer_broadcast",
  FrontendCoordinationAssumeBroadcast = "coordination_assume_broadcast",
  FrontendCoordinationRejectBroadcast = "coordination_reject_broadcast",
  FrontendCoordinationFreeBroadcast = "coordination_free_broadcast",
  FrontendRunWayConfiguration = "run_way_configuration",
  FrontendTacticalStripCreated = "tactical_strip_created",
  FrontendTacticalStripDeleted = "tactical_strip_deleted",
  FrontendTacticalStripUpdated = "tactical_strip_updated",
  FrontendTacticalStripMoved = "tactical_strip_moved",
  FrontendMessageReceived = "message_received",
  FrontendAtisUpdate = "atis_update",
  FrontendActionRejected = "action_rejected",
  ConnectRejected = "connect_rejected",
  FrontendAvailableSids = "available_sids",
}

export enum ActionType {
  FrontendToken = "token",
  FrontendMove = "move",
  FrontendGenerateSquawk = "generate_squawk",
  FrontendUpdateStripData = "update_strip_data",
  FrontendUpdateOrder = "update_order",
  FrontendSendMessage = "send_message",
  FrontendCdmReady = "cdm_ready",
  FrontendReleasePoint = "release_point",
  FrontendMarked = "marked",
  FrontendRunwayClearance = "runway_clearance",
  FrontendIssuePdcClearanceRequest = "issue_pdc_clearance",
  FrontendRevertToVoiceRequest = "revert_to_voice",
  FrontendCoordinationTransferRequest = "coordination_transfer_request",
  FrontendCoordinationAssumeRequest = "coordination_assume_request",
  FrontendCoordinationFreeRequest = "coordination_free_request",
  FrontendCoordinationCancelTransferRequest = "coordination_cancel_transfer_request",
  FrontendCoordinationForceAssumeRequest = "coordination_force_assume_request",
  FrontendCreateTacticalStrip = "create_tactical_strip",
  FrontendDeleteTacticalStrip = "delete_tactical_strip",
  FrontendConfirmTacticalStrip = "confirm_tactical_strip",
  FrontendStartTacticalTimer = "start_tactical_timer",
  FrontendMoveTacticalStrip = "move_tactical_strip",
  FrontendAcknowledgeUnexpectedChange = "acknowledge_unexpected_change",
}

export type PdcStatus = "NONE" | "REQUESTED" | "CLEARED" | "CONFIRMED" | "NO_RESPONSE" | "FAILED" | "REVERT_TO_VOICE";

export type TacticalStripType = "MEMAID" | "CROSSING" | "START" | "LAND";

export interface StripRef {
  kind: "flight" | "tactical";
  callsign?: string; // set when kind = "flight"
  id?: number;       // set when kind = "tactical"
}

export interface TacticalStrip {
  id: number;
  session_id: number;
  type: TacticalStripType;
  bay: string;
  label: string;
  aircraft: string;
  produced_by: string;
  sequence: number;
  timer_start: string | null;
  confirmed: boolean;
  confirmed_by: string;
  created_at: string;
}

export enum Bay {
  Unknown = "UNKNOWN",
  NotCleared = "NOT_CLEARED",
  Cleared = "CLEARED",
  Push = "PUSH",
  Taxi = "TAXI",
  TaxiLwr = "TAXI_LWR",
  DeIce = "TAXI_TWR",
  Depart = "DEPART",
  Airborne = "AIRBORNE",
  Final = "FINAL",
  Stand = "STAND",
  Hidden = "HIDDEN",
  ArrHidden = "ARR_HIDDEN",
  RwyDep = Depart, // alias — kept for backwards compat, same value
  RwyArr = "RWY_ARR",
  TwyArr = "TWY_ARR",
  Controlzone = "CONTROLZONE",
}

// Define interfaces for all event types
export interface RunwayConfiguration {
  departure: string[];
  arrival: string[];
}

export interface FrontendStrip {
  callsign: string;
  origin: string;
  destination: string;
  alternate: string;
  route: string;
  remarks: string;
  runway: string;
  squawk: string;
  assigned_squawk: string;
  sid: string;
  cleared_altitude: number;
  requested_altitude: number;
  heading: number;
  aircraft_type: string;
  aircraft_category: string;
  stand: string;
  capabilities: string;
  communication_type: CommunicationType;
  eobt: string;
  tobt: string;
  tsat: string;
  ctot: string;
  eldt: string;
  bay: string;
  release_point: string;
  version: number;
  sequence: number;
  next_controllers: string[];
  previous_controllers: string[];
  owner: string;
  pdc_state: PdcStatus;
  marked: boolean;
  runway_cleared: boolean;
  registration: string;
  ob?: boolean;
  unexpected_change_fields?: string[];
  controller_modified_fields?: string[];
}

export interface FrontendController {
  callsign: string;
  position: string;
  identifier: string;
  section: string;
}

export interface MessageReceived {
  id: number;
  sender: string;
  text: string;
  is_broadcast: boolean;
  recipients: string[];
}

export interface FrontendInitialEvent {
  type: EventType.FrontendInitial;
  controllers: FrontendController[];
  strips: FrontendStrip[];
  tactical_strips: TacticalStrip[];
  me: FrontendController;
  airport: string;
  layout: string;
  callsign: string;
  runway_setup: RunwayConfiguration;
  coordinations: Array<{ callsign: string; from: string; to: string }>;
  messages: MessageReceived[];
}

export interface FrontendStripUpdateEvent {
  type: EventType.FrontendStripUpdate;
  callsign: string;
  origin: string;
  destination: string;
  alternate: string;
  route: string;
  remarks: string;
  runway: string;
  squawk: string;
  assigned_squawk: string;
  sid: string;
  cleared_altitude: number;
  requested_altitude: number;
  heading: number;
  aircraft_type: string;
  aircraft_category: string;
  stand: string;
  capabilities: string;
  communication_type: CommunicationType;
  eobt: string;
  tobt: string;
  tsat: string;
  ctot: string;
  eldt: string;
  bay: string;
  release_point: string;
  version: number;
  sequence: number;
  next_controllers: string[];
  previous_controllers: string[];
  owner: string;
  pdc_state: PdcStatus;
  marked: boolean;
  runway_cleared: boolean;
  registration: string;
  unexpected_change_fields?: string[];
  controller_modified_fields?: string[];
}

export interface FrontendControllerOnlineEvent {
  type: EventType.FrontendControllerOnline;
  callsign: string;
  position: string;
  identifier: string;
  section: string;
}

export interface FrontendControllerOfflineEvent {
  type: EventType.FrontendControllerOffline;
  callsign: string;
  position: string;
  identifier: string;
}

export interface FrontendAssignedSquawkEvent {
  type: EventType.FrontendAssignedSquawk;
  callsign: string;
  squawk: string;
}

export interface FrontendSquawkEvent {
  type: EventType.FrontendSquawk;
  callsign: string;
  squawk: string;
}

export interface FrontendRequestedAltitudeEvent {
  type: EventType.FrontendRequestedAltitude;
  callsign: string;
  altitude: number;
}

export interface FrontendClearedAltitudeEvent {
  type: EventType.FrontendClearedAltitude;
  callsign: string;
  altitude: number;
}

export interface FrontendBayEvent {
  type: EventType.FrontendBay;
  callsign: string;
  bay: string;
  sequence: number;
}

export interface BulkBayEntry {
  callsign: string;
  sequence: number;
}

export interface FrontendBulkBayEvent {
  type: EventType.FrontendBulkBay;
  bay: string;
  strips: BulkBayEntry[];
}

// Define authentication event interface
export interface FrontendAuthenticationEvent {
  type: ActionType.FrontendToken;
  token: string;
}

export interface FrontendDisconnectEvent {
  type: EventType.FrontendDisconnect;
}

export interface FrontendAircraftDisconnectEvent {
  type: EventType.FrontendAircraftDisconnect;
  callsign: string;
}

export interface FrontendStandEvent {
  type: EventType.FrontendStand;
  callsign: string;
  stand: string;
}

export interface FrontendSetHeadingEvent {
  type: EventType.FrontendSetHeading;
  callsign: string;
  heading: number;
}

export enum CommunicationType {
  Voice = "V",
  Receive = "R",
  Text = "T",
  Unknown = ""
}

export interface FrontendCommunicationTypeEvent {
  type: EventType.FrontendCommunicationType;
  callsign: string;
  communication_type: CommunicationType;
}

export interface FrontendOwnersUpdateEvent {
  type: EventType.FrontendOwnersUpdate;
  callsign: string;
  owner: string;
  next_owners: string[];
  previous_owners: string[];
}

export interface FrontendLayoutUpdateEvent {
  type: EventType.FrontendLayoutUpdate;
  layout: string;
}

export interface FrontendBroadcastEvent {
  type: EventType.FrontendBroadcast;
  message: string;
  from: string;
}

export interface FrontendCdmDataEvent {
  type: EventType.FrontendCdmData;
  callsign: string;
  eobt: string;
  tobt: string;
  tsat: string;
  ctot: string;
}

export interface FrontendCdmWaitEvent {
  type: EventType.FrontendCdmWait;
  callsign: string;
}

export interface FrontendReleasePointEvent {
  type: EventType.FrontendReleasePoint;
  callsign: string;
  release_point: string;
}

export interface FrontendMarkedEvent {
  type: EventType.FrontendMarked;
  callsign: string;
  marked: boolean;
}

export interface FrontendSendMarkedEvent {
  type: ActionType.FrontendMarked;
  callsign: string;
  marked: boolean;
}

export interface FrontendSendRunwayClearanceEvent {
  type: ActionType.FrontendRunwayClearance;
  callsign: string;
}

export interface FrontendPdcStateUpdateEvent {
  type: EventType.FrontendPdcStateChange;
  callsign: string;
  state: "NONE" | "REQUESTED" | "CLEARED" | "CONFIRMED" | "NO_RESPONSE" | "FAILED";
}

export interface FrontendCoordinationTransferBroadcastEvent {
  type: EventType.FrontendCoordinationTransferBroadcast;
  callsign: string;
  from: string;
  to: string;
}

export interface FrontendCoordinationAssumeBroadcastEvent {
  type: EventType.FrontendCoordinationAssumeBroadcast;
  callsign: string;
  position: string;
}

export interface FrontendCoordinationRejectBroadcastEvent {
  type: EventType.FrontendCoordinationRejectBroadcast;
  callsign: string;
  position: string;
}

export interface FrontendCoordinationFreeBroadcastEvent {
  type: EventType.FrontendCoordinationFreeBroadcast;
  callsign: string;
}

export interface FrontendRunwayConfigurationEvent {
  type: EventType.FrontendRunWayConfiguration;
  runway_setup: RunwayConfiguration;
}

export interface FrontendTacticalStripCreatedEvent {
  type: EventType.FrontendTacticalStripCreated;
  strip: TacticalStrip;
}

export interface FrontendTacticalStripDeletedEvent {
  type: EventType.FrontendTacticalStripDeleted;
  id: number;
  bay: string;
}

export interface FrontendTacticalStripUpdatedEvent {
  type: EventType.FrontendTacticalStripUpdated;
  strip: TacticalStrip;
}

export interface FrontendTacticalStripMovedEvent {
  type: EventType.FrontendTacticalStripMoved;
  id: number;
  bay: string;
  sequence: number;
}

export interface FrontendMessageReceivedEvent extends MessageReceived {
  type: EventType.FrontendMessageReceived;
}

export interface FrontendAtisUpdateEvent {
  type: EventType.FrontendAtisUpdate;
  metar: string;
}

// Union type for all events that can be received
export type WebSocketEvent =
  | FrontendInitialEvent
  | FrontendStripUpdateEvent
  | FrontendControllerOnlineEvent
  | FrontendControllerOfflineEvent
  | FrontendAssignedSquawkEvent
  | FrontendSquawkEvent
  | FrontendRequestedAltitudeEvent
  | FrontendClearedAltitudeEvent
  | FrontendBayEvent
  | FrontendDisconnectEvent
  | FrontendAircraftDisconnectEvent
  | FrontendStandEvent
  | FrontendSetHeadingEvent
  | FrontendCommunicationTypeEvent
  | FrontendOwnersUpdateEvent
  | FrontendLayoutUpdateEvent
  | FrontendBroadcastEvent
  | FrontendCdmDataEvent
  | FrontendCdmWaitEvent
  | FrontendReleasePointEvent
  | FrontendMarkedEvent
  | FrontendPdcStateUpdateEvent
  | FrontendCoordinationTransferBroadcastEvent
  | FrontendCoordinationAssumeBroadcastEvent
  | FrontendCoordinationRejectBroadcastEvent
  | FrontendCoordinationFreeBroadcastEvent
  | FrontendRunwayConfigurationEvent
  | FrontendTacticalStripCreatedEvent
  | FrontendTacticalStripDeletedEvent
  | FrontendTacticalStripUpdatedEvent
  | FrontendTacticalStripMovedEvent
  | FrontendMessageReceivedEvent
  | FrontendAtisUpdateEvent
  | ActionRejectedEvent
  | ConnectRejectedEvent
  | FrontendBulkBayEvent;

export interface ActionRejectedEvent {
  type: EventType.FrontendActionRejected;
  action: string;
  reason: string;
}

export interface ConnectRejectedEvent {
  type: EventType.ConnectRejected;
  reason: string;
}

export interface FrontendMoveEvent {
  type: ActionType.FrontendMove;
  callsign: string;
  bay: Bay;
}

export interface FrontendGenerateSquawkEvent {
  type: ActionType.FrontendGenerateSquawk;
  callsign: string;
}

export interface FrontendUpdateStripDataEvent {
  type: ActionType.FrontendUpdateStripData;
  callsign: string;
  sid?: string;
  eobt?: string;
  route?: string;
  heading?: number;
  altitude?: number;
  stand?: string;
  runway?: string;
  ob?: boolean;
}

export interface FrontendUpdateOrder {
  type: ActionType.FrontendUpdateOrder;
  callsign: string;
  insert_after: StripRef | null;
}

export interface FrontendSendMessageEvent {
  type: ActionType.FrontendSendMessage;
  text: string;
  recipients: string[];
}

export interface FrontendCdmReadyEvent {
  type: ActionType.FrontendCdmReady;
  callsign: string;
}

export interface FrontendSendReleasePointEvent {
  type: ActionType.FrontendReleasePoint;
  callsign: string;
  release_point: string;
}

export interface FrontendIssuePdcClearanceRequest {
  type: ActionType.FrontendIssuePdcClearanceRequest;
  callsign: string;
  remarks: string | null;
}

export interface FrontendRevertToVoiceRequest {
  type: ActionType.FrontendRevertToVoiceRequest;
  callsign: string;
}

export interface FrontendCoordinationTransferRequestEvent {
  type: ActionType.FrontendCoordinationTransferRequest;
  callsign: string;
  to: string;
}

export interface FrontendCoordinationAssumeRequestEvent {
  type: ActionType.FrontendCoordinationAssumeRequest;
  callsign: string;
}

export interface FrontendCoordinationForceAssumeRequestEvent {
  type: ActionType.FrontendCoordinationForceAssumeRequest;
  callsign: string;
}

export interface FrontendCoordinationFreeRequestEvent {
  type: ActionType.FrontendCoordinationFreeRequest;
  callsign: string;
}

export interface FrontendCoordinationCancelTransferRequestEvent {
  type: ActionType.FrontendCoordinationCancelTransferRequest;
  callsign: string;
}

export interface FrontendCreateTacticalStripAction {
  type: ActionType.FrontendCreateTacticalStrip;
  strip_type: TacticalStripType;
  bay: string;
  label: string;
  aircraft: string;
}

export interface FrontendDeleteTacticalStripAction {
  type: ActionType.FrontendDeleteTacticalStrip;
  id: number;
}

export interface FrontendConfirmTacticalStripAction {
  type: ActionType.FrontendConfirmTacticalStrip;
  id: number;
}

export interface FrontendStartTacticalTimerAction {
  type: ActionType.FrontendStartTacticalTimer;
  id: number;
}

export interface FrontendMoveTacticalStripAction {
  type: ActionType.FrontendMoveTacticalStrip;
  id: number;
  insert_after: StripRef | null;
}

export interface FrontendAcknowledgeUnexpectedChangeEvent {
  type: ActionType.FrontendAcknowledgeUnexpectedChange;
  callsign: string;
  field_name: string;
}

// Union type for all events that can be sent
export type FrontendSendEvent = FrontendAuthenticationEvent | FrontendMoveEvent | FrontendGenerateSquawkEvent | FrontendUpdateStripDataEvent | FrontendUpdateOrder | FrontendSendMessageEvent | FrontendCdmReadyEvent | FrontendSendReleasePointEvent | FrontendSendMarkedEvent | FrontendSendRunwayClearanceEvent | FrontendIssuePdcClearanceRequest | FrontendRevertToVoiceRequest | FrontendCoordinationTransferRequestEvent | FrontendCoordinationAssumeRequestEvent | FrontendCoordinationForceAssumeRequestEvent | FrontendCoordinationFreeRequestEvent | FrontendCoordinationCancelTransferRequestEvent | FrontendCreateTacticalStripAction | FrontendDeleteTacticalStripAction | FrontendConfirmTacticalStripAction | FrontendStartTacticalTimerAction | FrontendMoveTacticalStripAction | FrontendAcknowledgeUnexpectedChangeEvent;

export type AnyStrip = FrontendStrip | TacticalStrip;
export const isFlight = (s: AnyStrip): s is FrontendStrip => 'callsign' in s;
/** Stable string ID for DnD frameworks — callsign for flights, "tactical-<id>" for tacticals. */
export const stripDndId = (s: AnyStrip): string =>
  isFlight(s) ? s.callsign : `tactical-${s.id}`;

export interface AvailableSidsEvent {
  type: EventType.FrontendAvailableSids;
  sids: string[];
}
