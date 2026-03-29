import type { FrontendStrip } from "@/api/models";

export type EsetView = "MAIN" | "CARGO";
export type EsetViewButtonId = "HANGAR" | "CARGO" | "TWY_C";

interface RawStandDefinition {
  label: string;
  x: number;
  y: number;
}

export interface EsetCanvasStand extends RawStandDefinition {
  left: number;
  top: number;
}

export interface EsetBackgroundBox {
  x: number;
  y: number;
  width: number;
  height: number;
  radius?: number;
  fill: string;
  label?: string;
  labelColor?: string;
}

export interface EsetViewButton extends EsetBackgroundBox {
  id: EsetViewButtonId;
  label: string;
  disabled?: boolean;
}

export const ESET_BOARD_WIDTH = 2560;
export const ESET_BOARD_HEIGHT = 1440;
export const ESET_CELL_WIDTH = 85.3701;
export const ESET_CELL_HEIGHT = 148.313;

const RAW_STANDS: RawStandDefinition[] = [
  { label: "A18", x: 461.851, y: 229 },
  { label: "A19", x: 371.48, y: 229 },
  { label: "A20", x: 281.11, y: 229 },
  { label: "A21", x: 190.74, y: 229 },
  { label: "A22", x: 100.37, y: 229 },
  { label: "A23", x: 10, y: 229 },
  { label: "A50", x: 247, y: 556 },
  { label: "W1", x: 16, y: 1015.94 },
  { label: "RI", x: 16, y: 862.626 },
  { label: "RII", x: 16, y: 709.313 },
  { label: "RIII", x: 16, y: 556 },
  { label: "A25", x: 735.145, y: 10 },
  { label: "A26", x: 644.774, y: 10 },
  { label: "A7", x: 670.633, y: 389 },
  { label: "A4", x: 580.263, y: 388.921 },
  { label: "B4", x: 895, y: 489 },
  { label: "C27", x: 1393, y: 167 },
  { label: "D1", x: 1483.89, y: 14 },
  { label: "D2", x: 1574.26, y: 14 },
  { label: "D3", x: 1664.63, y: 14 },
  { label: "D4", x: 1755, y: 14 },
  { label: "E25", x: 1935.48, y: 14 },
  { label: "E20", x: 1844, y: 167 },
  { label: "E27", x: 1935.48, y: 167.313 },
  { label: "E22", x: 1844, y: 320.313 },
  { label: "E29", x: 1935.48, y: 320.626 },
  { label: "E24", x: 1844, y: 473.626 },
  { label: "E31", x: 1934.48, y: 473.939 },
  { label: "E26", x: 1844, y: 626.939 },
  { label: "E33", x: 1935.48, y: 627.252 },
  { label: "E35", x: 1935.48, y: 780.565 },
  { label: "H105", x: 2180, y: 320.313 },
  { label: "E90", x: 2442, y: 10 },
  { label: "E78", x: 2340, y: 10 },
  { label: "E89", x: 2442, y: 163.313 },
  { label: "E77", x: 2340, y: 163.313 },
  { label: "E88", x: 2442, y: 316.626 },
  { label: "E76", x: 2340, y: 316.626 },
  { label: "E87", x: 2442, y: 469.939 },
  { label: "E75", x: 2340, y: 469.939 },
  { label: "E86", x: 2442, y: 623.252 },
  { label: "E74", x: 2340, y: 623.252 },
  { label: "E85", x: 2442, y: 776.565 },
  { label: "E73", x: 2340, y: 776.565 },
  { label: "E84", x: 2442, y: 929.878 },
  { label: "E72", x: 2340, y: 929.878 },
  { label: "E83", x: 2442, y: 1083.19 },
  { label: "E71", x: 2340, y: 1083.19 },
  { label: "E82", x: 2441, y: 1236.5 },
  { label: "E70", x: 2339, y: 1236.5 },
  { label: "H106", x: 2180, y: 167 },
  { label: "H103", x: 2180, y: 626.939 },
  { label: "H104", x: 2180, y: 473.626 },
  { label: "H101", x: 2180.22, y: 933.565 },
  { label: "F9", x: 2089.85, y: 933.565 },
  { label: "F8", x: 1999.48, y: 933.565 },
  { label: "F7", x: 1909.11, y: 933.565 },
  { label: "F5", x: 1818.74, y: 933.565 },
  { label: "F89", x: 2059.84, y: 1101.878 },
  { label: "F91", x: 1969.47, y: 1101.878 },
  { label: "F93", x: 1879.1, y: 1101.878 },
  { label: "F95", x: 1788.73, y: 1101.878 },
  { label: "F97", x: 1698.36, y: 1101.878 },
  { label: "F90", x: 2017.48, y: 1254.878 },
  { label: "F92", x: 1927.11, y: 1254.878 },
  { label: "F94", x: 1836.74, y: 1254.878 },
  { label: "F96", x: 1746.37, y: 1254.878 },
  { label: "F98", x: 1656, y: 1254.878 },
  { label: "F4", x: 1728.37, y: 933.565 },
  { label: "F1", x: 1638, y: 933.565 },
  { label: "H102", x: 2180, y: 780.252 },
  { label: "C29", x: 1393, y: 320 },
  { label: "C33", x: 1392, y: 473 },
  { label: "C35", x: 1391, y: 626 },
  { label: "C37", x: 1391, y: 779 },
  { label: "C39", x: 1391, y: 932 },
  { label: "C26", x: 1300, y: 350 },
  { label: "C30", x: 1301, y: 503 },
  { label: "C32", x: 1301, y: 656 },
  { label: "C34", x: 1301, y: 809 },
  { label: "C36", x: 1301.24, y: 962.1 },
  { label: "B6", x: 895, y: 642 },
  { label: "B8", x: 895, y: 795 },
  { label: "B10", x: 896, y: 948 },
  { label: "B19", x: 892, y: 1101.42 },
  { label: "B16", x: 802, y: 1104 },
  { label: "B17", x: 982.76, y: 1101.42 },
  { label: "B5", x: 988, y: 593.313 },
  { label: "B3", x: 988, y: 440 },
  { label: "B15", x: 1073.13, y: 1101 },
  { label: "B7", x: 988, y: 746.626 },
  { label: "B9", x: 988, y: 899.939 },
  { label: "A9", x: 648.37, y: 543 },
  { label: "A6", x: 558, y: 543 },
  { label: "A11", x: 635.884, y: 696 },
  { label: "A8", x: 546, y: 696 },
  { label: "A15", x: 473.883, y: 849 },
  { label: "A17", x: 384, y: 849 },
  { label: "A12", x: 654.624, y: 849 },
  { label: "A14", x: 564.253, y: 849 },
  { label: "A27", x: 554.404, y: 10 },
  { label: "A28", x: 464.034, y: 10 },
  { label: "A30", x: 373.664, y: 26 },
  { label: "A31", x: 283.294, y: 55.823 },
  { label: "A32", x: 192.767, y: 56 },
  { label: "A33", x: 103, y: 56 },
  { label: "A34", x: 13, y: 56 },
];

