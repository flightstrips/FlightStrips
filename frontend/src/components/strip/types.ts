import { Bay } from "@/api/models";
import type { PdcStatus } from "@/api/models";

export type StripStatus = "CLR" | "CLROK" | "HALF" | "PUSH" | "ARR" | "CLX-HALF" | "TAXI-DEP" | "TWY-DEP" | "FINAL-ARR";

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
  unexpectedChangeFields?: string[];
  controllerModifiedFields?: string[];
  eobt?: string;
  tobt?: string;
  tsat?: string;
  ctot?: string;
  aircraftType?: string;
  aircraftCategory?: string;
  squawk?: string;
  assignedSquawk?: string;
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
  runwayCleared?: boolean;
  runwayConfirmed?: boolean;
  registration?: string;
  fullWidth?: boolean;
  isManual?: boolean;
}

export const TWY_DEP_STRIP_WIDTH = 519; // W_SI(40) + W_CALLSIGN(120) + W_TYPE_SQ(60) + W_STAND_CTOT(60) + W_SMALL(53)*3 + W_SID_DEST(80)

export function getStripBg(pdcStatus?: PdcStatus, isArrival?: boolean, bay?: Bay): string {
  const pdcAllowed = !bay || bay === Bay.NotCleared || bay === Bay.Cleared;
  if (pdcAllowed) {
    if (pdcStatus === "REQUESTED") return "var(--color-pdc-requested)";
    if (pdcStatus === "REQUESTED_WITH_FAULTS") return "var(--color-pdc-faults)";
    if (pdcStatus === "CLEARED")   return "var(--color-pdc-cleared)";
  }
  return isArrival ? "var(--color-strip-arr-bg)" : "var(--color-strip-dep-bg)";
}
