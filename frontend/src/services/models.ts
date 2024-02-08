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

export enum StripStateEvent {
  Created = 'Created',
  Updated = 'Updated',
  Deleted = 'Deleted',
}

export interface StripUpdate {
  callsign: string
  origin: string | null
  destination: string | null
  sequence: number | null
  state: StripStateEvent
  cleared: boolean
  positionFrequency: string | null
  bay: string
  eventState: StripStateEvent
}

export interface SubscribeRequest {
  airport: string
  session: 'live' | 'test'
  frequency: string
}
export interface UnsubscribeRequest extends SubscribeRequest {
  unsubscribeFromAirport: boolean
}

export interface SectorUpadet {
  frequency: string
  sectors: string[]
}