export const ESET_STANDS: EsetCanvasStand[] = RAW_STANDS.map((stand) => ({
  ...stand,
  left: stand.x,
  top: stand.y,
}));

export const ESET_BACKGROUND_BOXES: EsetBackgroundBox[] = [
  { x: 552.221, y: 229, width: 551.699, height: 148.043, radius: 12, fill: "#959595" },
  { x: 1393.41, y: 14, width: 85.4775, height: 148, radius: 12, fill: "#959595" },
  { x: 1845.37, y: 14, width: 85.4775, height: 148, radius: 12, fill: "#959595" },
];

export const ESET_VIEW_BUTTONS: EsetViewButton[] = [
  {
    id: "HANGAR",
    x: 296,
    y: 1214,
    width: 199,
    height: 63.9131,
    radius: 12,
    fill: "#3D3D3D",
    label: "HANGAR",
    labelColor: "#FFFFFF",
    disabled: true,
  },
  {
    id: "CARGO",
    x: 514,
    y: 1214,
    width: 199,
    height: 63.9131,
    radius: 12,
    fill: "#3D3D3D",
    label: "CARGO",
    labelColor: "#FFFFFF",
  },
  {
    id: "TWY_C",
    x: 414,
    y: 1301,
    width: 199,
    height: 63.9131,
    radius: 12,
    fill: "#3D3D3D",
    label: "TWY C",
    labelColor: "#FFFFFF",
    disabled: true,
  },
];

const CARGO_STAND_PATTERN = /^(?:E(?:7\d|8\d|90)|F\d+|H\d+)$/;

export function isCargoStand(stand: string) {
  return CARGO_STAND_PATTERN.test(stand);
}

export function getEsetStandsForView(view: EsetView) {
  return view === "CARGO"
    ? ESET_STANDS.filter((stand) => isCargoStand(stand.label))
    : ESET_STANDS;
}

export function getVgdsStatus(stand: string): string | null {
  switch (stand) {
    case "A19":
    case "A14":
      return "MAX CAT D";
    case "C32":
    case "C30":
    case "C28":
      return "NO 77W/747/A346/A359";
    case "A7":
      return "MAX CAT B";
    default:
      return null;
  }
}

export function getBridgeStatus(stand: string): string | null {
  if (stand.startsWith("C")) return "NON-SCHENGEN JETWAY";
  if (stand.startsWith("D")) return "SCHENGEN + NON-SCHENGEN";
  if (stand.startsWith("A") || stand.startsWith("B")) return "SCHENGEN ONLY";
  if (stand.startsWith("F")) return "NO BRIDGE";
  if (stand.startsWith("H") || stand.startsWith("R") || stand.startsWith("W")) return "NO BRIDGE";
  if (stand.startsWith("E") && Number.parseInt(stand.slice(1), 10) >= 70) return "NO BRIDGE";
  return null;
}

function parseCompactTime(value: string) {
  if (!/^\d{4}$/.test(value)) {
    return null;
  }

  const hours = Number.parseInt(value.slice(0, 2), 10);
  const minutes = Number.parseInt(value.slice(2), 10);

  if (Number.isNaN(hours) || Number.isNaN(minutes)) {
    return null;
  }

  const now = new Date();
  const timestamp = Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
    hours,
    minutes,
  );

  return new Date(timestamp);
}

export function parseTimestamp(value: string) {
  if (!value) {
    return null;
  }

  // Try compact HHMM first — new Date("1423") parses as year 1423, not 14:23
  const compact = parseCompactTime(value);
  if (compact) {
    return compact;
  }

  const direct = new Date(value);
  if (!Number.isNaN(direct.getTime())) {
    return direct;
  }

  return null;
}

export function parseTimestampMs(value: string) {
  return parseTimestamp(value)?.getTime() ?? null;
}

export function formatTimeLabel(value: string) {
  const date = parseTimestamp(value);

  if (!date) {
    return "—";
  }

  return date.toISOString().slice(11, 16);
}

export function isStandOccupied(strip: FrontendStrip | undefined) {
  return !!strip && strip.bay !== "HIDDEN";
}
