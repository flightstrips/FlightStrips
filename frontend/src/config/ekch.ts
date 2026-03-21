/**
 * Operational context evaluated at render time to drive per-point visibility
 * and dynamic labels on map dialogs.
 */
export interface VisibilityContext {
  /** Active departure runways, e.g. ["22L", "22R"] */
  dep: string[];
  /** Active arrival runways, e.g. ["04L"] */
  arr: string[];
  /** Whether the strip that opened this dialog is a departure or arrival. */
  stripType: "dep" | "arr" | undefined;
  apronOnline: boolean;
  twrOnline: boolean;
  isTwr: boolean;
}

type VisibleFn = (ctx: VisibilityContext) => boolean;

/** Show when `rwy` is active as either departure or arrival. */
export const onRwy = (rwy: string): VisibleFn => (ctx) => ctx.dep.includes(rwy) || ctx.arr.includes(rwy);
/** Show when `rwy` is an active departure runway. */
export const onDep = (rwy: string): VisibleFn => (ctx) => ctx.dep.includes(rwy);
/** Show when `rwy` is an active arrival runway. */
export const onArr = (rwy: string): VisibleFn => (ctx) => ctx.arr.includes(rwy);
/** Show only for departure strips. */
export const isDep: VisibleFn = (ctx) => ctx.stripType === "dep";
/** Show only for arrival strips. */
export const isArr: VisibleFn = (ctx) => ctx.stripType === "arr";
/** Show when all provided conditions are true. */
export const and = (...fns: VisibleFn[]): VisibleFn => (ctx) => fns.every((f) => f(ctx));
/** Show when at least one condition is true. */
export const or  = (...fns: VisibleFn[]): VisibleFn => (ctx) => fns.some((f) => f(ctx));
/** Invert a condition. */
export const not: (fn: VisibleFn) => VisibleFn = (fn) => (ctx) => !fn(ctx);

/** Show when a separate apron controller is online. */
export const isApronOnline: VisibleFn = (ctx) => ctx.apronOnline;
/** Show when a separate TWR controller is online. */
export const isTwrOnline: VisibleFn = (ctx) => ctx.twrOnline;
/** Show when this position is TWR with no separate apron (solo TWR). */
export const isSoloTwr: VisibleFn = (ctx) => ctx.isTwr && !ctx.apronOnline;

export interface ClickPoint {
  /**
   * Static string, or a function that returns the label from operational context.
   * Drives both the button display text and the value sent to setReleasePoint.
   */
  label: string | ((ctx: VisibilityContext) => string);
  /** Percentage from left edge of the map container */
  left: string;
  /** Percentage from top edge of the map container */
  top: string;

  width?: string;
  height?: string;

  /**
   * Point classification for the TWR taxi map dialog.
   *   "hp" — runway / ILS holding point (persisted as release_point)
   *   "cl" — clearance limit / taxiway intersection (local UI state only)
   * Omitted type defaults to "cl".
   */
  type?: "hp" | "cl";

  /**
   * Optional visibility predicate. When absent the point is always shown.
   * When present, the point is only rendered when the predicate returns true.
   * Requires a visibilityContext to be passed to MapDialogShell.
   */
  visible?: (ctx: VisibilityContext) => boolean;
}

/**
 * EKCH pushback release points positioned over apron_push.png.
 * Positions are percentages relative to the image content area (1920×768).
 * Derived from SVG layout: (cx - 140) / 1920 and (cy - 140) / 768.
 * One unknown label is marked TODO — identify from the map and replace.
 */
