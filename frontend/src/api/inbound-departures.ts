import {EventType} from "./models";

export interface InboundDeparture {
  callsign: string;
  departure: string;
  arrival: string;
  ctot: string;
  atot: string;
  eta: string;
  status: "early" | "late";
  deviation_seconds: number;
  outside_window_seconds: number;
}

export interface InboundCtotFlight {
  callsign: string;
  departure: string;
  arrival: string;
  eobt: string;
  tobt: string;
  ctot: string;
  atot: string;
  eta: string;
  cdm_status: string;
  regulation: string;
  status: "pending" | "compliant" | "early" | "late";
  deviation_seconds: number;
}

export interface InboundDeparturesSnapshot {
  session_id: number;
  airport: string;
  enabled: boolean;
  updated_at: string;
  stale: boolean;
  /** Early departures more than 10 minutes before CTOT. */
  flights: InboundDeparture[];
  /** Every inbound flight holding a CTOT. */
  ctot_flights?: InboundCtotFlight[];
}

export interface InboundDeparturesEvent extends InboundDeparturesSnapshot {
  type: EventType.FrontendInboundDepartures;
}

export function inboundTime(value: string): string {
  return /^\d{4}(\d{2})?$/.test(value) ? `${value.slice(0, 2)}:${value.slice(2, 4)}${value.length === 6 ? `:${value.slice(4)}` : ""}Z` : "—";
}

export function inboundDuration(seconds: number): string {
  const minutes = Math.floor(Math.abs(seconds) / 60);
  const remainder = Math.abs(seconds) % 60;
  return `${minutes}m${remainder ? ` ${remainder}s` : ""}`;
}

export function inboundDepartureDescription(flight: InboundDeparture): string {
  return `CTOT ${inboundTime(flight.ctot)} · ATOT ${inboundTime(flight.atot)} · ${inboundDuration(flight.deviation_seconds)} ${flight.status === "early" ? "before" : "after"} CTOT · ${inboundDuration(flight.outside_window_seconds)} outside window`;
}
