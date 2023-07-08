import { ActiveRunway } from '../../../../shared/ActiveRunway'
import { CommunicationType } from '../../../../shared/CommunicationType'
import { FlightPlanUpdate } from '../../../../shared/FlightPlanUpdate'
import { GroundState } from '../../../../shared/GroundState'

export interface IpcInterface {
  sendFlightPlanUpdate(plan: FlightPlanUpdate): void
  sendFlightPlanDisconnect(callsign: string): void
  sendSetSquawk(callsign: string, squawk: number): void
  sendSetFinalAltitude(callsign: string, altitude: number): void
  sendSetClearedAltitude(callsign: string, altitude: number): void
  sendSetCleared(callsign: string, clear: boolean): void
  sendSetCommunicationType(
    callsign: string,
    communication_type: CommunicationType,
  ): void
  sendSetGroundState(callsign: string, state: GroundState): void
  sendSquawkUpdate(callsign: string, squawk: number): void
  sendActiveRunways(runways: ActiveRunway[]): void
}
