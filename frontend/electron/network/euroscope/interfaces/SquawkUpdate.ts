import { Message } from "./Message";

export interface SquawkUpdate extends Message {
    $type: 'SquawkUpdate'
    callsign: string,
    squawk: number
}