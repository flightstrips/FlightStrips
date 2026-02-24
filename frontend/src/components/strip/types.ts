import type { PdcStatus } from "@/api/models";

export type StripStatus = "CLR" | "CLROK" | "HALF";

export interface StripProps {
  callsign: string;
  pdcStatus?: PdcStatus;
  destination?: string;
  origin?: string;
  stand?: string;
  standChanged?: boolean;
  eobt?: string;
  tobt?: string;
  tsat?: string;
  ctot?: string;
  aircraftType?: string;
  squawk?: string;
  sid?: string;
  runway?: string;
  clearedAltitude?: number;
  requestedAltitude?: number;
  taxiway?: string;
  holdingPoint?: string;
  clearances?: boolean;
  frequency?: string;
  arrival?: boolean;
  owner?: string;
}

export function getStripBg(pdcStatus?: PdcStatus | string): string {
  return pdcStatus === "REQUESTED" || pdcStatus === "CLEARED"
    ? "#393455"
    : "#bef5ef";
}
