import { WebContents } from "electron";
import { CommunicationType } from "../../../shared/CommunicationType";
import { FlightPlanUpdate } from "../../../shared/FlightPlanUpdate";
import { IpcInterface } from "./interfaces/IpcInterface";

export class Ipc implements IpcInterface {
    private readonly webContents: WebContents

    constructor(webContents: WebContents) {
        this.webContents = webContents
    }

    sendFlightPlanUpdate(plan: FlightPlanUpdate): void {
        this.webContents.send('FlightPlanUpdated', JSON.stringify(plan))
    }
    sendFlightPlanDisconnect(callsign: string): void {
        this.webContents.send('FlightPlanDisconnect', callsign)
    }
    sendSetSquawk(callsign: string, squawk: number): void {
        this.webContents.send('SetSquawk', callsign, squawk)
    }
    sendSetFinalAltitude(callsign: string, altitude: number): void {
        this.webContents.send("SetFinalAltitude", callsign, altitude)
    }
    sendSetClearedAltitude(callsign: string, altitude: number): void {
        this.webContents.send('SetClearedAltitude', callsign, altitude)
    }
    sendSetCleared(callsign: string, clear: boolean): void {
        this.webContents.send('SetCleared', callsign, clear)
    }
    sendSetCommunicationType(callsign: string, communication_type: CommunicationType): void {
        this.webContents.send('SetCommunicationType', callsign, communication_type)
    }
    sendSetGroundState(callsign: string, state: string): void {
        this.webContents.send('SetGroundState', callsign, state)
    }
    sendSquawkUpdate(callsign: string, squawk: number): void {
        this.webContents.send("SquawkUpdaet", callsign, squawk)
    }
}