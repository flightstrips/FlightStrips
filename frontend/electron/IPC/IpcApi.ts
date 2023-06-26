import { ipcRenderer } from "electron";
import { FlightDataUpdatedMessage } from "../network/euroscope/interfaces/FlightDataUpdatedMessage";

export default {
    onFlightPlanUpdated: (handler: (plan: FlightDataUpdatedMessage) => void) => {
        ipcRenderer.on('FlightPlanUpdated', (_, args) => handler(JSON.parse(args) as FlightDataUpdatedMessage))
    }
}