export const RELEASE_POINTS: ClickPoint[] = [
  { label: "K1",  left: "2.89%",  top: "39.71%" },
  { label: "Z1",  left: "7.19%",  top: "35.29%" },
  { label: "Z2",  left: "9.77%",  top: "39.19%" },
  { label: "J1",  left: "12.52%", top: "33.85%" },
  { label: "Z3",  left: "13.00%", top: "43.62%" },
  { label: "J2",  left: "13.88%", top: "28.58%" },
  { label: "Z4",  left: "16.07%", top: "46.98%" },
  { label: "J3",  left: "20.08%", top: "24.93%" },
  { label: "Z5",  left: "22.11%", top: "46.09%" },
  { label: "A50", left: "24.87%", top: "32.68%" },
  { label: "J4",  left: "25.55%", top: "26.04%" },
  { label: "L3",  left: "26.30%", top: "39.32%" },
  { label: "L2",  left: "28.70%", top: "34.11%" },
  { label: "Y0",  left: "29.48%", top: "44.92%" },
  { label: "Z6",  left: "31.56%", top: "56.90%" },
  { label: "Y1",  left: "32.99%", top: "49.87%" },
  { label: "Z7",  left: "36.98%", top: "64.06%" },
  { label: "M1",  left: "37.68%", top: "44.14%" },
  { label: "Y2",  left: "38.41%", top: "56.90%" },
  { label: "Z8", left: "40.42%", top: "69.66%" },
  { label: "Y3",  left: "43.30%", top: "64.97%" },
  { label: "P2",  left: "46.54%", top: "57.68%" },
  { label: "P1",  left: "46.54%", top: "46.09%" },
  { label: "Q1",  left: "49.58%", top: "47.92%" },
  { label: "Q2",  left: "50.29%", top: "63.15%" },
  { label: "Y4",  left: "55.68%", top: "82.16%" },
  { label: "Z9",  left: "55.68%", top: "91.80%" },
  { label: "R1",  left: "60.31%", top: "47.92%" },
  { label: "R2",  left: "60.68%", top: "60.81%" },
  { label: "R3",  left: "61.41%", top: "72.79%" },
  { label: "R4",  left: "61.41%", top: "78.26%" },
  { label: "B",  left: "61.00%", top: "95.00%" },
  { label: "S1",  left: "63.88%", top: "47.92%" },
  { label: "S2",  left: "64.48%", top: "60.81%" },
  { label: "S3",  left: "64.94%", top: "72.79%" },
  { label: "S4",  left: "65.23%", top: "78.26%" },
  { label: "V3",   left: "64.17%", top: "84.25%" },
  { label: "W2",  left: "68.59%", top: "66.06%" },
  { label: "V4",  left: "70.65%", top: "83.14%" },
  { label: "U1",  left: "77.40%", top: "43.36%" },
  { label: "W3",  left: "77.71%", top: "63.93%" },
  { label: "F89",  left: "80.42%", top: "70.59%" },
  { label: "U2",  left: "81.51%", top: "34.70%" },
  { label: "T1",  left: "85.49%", top: "49.09%" },
  { label: "T2",  left: "86.20%", top: "37.76%" },
  { label: "T3",  left: "89.03%", top: "29.04%" },
  { label: "V5", left: "90.94%", top: "50.85%" },
  { label: "T4",  left: "92.32%", top: "18.23%" },
  { label: "T5",  left: "95.14%", top: "9.70%"  },
];

/**
 * EKCH apron taxi points positioned over apron_taxi.png.
 * Positions are percentages relative to the image content area (1859×903).
 * Derived from SVG layout: (cx - 40) / 1859 and (cy - 40) / 903.
 * Labels are placeholders (TD001–TD032) — replace with actual taxiway names.
 */
