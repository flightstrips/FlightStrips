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
