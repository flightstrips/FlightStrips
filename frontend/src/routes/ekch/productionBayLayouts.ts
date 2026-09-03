export const EQUAL_COLUMN_CLASSES = ["bay-col-flex", "bay-col-flex", "bay-col-flex", "bay-col-flex"] as const;
export const TOWER_COLUMN_CLASSES = ["w-1/4 bay-col", "w-[29%] bay-col", "w-1/4 bay-col", "w-[21%] bay-col"] as const;
export const GEGW_COLUMN_CLASSES = ["w-[27%] bay-col", "w-[28%] bay-col", "w-1/4 bay-col", "w-1/5 bay-col"] as const;

export const PRODUCTION_BAY_CLASS = {
  towerFinal: "h-[35%] bay-scroll-area-bottom",
  towerRunwayArrival: "h-[20%] bay-scroll-area-dark",
  towerTaxiDeparture: "h-[35%] bay-scroll-area-bottom",
  towerControlZoneWithStartup: "h-[21.65%] bay-scroll-area-bottom",
  towerClearanceDelivery: "h-[45%] bay-scroll-area",
  gegwPushback: "h-[12%] bay-scroll-area-bottom",
  gegwStartup: "h-[33%] bay-scroll-area-bottom",
  aaFinal: "h-[30%] bay-scroll-area-bottom",
  aaRunwayArrival: "h-[25%] bay-scroll-area-bottom",
  aaTaxiDepartureUpper: "h-[25%] bay-scroll-area-bottom",
  adMessages: "h-[15%] bay-scroll-area",
  clxCleared: "h-[calc(67%-2.5rem)] bay-scroll-area",
  clxPushback: "h-2/5 bay-scroll-area-bottom",
} as const;
