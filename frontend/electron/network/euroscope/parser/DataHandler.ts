import { BrowserWindow } from "electron";
import { ControllerDataUpdated } from "../interfaces/ControllerDataUpdated";
import { FlightDataUpdatedMessage } from "../interfaces/FlightDataUpdatedMessage";
import { FlightPlanDisconnected } from "../interfaces/FlightPlanDisconnected";

let index = 0;
// for now assume messages are less than 4096 bytes.
const dataBuffer: Buffer = Buffer.alloc(4096)


export function onData(data: Buffer, window: BrowserWindow) {

    for (let i = 0; i < data.length; i++) {
        const byte = data[i];

        if (byte == 0) {
            // new message
            parseMessage(dataBuffer.subarray(0, index), window) 
            index = 0;
            continue;
        }

        dataBuffer[index++] = byte;
    }
}

function parseMessage(bytes: Buffer, window: BrowserWindow) {
    let json = new TextDecoder().decode(bytes)
    let obj = JSON.parse(json)
    let message = obj as FlightDataUpdatedMessage | ControllerDataUpdated | FlightPlanDisconnected

    if (message.$type == 'FlightPlanUpdated') {
        window.webContents.send('FlightPlanUpdated', JSON.stringify(message))
    }
}