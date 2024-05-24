export interface AtisUpdate {
  letter: string
  callsign: string
  metar: string
}

export enum ControllerState {
  Online = 'Online',
  Offline = 'Offline',
}

export interface ControllerUpdate {
  state: ControllerState
  frequency: string
  position: string
}

export enum CoordinationState {
  Created = 'Created',
  Accepted = 'Accepted',
  Rejected = 'Rejected',
  Cancelled = 'Cancelled',
}

export interface CoordinationUpdate {
  coordinationId: number
  callsign: string
  to: string
  from: string
  state: CoordinationState
}

export enum CommunicationType {
  unassigned = 'Unassigned',
  voice = 'Voice',
  receive = 'Receive',
  text = 'Text',
}

export enum WeightCategory {
  unknown = 'Unknown',
  light = 'Light',
  medium = 'Medium',
  heavy = 'Heavy',
  superHeavy = 'SuperHeavy',
}

export interface StripUpdate {
  callsign: string
  origin: string
  destination: string
  alternate: string
  route: string
  remarks: string
  assignedSquawk: string
  squawk: string
  sid: string | null
  clearedAltitude: number | null
  finalAltitude: number
  heading: number | null
  aircraftCategory: WeightCategory
  aircraftType: string
  runway: string
  capabilities: string
  communicationType: CommunicationType
  stand: string
  tobt: string
  tsat: string | null
  sequence: number | null
  cleared: boolean
  controller: string | null
  bay: string
}

export interface StripDisconnectUpdate {
  callsign: string
}

export interface SubscribeRequest {
  airport: string
  session: 'live' | 'test'
  frequency: string
}
export interface UnsubscribeRequest extends SubscribeRequest {
  unsubscribeFromAirport: boolean
}

export interface SectorUpdate {
  frequency: string
  sectors: string[]
}

export interface RunwayConfiguration {
  departure: string | null
  arrival: string | null
}
