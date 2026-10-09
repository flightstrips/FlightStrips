import type { TacticalStrip } from "@/api/models";

const DEICE_LANE_PREFIX = "DEICE_LANE:";
const DEICE_HEADER_PREFIX = "DEICE_HEADER:";

export type DeiceHeaderArea = "A" | "B" | "TWRGND";

export function buildDeiceLaneLabel(area: "A" | "B", lane: number, frequency: string) {
  return `${DEICE_HEADER_PREFIX}${area}:LANE ${lane} \u2013 ${frequency}MHz`;
}

export function buildDeicePlatformLabel(platform: "A" | "B" | "V") {
  return `${DEICE_HEADER_PREFIX}TWRGND:DE-ICE ${platform}`;
}

export function isDeiceHeaderTacticalStrip(strip: TacticalStrip) {
  return strip.type === "MEMAID"
    && (strip.label.startsWith(DEICE_HEADER_PREFIX) || strip.label.startsWith(DEICE_LANE_PREFIX));
}

export function getDeiceHeaderArea(strip: TacticalStrip): DeiceHeaderArea | null {
  if (strip.type !== "MEMAID") return null;
  if (strip.label.startsWith(DEICE_LANE_PREFIX)) return "A";
  if (!strip.label.startsWith(DEICE_HEADER_PREFIX)) return null;

  const area = strip.label.slice(DEICE_HEADER_PREFIX.length).split(":", 1)[0];
  return area === "A" || area === "B" || area === "TWRGND" ? area : null;
}

export function getDeiceHeaderDisplayLabel(strip: TacticalStrip) {
  if (strip.label.startsWith(DEICE_LANE_PREFIX)) {
    return strip.label.slice(DEICE_LANE_PREFIX.length);
  }

  const areaSeparator = strip.label.indexOf(":", DEICE_HEADER_PREFIX.length);
  return areaSeparator === -1 ? strip.label : strip.label.slice(areaSeparator + 1);
}
