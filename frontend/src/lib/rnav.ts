import type { ClxValidationFault, FrontendStrip } from "@/api/models";
import { isTobtRelevant } from "@/lib/cdmColors";

export type RnavCapability = "1" | "2" | "5" | "10" | "NIL";

const PBN_BY_CAPABILITY: Record<Exclude<RnavCapability, "NIL">, string> = {
  "10": "PBN/A1",
  "5": "PBN/A1B1",
  "2": "PBN/A1B1C1",
  "1": "PBN/A1B1C1D1S1S2",
};

const PBN_TOKEN_PATTERN = /^PBN\/[A-Z0-9]+$/i;
const ASSUMED_RNAV1_AIRCRAFT_PATTERN = /^(?:A3[A-Z0-9]{2}|B73[6-9]|B38M|B74[48]|B75[23]|B76[234]|B77[A-Z0-9]|B78[A-Z0-9]|BCS3|AT[A-Z0-9]{2}|DH8D|CRJ[A-Z0-9]|E[A-Z0-9]{3}|MD8[A-Z0-9])$/;

export const RNAV_CAPABILITIES: RnavCapability[] = ["1", "2", "5", "10", "NIL"];

export function effectiveRnavCapability(aircraftInfo: string, remarks: string): RnavCapability {
  const aircraft = aircraftInfo.trim().toUpperCase().split(/[/\s-]/, 1)[0];
  return ASSUMED_RNAV1_AIRCRAFT_PATTERN.test(aircraft) ? "1" : deriveRnavCapability(aircraftInfo, remarks);
}

export function rnavAssumptionRemark(aircraftInfo: string, remarks: string): string {
  if (effectiveRnavCapability(aircraftInfo, remarks) !== "1" || deriveRnavCapability(aircraftInfo, remarks) === "1") return "";
  return "Aircraft filed without RNAV capability, but corrected based on A/C Type. Consider verification if deemed relevant.";
}

export function effectiveClxFaults(
  strip: (Pick<FrontendStrip, "aircraft_type" | "remarks" | "clx_validation"> & Partial<Pick<FrontendStrip, "bay">>) | undefined,
): ClxValidationFault[] {
  const faults = (strip?.clx_validation?.faults ?? []).filter(
    fault => strip?.bay === undefined || isTobtRelevant(strip.bay) || fault.code !== "eobt_tobt_past",
  );
  if (!strip || effectiveRnavCapability(strip.aircraft_type, strip.remarks) !== "1") return faults;
  return faults.filter(fault => !fault.fields.includes("rnav") || fault.fields.some(field => field !== "rnav"));
}

export function deriveRnavCapability(aircraftInfo: string, remarks: string): RnavCapability {
  if (!hasEquipmentMarkerR(aircraftInfo)) return "NIL";

  const pbn = firstPbnSuffix(remarks);
  if (!pbn) return "NIL";

  if (pbn.includes("D1")) return "1";
  if (pbn.includes("C1")) return "2";
  if (pbn.includes("B1")) return "5";
  if (pbn.includes("A1")) return "10";
  return "NIL";
}

export function buildRnavUpdate(
  aircraftInfo: string,
  remarks: string,
  capability: RnavCapability,
): { aircraft_type: string; remarks: string; capabilities: RnavCapability } {
  if (capability === "NIL") {
    return {
      aircraft_type: aircraftInfo,
      remarks: replacePbnTokens(remarks, ""),
      capabilities: effectiveRnavCapability(aircraftInfo, replacePbnTokens(remarks, "")),
    };
  }

  const aircraft_type = addEquipmentMarkerR(aircraftInfo);
  const updatedRemarks = replacePbnTokens(remarks, PBN_BY_CAPABILITY[capability]);

  return {
    aircraft_type,
    remarks: updatedRemarks,
    capabilities: effectiveRnavCapability(aircraft_type, updatedRemarks),
  };
}

function hasEquipmentMarkerR(aircraftInfo: string): boolean {
  const bounds = equipmentBounds(aircraftInfo);
  if (!bounds) return false;
  return aircraftInfo.slice(bounds.start, bounds.end).toUpperCase().includes("R");
}

