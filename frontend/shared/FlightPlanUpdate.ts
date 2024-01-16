import { Capibilities } from './Capibilities'
import { CommunicationType } from './CommunicationType'
import { FlightRules } from './FlightRules'
import { Wtc } from './Wtc'
import { AircraftType } from './AircraftType'

export interface FlightPlanUpdate {
  callsign: string
  origin: string
  destination: string
  alternate: string
  planType: FlightRules
  capibilities: Capibilities
  aircraftWtc: Wtc
  aircraftType: AircraftType
  aircraftFPType: string
  route: string
  remarks: string
  communicationType: CommunicationType
  departureRwy: string
  arrivalRwy: string
  sidName: string
  starName: string
  estimatedDepartureTime: string
  finalAltitude: number
  stand: string
}
