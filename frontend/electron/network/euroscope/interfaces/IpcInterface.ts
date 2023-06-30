import { CommunicationType } from "../../../../shared/CommunicationType"
import { FlightPlanUpdate } from "../../../../shared/FlightPlanUpdate"

export interface IpcInterface {
    sendFlightPlanUpdate(plan: FlightPlanUpdate): void
    sendFlightPlanDisconnect(callsign: string): void
    sendSetSquawk(callsign: string, squawk: number): void
    sendSetFinalAltitude(callsign: string, altitude: number): void
    sendSetClearedAltitude(callsign: string, altitude: number): void
    sendSetCleared(callsign: string, clear: boolean): void
    sendSetCommunicationType(callsign: string, communication_type: CommunicationType): void
    sendSetGroundState(callsign: string, state: string): void
}