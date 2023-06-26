import { AircraftType } from "./AircraftType";
import { Capibilities } from "./Capibilities";
import { COmmunicationType } from "./CommunicationType";
import { FlightRules } from "./FlightRules";
import { Message } from "./Message";
import { Wtc } from "./Wtc";

export interface FlightDataUpdatedMessage extends Message {
    $type: "FlightPlanUpdated",
    callsign: string,
    origin: string,
    destination: string,
    alternate: string,
    planType: FlightRules,
    capibilities: Capibilities
    aircraftWtc: Wtc
    aircraftType: AircraftType,
    aircraftFPType: string,
    route: string,
    remarks: string,
    communicationType: COmmunicationType,
    departureRwy: string,
    arrivalRwy: string,
    sidName: string,
    starName: string,
    estimatedDeparture: string,
    finalAltitude: number,
}