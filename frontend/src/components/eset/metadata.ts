import type { FrontendStrip } from "@/api/models";

export type StandSection = "NORTH" | "OTHER";

interface RawStandDefinition {
  label: string;
  x: number;
  y: number;
  section: StandSection;
}

export interface EsetStandDefinition extends RawStandDefinition {
  column: number;
  row: number;
}

export interface EsetCanvasStand extends RawStandDefinition {
  left: number;
  top: number;
}

export interface EsetSectionLayout {
  columns: number;
  rows: number;
  stands: EsetStandDefinition[];
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

const POSITION_TOLERANCE = 16;

export const ESET_BOARD_WIDTH = 2560;
export const ESET_BOARD_HEIGHT = 1365;
export const ESET_CELL_WIDTH = 74;
export const ESET_CELL_HEIGHT = 128;
export const ESET_CELL_GAP = 8;
export const ESET_CELL_OFFSET_X = 24;
export const ESET_CELL_OFFSET_Y = 39;

const RAW_STANDS: RawStandDefinition[] = [
  { label: "A4", x: 820.0, y: 219.5, section: "NORTH" },
  { label: "A6", x: 820.0, y: 354.3, section: "NORTH" },
  { label: "A7", x: 977.6, y: 219.5, section: "NORTH" },
  { label: "A8", x: 820.0, y: 489.1, section: "NORTH" },
  { label: "A9", x: 977.6, y: 354.3, section: "NORTH" },
  { label: "A11", x: 971.8, y: 489.1, section: "NORTH" },
  { label: "A12", x: 971.8, y: 623.8, section: "NORTH" },
  { label: "A14", x: 893.0, y: 623.8, section: "NORTH" },
  { label: "A15", x: 814.2, y: 623.8, section: "NORTH" },
  { label: "A17", x: 735.4, y: 623.8, section: "NORTH" },
  { label: "A18", x: 736.6, y: 71.9, section: "NORTH" },
  { label: "A19", x: 657.8, y: 71.9, section: "NORTH" },
  { label: "A20", x: 579.6, y: 71.9, section: "NORTH" },
  { label: "A21", x: 500.6, y: 71.9, section: "NORTH" },
  { label: "A22", x: 421.6, y: 70.9, section: "NORTH" },
  { label: "A25", x: 342.6, y: 71.9, section: "NORTH" },
  { label: "A26", x: 263.8, y: 71.9, section: "NORTH" },
  { label: "A27", x: 185.0, y: 71.9, section: "NORTH" },
  { label: "A28", x: 185.0, y: 206.7, section: "NORTH" },
  { label: "A30", x: 185.0, y: 341.5, section: "NORTH" },
  { label: "A31", x: 185.0, y: 476.2, section: "NORTH" },
  { label: "A32", x: 185.0, y: 611.0, section: "NORTH" },
  { label: "A33", x: 106.2, y: 611.0, section: "NORTH" },
  { label: "A34", x: 26.1, y: 611.0, section: "NORTH" },
  { label: "A50", x: 474.1, y: 399.9, section: "NORTH" },
  { label: "B4", x: 1135.5, y: 219.5, section: "NORTH" },
  { label: "B6", x: 1135.5, y: 354.3, section: "NORTH" },
  { label: "B7", x: 1213.3, y: 354.3, section: "NORTH" },
  { label: "B8", x: 1135.5, y: 489.1, section: "NORTH" },
  { label: "B9", x: 1213.3, y: 489.1, section: "NORTH" },
  { label: "B10", x: 1129.8, y: 623.8, section: "NORTH" },
  { label: "B15", x: 1286.4, y: 623.8, section: "NORTH" },
  { label: "B17", x: 1207.5, y: 623.8, section: "NORTH" },
  { label: "B19", x: 1207.5, y: 754.9, section: "NORTH" },
  { label: "C27", x: 1572.9, y: 219.5, section: "OTHER" },
  { label: "C28", x: 1493.0, y: 218.2, section: "OTHER" },
  { label: "C29", x: 1572.9, y: 350.2, section: "OTHER" },
  { label: "C30", x: 1493.0, y: 348.8, section: "OTHER" },
  { label: "C32", x: 1493.0, y: 479.5, section: "OTHER" },
  { label: "C33", x: 1572.9, y: 480.8, section: "OTHER" },
  { label: "C34", x: 1493.0, y: 610.1, section: "OTHER" },
  { label: "C35", x: 1572.9, y: 611.5, section: "OTHER" },
  { label: "C36", x: 1493.0, y: 740.8, section: "OTHER" },
  { label: "C37", x: 1572.9, y: 742.2, section: "OTHER" },
  { label: "C39", x: 1572.9, y: 872.9, section: "OTHER" },
  { label: "D1", x: 1655.2, y: 71.9, section: "OTHER" },
  { label: "D2", x: 1734.0, y: 71.9, section: "OTHER" },
  { label: "D3", x: 1812.8, y: 71.9, section: "OTHER" },
  { label: "D4", x: 1891.7, y: 71.9, section: "OTHER" },
  { label: "E20", x: 1972.2, y: 222.3, section: "OTHER" },
  { label: "E22", x: 1972.2, y: 353.0, section: "OTHER" },
  { label: "E24", x: 1972.2, y: 483.7, section: "OTHER" },
  { label: "E25", x: 2156.7, y: 78.8, section: "OTHER" },
  { label: "E27", x: 2156.7, y: 209.5, section: "OTHER" },
  { label: "E29", x: 2156.7, y: 340.2, section: "OTHER" },
  { label: "E31", x: 2156.7, y: 470.9, section: "OTHER" },
  { label: "E35", x: 2156.7, y: 601.6, section: "OTHER" },
  { label: "E36", x: 1972.2, y: 614.4, section: "OTHER" },
  { label: "E37", x: 2156.7, y: 732.3, section: "OTHER" },
  { label: "E70", x: 2402.8, y: 1120.8, section: "OTHER" },
  { label: "E71", x: 2402.8, y: 990.1, section: "OTHER" },
  { label: "E72", x: 2402.8, y: 859.4, section: "OTHER" },
  { label: "E73", x: 2402.8, y: 728.6, section: "OTHER" },
  { label: "E74", x: 2402.8, y: 597.9, section: "OTHER" },
  { label: "E75", x: 2402.8, y: 467.2, section: "OTHER" },
  { label: "E76", x: 2402.8, y: 336.5, section: "OTHER" },
  { label: "E77", x: 2402.8, y: 205.8, section: "OTHER" },
  { label: "E78", x: 2402.8, y: 75.1, section: "OTHER" },
  { label: "E82", x: 2478.5, y: 1120.8, section: "OTHER" },
  { label: "E83", x: 2478.5, y: 990.1, section: "OTHER" },
  { label: "E84", x: 2478.5, y: 859.4, section: "OTHER" },
  { label: "E85", x: 2478.5, y: 728.6, section: "OTHER" },
  { label: "E86", x: 2478.5, y: 597.9, section: "OTHER" },
  { label: "E87", x: 2478.5, y: 467.2, section: "OTHER" },
  { label: "E88", x: 2478.5, y: 336.5, section: "OTHER" },
  { label: "E89", x: 2478.5, y: 205.8, section: "OTHER" },
  { label: "E90", x: 2478.5, y: 75.1, section: "OTHER" },
  { label: "F1", x: 1900.1, y: 862.5, section: "OTHER" },
  { label: "F4", x: 1976.8, y: 862.5, section: "OTHER" },
  { label: "F5", x: 2053.5, y: 862.5, section: "OTHER" },
  { label: "F7", x: 2130.2, y: 862.5, section: "OTHER" },
  { label: "F8", x: 2207.1, y: 862.5, section: "OTHER" },
  { label: "F89", x: 2147.9, y: 1116.7, section: "OTHER" },
  { label: "F90", x: 2115.3, y: 1255.6, section: "OTHER" },
  { label: "F91", x: 2071.0, y: 1116.7, section: "OTHER" },
  { label: "F92", x: 2038.5, y: 1255.6, section: "OTHER" },
  { label: "F93", x: 1995.4, y: 1116.7, section: "OTHER" },
  { label: "F94", x: 1961.8, y: 1255.6, section: "OTHER" },
  { label: "F95", x: 1919.7, y: 1116.7, section: "OTHER" },
  { label: "F96", x: 1885.0, y: 1255.6, section: "OTHER" },
  { label: "F97", x: 1844.1, y: 1116.7, section: "OTHER" },
  { label: "F98", x: 1808.3, y: 1255.6, section: "OTHER" },
];

function clusterPositions(values: number[]) {
  const clusters: Array<{ center: number; values: number[] }> = [];

  for (const value of [...values].sort((left, right) => left - right)) {
    const cluster = clusters[clusters.length - 1];

    if (!cluster || Math.abs(value - cluster.center) > POSITION_TOLERANCE) {
      clusters.push({ center: value, values: [value] });
      continue;
    }

    cluster.values.push(value);
    cluster.center = cluster.values.reduce((sum, current) => sum + current, 0) / cluster.values.length;
  }

  return clusters.map((cluster) => cluster.center);
}

function resolveClusterIndex(centers: number[], value: number) {
  return centers.findIndex((center) => Math.abs(center - value) <= POSITION_TOLERANCE);
}

function createSectionLayout(section: StandSection): EsetSectionLayout {
  const stands = RAW_STANDS.filter((stand) => stand.section === section);
  const xClusters = clusterPositions(stands.map((stand) => stand.x));
  const yClusters = clusterPositions(stands.map((stand) => stand.y));

  return {
    columns: xClusters.length,
    rows: yClusters.length,
    stands: stands
      .map((stand) => ({
        ...stand,
        column: resolveClusterIndex(xClusters, stand.x) + 1,
        row: resolveClusterIndex(yClusters, stand.y) + 1,
      }))
      .sort((left, right) => left.row - right.row || left.column - right.column),
  };
}

export const ESET_SECTION_LAYOUTS: Record<StandSection, EsetSectionLayout> = {
  NORTH: createSectionLayout("NORTH"),
  OTHER: createSectionLayout("OTHER"),
};

export const ESET_STANDS: EsetCanvasStand[] = RAW_STANDS.map((stand) => ({
  ...stand,
  left: stand.x - ESET_CELL_OFFSET_X,
  top: stand.y - ESET_CELL_OFFSET_Y,
}));

export const ESET_BACKGROUND_BOXES: EsetBackgroundBox[] = [
  { x: 798.303, y: 33, width: 385.249, height: 128.348, radius: 12, fill: "#959595" },
  { x: 1474.55, y: 33, width: 150.227, height: 128.348, radius: 12, fill: "#959595" },
  { x: 1950.44, y: 35.7378, width: 78.7903, height: 128.348, radius: 12, fill: "#959595" },
  { x: 2035.54, y: 35.7378, width: 95.5989, height: 680.302, radius: 12, fill: "#959595" },
  { x: 1875.33, y: 755, width: 227, height: 61.3021, radius: 12, fill: "#959595" },
  { x: 949, y: 1144, width: 199, height: 63.9131, radius: 12, fill: "#3D3D3D", label: "GA", labelColor: "#FFFFFF" },
  { x: 1167, y: 1144, width: 199, height: 63.9131, radius: 12, fill: "#3D3D3D", label: "CARGO", labelColor: "#FFFFFF" },
];

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

  const direct = new Date(value);
  if (!Number.isNaN(direct.getTime())) {
    return direct;
  }

  return parseCompactTime(value);
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
