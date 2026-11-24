export const DEICE_INDICATION_BG = "#131376";
export const DEICE_INDICATION_FG = "#FFFFFF";

const DEICE_PLATFORMS = new Set(["A", "B", "V"]);

export function normalizeDeIcePlatform(platform: string | undefined) {
  const value = platform?.trim().toUpperCase() ?? "";
  return DEICE_PLATFORMS.has(value) ? value : "";
}

export function formatDeIceIndication(platform: string | undefined) {
  const value = normalizeDeIcePlatform(platform);
  return value ? `DE-ICE ${value}` : "";
}

export const DEICE_DONE_LABEL = "DONE";

// Strip label: once the platform is acknowledged (strip moved into the de-ice
// bay) every platform reads DONE instead of DE-ICE A/B/V.
export function formatStripDeIceIndication(platform: string | undefined, acknowledged: boolean) {
  if (!normalizeDeIcePlatform(platform)) return "";
  return acknowledged ? DEICE_DONE_LABEL : formatDeIceIndication(platform);
}
