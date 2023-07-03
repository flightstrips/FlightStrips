import { ActiveRunway } from "../../../../shared/ActiveRunway";
import { Message } from "./Message";

export interface ActiveRunwaysMessage extends Message {
    $type: 'ActiveRunways',
    runways: ActiveRunway[]
}