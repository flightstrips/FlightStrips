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
  cleared: boolean;
  cleared_altitude: number;
  requested_altitude: number;
  heading: number;
  aircraft_type: string;
  aircraft_category: string;
  stand: string;
  capabilities: string;
  communication_type: string;
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
}

export interface FrontendInitialEvent {
  type: EventType.FrontendInitial;
  controllers: FrontendController[];
  strips: FrontendStrip[];
  position: string;
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
  cleared: boolean;
  cleared_altitude: number;
  requested_altitude: number;
  heading: number;
  aircraft_type: string;
  aircraft_category: string;
  stand: string;
  capabilities: string;
  communication_type: string;
  eobt: string;
  eldt: string;
  bay: string;
  release_point: string;
  version: number;
  sequence: number;
}

export interface FrontendControllerOnlineEvent {
  type: EventType.FrontendControllerOnline;
  callsign: string;
  position: string;
}

export interface FrontendControllerOfflineEvent {
  type: EventType.FrontendControllerOffline;
  callsign: string;
  position: string;
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
  type: 'token';
  token: string;
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
  | FrontendBayEvent;

// Union type for all events that can be sent
export type FrontendSendEvent = FrontendAuthenticationEvent
