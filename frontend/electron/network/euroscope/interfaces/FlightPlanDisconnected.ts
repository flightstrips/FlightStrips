import { Message } from "./Message";

export interface FlightPlanDisconnected extends Message {
    $type: 'FlightPlanDisconnected',
    callsign: string
}