export const APRON_TAXI_POINTS: ClickPoint[] = [
  { label: "K/J", left: "8.42%",  top: "30.40%" },
  { label: "K2", left: "16.54%", top: "43.63%", type: "cl" },
  { label: "K/L", left: "21.71%", top: "37.76%" },
  { label: "K3", left: "23.75%", top: "51.05%", type: "cl" },
  { label: "Z/L", left: "27.25%", top: "43.30%" },
  { label: "L/Y", left: "27.62%", top: "28.90%" },
  { label: "Y/L", left: "29.67%", top: "37.65%" },
  { label: "Z/M", left: "33.06%", top: "50.06%" },
  { label: "Y/M", left: "35.69%", top: "44.63%" },
  { label: "Z/A", left: "38.33%", top: "56.81%" },
  { label: "Y/A", left: "40.32%", top: "50.72%" },
  { label: "A", left: "40.91%", top: "69.21%" },
  { label: "Z/F", left: "44.94%", top: "63.57%" },
  { label: "P/Y", left: "45.54%", top: "44.85%" },
  { label: "F", left: "45.54%", top: "72.87%" },
  { label: "Z/D", left: "50.91%", top: "70.54%" },
  { label: "D", left: "50.91%", top: "78.52%" },
  { label: "Q/Y", left: "51.51%", top: "53.27%" },
  { label: "Y/V", left: "55.86%", top: "68.49%" },
  { label: "B", left: "58.66%", top: "86.93%" },
  { label: "R/W", left: "60.44%", top: "55.59%" },
  { label: "R/V", left: "60.97%", top: "65.67%" },
  { label: "S/W", left: "65.30%", top: "55.48%" },
  { label: "S/V", left: "65.44%", top: "65.67%" },
  { label: "DE-ICE B", left: "67.05%", top: "86.16%", width: "115px" },
  { label: "W/S", left: "70.60%", top: "54.37%" },
  { label: "V/S", left: "70.60%", top: "68.11%" },
  { label: "DE-ICE V", left: "73.13%", top: "76.97%", width: "115px" },
  { label: "V2", left: "84.75%", top: "65.56%", type: "cl" },
  { label: "T/S", left: "86.20%", top: "43.74%" },
  { label: "V/T", left: "86.74%", top: "51.83%" },
  { label: "V1", left: "96.26%", top: "35.22%", type: "cl" },
];

/**
 * EKCH TWR taxi map points positioned over taxi_map.png.
 * Positions are percentages relative to the image content area (1801×1013).
 * Derived from SVG layout: (cx - 40) / 1801 and (cy - 40) / 1013.
 *
 * type "cl" — clearance limit / taxiway intersection
 * type "hp" — runway / ILS holding point
 * Omitted type defaults to "cl".
 *
 * --- Visibility & dynamic label examples ---
 *
 * Show only when RWY 22R is active (dep or arr):
 *   visible: onRwy("22R")
 *
 * Show only when RWY 22L is an active arrival runway:
 *   visible: onArr("22L")
 *
 * Show only during mixed 12+22L ops:
 *   visible: and(onRwy("12"), onRwy("22L"))
 *
 * Show only for arrival strips:
 *   visible: isArr
 *
 * Show only for arrival strips on RWY 22L:
 *   visible: and(onArr("22L"), isArr)
 *
 * Hide apron taxiway points when apron is staffed separately:
 *   visible: not(isApronOnline)
 *
 * Show only when TWR is working solo (no separate apron):
 *   visible: isSoloTwr
 *
 * Show when either runway is active:
 *   visible: or(onRwy("22L"), onRwy("22R"))
 *
 * Dynamic label — helpers work as plain booleans inside label functions:
 *   label: (ctx) => onRwy("04R")(ctx) ? "C/04R" : "C/22L"
 *   label: (ctx) => ctx.dep[0] ? `K3/${ctx.dep[0]}` : "K3"
 */
