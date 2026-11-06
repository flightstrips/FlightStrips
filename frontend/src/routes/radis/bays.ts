export const RADIS_BAYS = ["ACTIVE", "RUNWAY", "PASSIVE_TOP", "PASSIVE_BOTTOM"] as const;
export type RADISBay = typeof RADIS_BAYS[number];
