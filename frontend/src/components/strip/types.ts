import type { Bay, PdcStatus } from "@/api/models";

export type StripStatus = "CLR" | "CLROK" | "HALF" | "PUSH" | "ARR" | "CLX-HALF" | "TAXI-DEP" | "TWY-DEP";

export type HalfStripVariant =
  | "APN-PUSH"    // Pushback bays
  | "APN-ARR"     // Arrival taxi bays
  | "LOCKED-DEP"  // DEP-LOCKED bays (read-only)
  | "LOCKED-ARR"  // ARR-LOCKED bays (read-only)
  | "MESSAGES"    // MESSAGES bay
  | "MEM-AID"     // Memory aid strip
  | "LAND-START"  // Land/start clearance
  | "CROSSING";   // Runway crossing

export interface StripProps {
  halfStripVariant?: HalfStripVariant;
  bay?: Bay;
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
  heading?: number;
  arrival?: boolean;
  owner?: string;
  nextControllers?: string[];
  previousControllers?: string[];
  myPosition?: string;
  selectable?: boolean;
  marked?: boolean;
  registration?: string;
  fullWidth?: boolean;
}

export function getStripBg(pdcStatus?: PdcStatus, isArrival?: boolean): string {
  if (pdcStatus === "REQUESTED") return "#B8860B";
  if (pdcStatus === "CLEARED")   return "#00154A";
  return isArrival ? "#fff28e" : "#bef5ef";
}