export const TAXI_MAP_POINTS: ClickPoint[] = [
  { label: "K1/K",   left: "13.16%", top: "5.38%",  type: "cl" },
  { label: "A5",     left: "18.98%", top: "96.84%", type: "hp", visible: onDep("22R") },
  { label: "K2/K",   left: "20.12%", top: "10.73%", type: "cl" },
  { label: "K2/12",  left: "20.12%", top: "17.81%", type: "cl" },
  { label: "K3/Z",   left: "26.13%", top: "13.58%", type: "cl" },
  { label: "A4",     left: "26.13%", top: "84.76%", type: "hp", visible: onDep("22R") },
  { label: "K3/12",  left: "26.83%", top: "22.74%", type: "cl" },
  { label: "Y/L",    left: "31.44%", top: "9.62%",  type: "cl", visible: not(isApronOnline) },
  { label: "Y/M",    left: "38.83%", top: "17.82%", type: "cl", visible: not(isApronOnline) },
  { label: "P/Y",    left: "44.57%", top: "16.49%", type: "cl", visible: not(isApronOnline) },
  { label: "Q/Y",    left: "47.03%", top: "21.49%", type: "cl", visible: not(isApronOnline) },
  { label: "Y/Q",    left: "49.49%", top: "27.67%", type: "cl", visible: not(isApronOnline) },
  { label: "Y/V",    left: "52.38%", top: "32.47%", type: "cl", visible: not(isApronOnline) },
  { label: "R/V",    left: "55.62%", top: "27.33%", type: "cl", visible: not(isApronOnline) },
  { label: "S/V",    left: "60.19%", top: "27.39%", type: "cl", visible: not(isApronOnline) },
  { label: "W/S",    left: "61.87%", top: "22.05%", type: "cl", visible: not(isApronOnline) },
  { label: "DEICE-B", left: "60.62%", top: "42.81%", width: "80px", visible: and(not(isApronOnline), isDep) },
  { label: "LINE 3", left: "27.03%", top: "47.74%", width: "80px" },
  { label: "LINE 2", left: "27.03%", top: "51.91%", width: "80px" },
  { label: "LINE 1", left: "27.03%", top: "56.08%", width: "80px" },
  { label: "A3",     left: "28.79%", top: "78.92%", type: "hp", visible: onDep("22R") },
  { label: "A2",     left: "31.44%", top: "73.51%", type: "hp", visible: onDep("22R") },
  { label: "F2/30",  left: "32.42%", top: "35.38%", type: "cl" },
  { label: "A/A1",   left: "33.88%", top: "62.14%" },
  { label: "A1",     left: "34.29%", top: "67.47%", type: "hp", visible: onDep("22R") },
  { label: "A/D",    left: "35.82%", top: "52.05%" },
  { label: "B1",     left: "36.29%", top: "97.24%", type: "hp", visible: onDep("04R") },
  { label: "B2",     left: "38.67%", top: "92.69%", type: "hp", visible: onDep("04R") },
  { label: "E1",     left: "38.83%", top: "76.01%", type: "hp", visible: onDep("22R") },
  { label: "D/A",    left: "39.22%", top: "57.05%" },
  { label: (ctx) => isDep(ctx) ? "A/30" : "A/Z", left: "40.00%", top: "28.37%", type: "cl" },
  { label: "C/30",   left: "40.04%", top: "44.86%", type: "cl" },
  { label: "B3",     left: "40.67%", top: "88.10%", type: "hp", visible: onDep("04R") },
  { label: "30/A",   left: "41.95%", top: "39.27%" },
  { label: "B/C",    left: "42.26%", top: "80.60%", visible: onArr("22L") },
  { label: "C/D",    left: "43.12%", top: "62.05%" },
  { label: (ctx) => isDep(ctx) ?"F/30" : "F/Z", left: "43.90%", top: "32.47%", type: "cl" },
  { label: "D/30",   left: "43.90%", top: "50.52%", type: "cl" },
  { label: "30/D",   left: "46.56%", top: "44.20%" },
  { label: "B/C",    left: "46.83%", top: "67.05%" },
  { label: (ctx) => onRwy("04R")(ctx) ? "C/04R" : "C/22L",  left: "47.54%", top: "91.26%", type: "cl" },
  { label: "B4",     left: "47.56%", top: "97.29%", type: "hp", visible: onDep("04R") },
  { label: (ctx) => isDep(ctx) ? "D/30" : "D/Z",    left: "47.81%", top: "37.47%" },
  { label: "B/30",   left: "50.43%", top: "55.72%", type: "cl" },
  { label: (ctx) => isDep(ctx) ? "B/30" : "B/Z", left: "55.00%", top: "43.51%" },
  { label: "30/B",   left: "55.82%", top: "52.81%" },
  { label: "V/S",    left: "62.30%", top: "32.53%" },
  { label: (ctx) => onRwy("04R")(ctx) ? "12/04R" : "12/22L", left: "62.34%", top: "59.90%", width: "80px", type: "cl" },
  { label: (ctx) => onRwy("04R")(ctx) ? "30/04R" : "30/22L", left: "70.03%", top: "68.16%", width: "80px", type: "cl" },
  { label: "V2",     left: "75.62%", top: "30.38%", type: "hp" },
  { label: "N2/30",  left: "76.36%", top: "84.06%", type: "hp" },
  { label: (ctx) => onRwy("04R")(ctx) ? "I/04R" : "I/22L",  left: "79.84%", top: "46.35%", type: "hp" },
  { label: "G2/30",  left: "81.13%", top: "68.16%", type: "hp" },
  { label: "V1",     left: "84.33%", top: "8.58%",  type: "hp" },
  { label: "G1",     left: "97.61%", top: "87.60%", type: "hp" },
];

