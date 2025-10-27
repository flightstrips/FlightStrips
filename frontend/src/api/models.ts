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
}

export enum ActionType {
  FrontendToken = "token",
  FrontendMove = "move",
  FrontendGenerateSquawk = "generate_squawk",
  FrontendUpdateStripData = "update_strip_data",
}

export enum Bay {
  Unknown = "UNKNOWN",
  NotCleared = "NOT_CLEARED",
  Cleared = "CLEARED",
  Push = "PUSH",
  Taxi = "TAXI",
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
  eldt: string;
  bay: string;
  release_point: string;
  version: number;
  sequence: number;
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
  callsign: string;
  runway_setup: RunwayConfiguration;
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
  eldt: string;
  bay: string;
  release_point: string;
  version: number;
  sequence: number;
  next_owners: string[];
  previous_owners: string[];
  owner: string;
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
  | FrontendOwnersUpdateEvent;

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

// Union type for all events that can be sent
export type FrontendSendEvent = FrontendAuthenticationEvent | FrontendMoveEvent | FrontendGenerateSquawkEvent | FrontendUpdateStripDataEvent;
