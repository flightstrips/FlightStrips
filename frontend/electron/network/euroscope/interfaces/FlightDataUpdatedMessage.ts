import { FlightPlanUpdate } from "../../../../shared/FlightPlanUpdate";
import { Message } from "./Message";

export interface FlightDataUpdatedMessage extends Message, FlightPlanUpdate {
    $type: "FlightPlanUpdated",
}