/**
 * EKCH runway holding points, organized per runway, for the HoldingPointDialog.
 * Image dimensions and button positions derived from the design-doc SVG.
 * Each position is a percentage of the runway image dimensions.
 */
export interface HoldingPointRunway {
  runway: string;
  imageSrc: string;
  imgWidth: number;
  imgHeight: number;
  points: ClickPoint[];
}

export const HOLDING_POINT_RUNWAYS: HoldingPointRunway[] = [
  {
    runway: "04L",
    imageSrc: "/holding_points/04L.png",
    imgWidth: 1281,
    imgHeight: 284,
    points: [
      { label: "A10", left: "37.7%", top: "40.7%" },
      { label: "A9",  left: "90.4%", top: "40.7%" },
    ],
  },
  {
    runway: "04R",
    imageSrc: "/holding_points/04R.png",
    imgWidth: 1281,
    imgHeight: 245,
    points: [
      { label: "B1", left: "7.5%",  top: "42.3%" },
      { label: "B2", left: "16.4%", top: "42.3%" },
      { label: "B3", left: "41.3%", top: "42.3%" },
      { label: "B4", left: "85.5%", top: "42.3%" },
      { label: "C",  left: "95.8%", top: "42.3%" },
    ],
  },
  {
    runway: "12",
    imageSrc: "/holding_points/12.png",
    imgWidth: 1267,
    imgHeight: 334,
    points: [
      { label: "K1", left: "4.4%",  top: "45.1%" },
      { label: "K2", left: "28.8%", top: "45.1%" },
      { label: "K3", left: "40.5%", top: "37.3%" },
      { label: "D",  left: "91.9%", top: "37.0%" },
      { label: "F1", left: "5.3%",  top: "85.2%" },
      { label: "F2", left: "61.8%", top: "87.9%" },
    ],
  },
  {
    runway: "22L",
    imageSrc: "/holding_points/22L.png",
    imgWidth: 1281,
    imgHeight: 423,
    points: [
      { label: "V2", left: "53.5%", top: "27.5%" },
      { label: "V1", left: "92.4%", top: "27.5%" },
      { label: "I",  left: "41.6%", top: "89.0%" },
    ],
  },
  {
    runway: "22R",
    imageSrc: "/holding_points/22R.png",
    imgWidth: 1271,
    imgHeight: 423,
    points: [
      { label: "A4", left: "14.2%", top: "30.1%" },
      { label: "A3", left: "30.4%", top: "30.1%" },
      { label: "A2", left: "45.6%", top: "30.1%" },
      { label: "A1", left: "59.6%", top: "30.1%" },
      { label: "E1", left: "51.4%", top: "91.1%" },
    ],
  },
  {
    runway: "30",
    imageSrc: "/holding_points/30.png",
    imgWidth: 1267,
    imgHeight: 392,
    points: [
      { label: "G2", left: "16.2%", top: "43.2%" },
      { label: "G1", left: "82.0%", top: "43.2%" },
      { label: "N2", left: "18.4%", top: "88.7%" },
    ],
  },
];