function addEquipmentMarkerR(aircraftInfo: string): string {
  if (hasEquipmentMarkerR(aircraftInfo)) return aircraftInfo;

  const { token, suffix } = splitAircraftInfoToken(aircraftInfo);
  if (!token) {
    return `/M-R${suffix}`;
  }

  return `${addEquipmentMarkerRToToken(token)}${suffix}`;
}

function addEquipmentMarkerRToToken(token: string): string {
  const dash = token.indexOf("-");
  if (dash >= 0) {
    let end = segmentEnd(token, dash + 1);
    if (end <= dash + 1) return token;
    if (!hasWakeTurbulenceCategoryBeforeDash(token, dash)) {
      token = `${token.slice(0, dash)}/M${token.slice(dash)}`;
      end += 2;
    }
    return `${token.slice(0, end)}R${token.slice(end)}`;
  }

  const firstSlash = token.indexOf("/");
  if (firstSlash < 0) {
    return `${token}/M-R`;
  }

  const start = firstSlash + 1;
  const end = segmentEnd(token, start);
  if (isWakeTurbulenceCategory(token.slice(start, end))) {
    return `${token.slice(0, end)}-R${token.slice(end)}`;
  }

  return `${token.slice(0, firstSlash)}/M-R${token.slice(firstSlash)}`;
}

function firstPbnSuffix(remarks: string): string {
  for (const token of remarks.split(/\s+/).filter(Boolean)) {
    if (PBN_TOKEN_PATTERN.test(token)) {
      return token.toUpperCase().replace(/^PBN\//, "");
    }
  }
  return "";
}

function replacePbnTokens(remarks: string, replacement: string): string {
  const fields = remarks.split(/\s+/).filter(Boolean);
  if (fields.length === 0) return replacement;

  const result: string[] = [];
  let replaced = false;

  for (const field of fields) {
    if (PBN_TOKEN_PATTERN.test(field)) {
      if (replacement && !replaced) {
        result.push(replacement);
        replaced = true;
      }
      continue;
    }
    result.push(field);
  }

  if (replacement && !replaced) {
    result.push(replacement);
  }

  return result.join(" ");
}

function equipmentBounds(aircraftInfo: string): { start: number; end: number } | null {
  if (!aircraftInfo) return null;

  let start = aircraftInfo.indexOf("-");
  if (start >= 0) {
    start += 1;
    const end = segmentEnd(aircraftInfo, start);
    return start < end ? { start, end } : null;
  }

  for (let slash = aircraftInfo.indexOf("/"); slash >= 0 && slash < aircraftInfo.length - 1;) {
    start = slash + 1;
    const end = segmentEnd(aircraftInfo, start);
    const segment = aircraftInfo.slice(start, end);
    if (!isWakeTurbulenceCategory(segment)) {
      return { start, end };
    }

    const next = aircraftInfo.indexOf("/", end);
    if (next < 0) break;
    slash = next;
  }

  return null;
}

function segmentEnd(value: string, start: number): number {
  for (let i = start; i < value.length; i += 1) {
    if (value[i] === "/" || value[i] === " " || value[i] === "\t") return i;
  }
  return value.length;
}

function isWakeTurbulenceCategory(segment: string): boolean {
  return ["L", "M", "H", "J"].includes(segment.toUpperCase());
}

function splitAircraftInfoToken(aircraftInfo: string): { token: string; suffix: string } {
  const trimmed = aircraftInfo.trim();
  if (!trimmed) return { token: "", suffix: "" };

  const separator = trimmed.search(/[ \t]/);
  if (separator < 0) return { token: trimmed, suffix: "" };

  return {
    token: trimmed.slice(0, separator),
    suffix: trimmed.slice(separator),
  };
}

function hasWakeTurbulenceCategoryBeforeDash(token: string, dash: number): boolean {
  if (dash <= 0) return false;

  const slash = token.slice(0, dash).lastIndexOf("/");
  if (slash < 0 || slash === dash - 1) return false;

  return isWakeTurbulenceCategory(token.slice(slash + 1, dash));
}
