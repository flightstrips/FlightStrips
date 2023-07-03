import { ipcRenderer } from "electron";
import { FlightDataUpdatedMessage } from "../network/euroscope/interfaces/FlightDataUpdatedMessage";
import { CommunicationType } from "../../shared/CommunicationType";
import { ActiveRunway } from "../../shared/ActiveRunway";

export default {
    onFlightPlanUpdated: (handler: (plan: FlightDataUpdatedMessage) => void) => {
        ipcRenderer.on('FlightPlanUpdated', (_, args) => handler(JSON.parse(args) as FlightDataUpdatedMessage))
    },
    onFlightPlanDisconnect: (handler: (callsign: string) => void) => {
        ipcRenderer.on('FlightPlanDiconnect', (_, args) => handler(args))
    },
    onSetSquawk: (handler: (callsign: string, squawk: number) => void) => {
        ipcRenderer.on("SetSquawk", (_, ...args) => handler(args[0], args[1]))
    },
    onSetFinalAltitude: (handler: (callsign: string, altitude: number) => void) => {
        ipcRenderer.on("SetFinalAltitude", (_, ...args) => handler(args[0], args[1]))
    },
    onSetClearedAltitude: (handler: (callsign: string, altitude: number) => void) => {
        ipcRenderer.on("SetClearedAltitude", (_, ...args) => handler(args[0], args[1]))
    },
    onSetCommunicationType: (handler: (callsign: string, communication_type: CommunicationType) => void) => {
        ipcRenderer.on("SetCommunicationType", (_, ...args) => handler(args[0], args[1]))
    },
    onSetGroundState: (handler: (callsign: string, state: string) => void) => {
        ipcRenderer.on("SetGroundState", (_, ...args) => handler(args[0], args[1]))
    },
    onSetCleared: (handler: (callsign: string, clear: boolean) => void) => {
        ipcRenderer.on("SetCleared", (_, ...args) => handler(args[0], args[1]))
    },
    onActiveRunways: (handler: (runways: ActiveRunway[]) => void) => {
        ipcRenderer.on("OnActiveRunways", (_, args) => handler(args))
    }
}