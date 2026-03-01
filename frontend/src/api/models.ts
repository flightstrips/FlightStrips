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
  FrontendIssuePdcClearanceRequest = "issue_pdc_clearance",
  FrontendRevertToVoiceRequest = "revert_to_voice",
  FrontendCoordinationTransferRequest = "coordination_transfer_request",
  FrontendCoordinationAssumeRequest = "coordination_assume_request",
  FrontendCoordinationFreeRequest = "coordination_free_request",
}

export type PdcStatus = "NONE" | "REQUESTED" | "CLEARED" | "CONFIRMED" | "NO_RESPONSE" | "FAILED" | "REVERT_TO_VOICE";

export enum Bay {
  Unknown = "UNKNOWN",
  NotCleared = "NOT_CLEARED",
  Cleared = "CLEARED",
  Push = "PUSH",
  Taxi = "TAXI",
  DeIce = "DE_ICE",
  Depart = "DEPART",
  Airborne = "AIRBORNE",
  Final = "FINAL",
  Stand = "STAND",
  Hidden = "HIDDEN",
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
}

export interface FrontendController {
  callsign: string;
  position: string;
  identifier: string;
}

export interface FrontendInitialEvent {
  type: EventType.FrontendInitial;
  controllers: FrontendController[];
  strips: FrontendStrip[];
  me: FrontendController;
  airport: string;
  layout: string;
  callsign: string;
  runway_setup: RunwayConfiguration;
  coordinations: Array<{ callsign: string; from: string; to: string }>;
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
}

export interface FrontendControllerOnlineEvent {
  type: EventType.FrontendControllerOnline;
  callsign: string;
  position: string;
  identifier: string;
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
  | FrontendRunwayConfigurationEvent;

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
}

export interface FrontendUpdateOrder {
  type: ActionType.FrontendUpdateOrder;
  callsign: string;
  before: string | null;
}

export interface FrontendSendMessageEvent {
  type: ActionType.FrontendSendMessage;
  message: string;
  to: string | null
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

export interface FrontendCoordinationFreeRequestEvent {
  type: ActionType.FrontendCoordinationFreeRequest;
  callsign: string;
}

// Union type for all events that can be sent
export type FrontendSendEvent = FrontendAuthenticationEvent | FrontendMoveEvent | FrontendGenerateSquawkEvent | FrontendUpdateStripDataEvent | FrontendUpdateOrder | FrontendSendMessageEvent | FrontendCdmReadyEvent | FrontendSendReleasePointEvent | FrontendSendMarkedEvent | FrontendIssuePdcClearanceRequest | FrontendRevertToVoiceRequest | FrontendCoordinationTransferRequestEvent | FrontendCoordinationAssumeRequestEvent | FrontendCoordinationFreeRequestEvent;
