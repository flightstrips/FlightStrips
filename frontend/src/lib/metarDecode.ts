import { parseMetar, CloudQuantity, type IMetar } from "metar-taf-parser";

/** Flight category from visibility (km) and ceiling (ft). */
export type FlightCategory = "VFR" | "MVFR" | "IFR" | "LIFR" | "UNKN";

const FLIGHT_CATEGORY_LABELS: Record<FlightCategory, string> = {
  VFR: "VFR — Visual Flight Rules",
  MVFR: "MVFR — Marginal VFR",
  IFR: "IFR — Instrument Flight Rules",
  LIFR: "LIFR — Low IFR",
  UNKN: "UNKN — Incomplete or expired data",
};

/**
 * Derive flight category from visibility (km) and ceiling (ft).
 * Uses the standard table: VFR >8 km / >3000 ft, MVFR 5–8 km / 1000–3000 ft,
 * IFR 1.5–5 km / 500–1000 ft, LIFR <1.5 km / <500 ft, UNKN if data missing.
 */
export function getFlightCategory(visibilityKm: number | null, ceilingFt: number | null): FlightCategory {
  if (visibilityKm == null || ceilingFt == null) return "UNKN";
  if (visibilityKm < 1.5 || ceilingFt < 500) return "LIFR";
  if (visibilityKm < 5 || ceilingFt < 1000) return "IFR";
  if (visibilityKm < 8 || ceilingFt < 3000) return "MVFR";
  return "VFR";
}

export function getFlightCategoryLabel(cat: FlightCategory): string {
  return FLIGHT_CATEGORY_LABELS[cat];
}

/** Parsed METAR fields for display (visibility in km, ceiling in ft, wind in kts/hdg). */
export interface MetarDecoded {
  raw: string;
  flightCategory: FlightCategory;
  flightCategoryLabel: string;
  temperature: number | null;
  dewPoint: number | null;
  windSpeedKts: number | null;
  windDegrees: number | null;
  windDirection: string | null;
  visibilityKm: number | null;
  visibilityDisplay: string;
  ceilingFt: number | null;
  qnh: number | null;
  qnhUnit: "hPa" | "inHg";
  /** Cloud/weather summary for choosing an icon: clear, few, sct, bkn, ovc, + precip/fog etc. */
  condition: "clear" | "few" | "sct" | "bkn" | "ovc" | "fg" | "precip" | "thunderstorm";
  parsed: IMetar | null;
}

function visibilityToKm(vis: IMetar["visibility"]): number | null {
  if (!vis) return null;
  const v = vis.value;
  if (vis.unit === "m") return v / 1000;
  if (vis.unit === "SM") return v * 1.60934;
  return v / 1000;
}

function visibilityDisplay(vis: IMetar["visibility"]): string {
  if (!vis) return "—";
  if (vis.unit === "m") {
    // In METAR, 9999m is effectively "10 km or more"
    if (vis.value >= 9999) return "≥10 km";
    return `${(vis.value / 1000).toFixed(1)} km`;
  }
  return `${vis.value} SM`;
}

/** Lowest BKN or OVC layer height in feet; parser returns height in feet. */
function ceilingFt(clouds: IMetar["clouds"]): number | null {
  if (!clouds?.length) return null;
  const ceilingLayers = clouds.filter(
    (c) => c.quantity === CloudQuantity.BKN || c.quantity === CloudQuantity.OVC
  );
  if (ceilingLayers.length === 0) return null;
  const heights = ceilingLayers.map((c) => c.height).filter((h): h is number => h != null);
  if (heights.length === 0) return null;
  return Math.min(...heights);
}

/** Summarise clouds/weather for icon: clear, few, sct, bkn, ovc, fg, precip, thunderstorm. */
function condition(parsed: IMetar): MetarDecoded["condition"] {
  const wx = parsed.weatherConditions ?? [];
  const hasFg = wx.some((w) => w.phenomenons.some((p) => p === "FG" || p === "BR"));
  const hasTs = wx.some((w) => w.phenomenons.some((p) => p === "TS"));
  const hasPrecip = wx.some((w) =>
    w.phenomenons.some((p) => ["RA", "SN", "DZ", "PL", "GR", "GS", "SG", "IC", "UP"].includes(p))
  );
  if (hasTs) return "thunderstorm";
  if (hasFg) return "fg";
  if (hasPrecip) return "precip";

  const clouds = parsed.clouds ?? [];
  const quantities = clouds.map((c) => c.quantity);
  if (quantities.includes(CloudQuantity.OVC)) return "ovc";
  if (quantities.includes(CloudQuantity.BKN)) return "bkn";
  if (quantities.includes(CloudQuantity.SCT)) return "sct";
  if (quantities.includes(CloudQuantity.FEW)) return "few";
  if (parsed.cavok) return "clear";
  return "clear";
}

export function decodeMetar(raw: string | null | undefined): MetarDecoded {
  const empty: MetarDecoded = {
    raw: raw ?? "",
    flightCategory: "UNKN",
    flightCategoryLabel: getFlightCategoryLabel("UNKN"),
    temperature: null,
    dewPoint: null,
    windSpeedKts: null,
    windDegrees: null,
    windDirection: null,
    visibilityKm: null,
    visibilityDisplay: "—",
    ceilingFt: null,
    qnh: null,
    qnhUnit: "hPa",
    condition: "clear",
    parsed: null,
  };

  if (!raw?.trim()) return empty;

  let parsed: IMetar;
  try {
    parsed = parseMetar(raw);
  } catch {
    return { ...empty, raw };
  }

  const visKm = visibilityToKm(parsed.visibility);
  const ceiling = ceilingFt(parsed.clouds);
  const flightCategory = getFlightCategory(visKm, ceiling);

  let qnh: number | null = null;
  let qnhUnit: "hPa" | "inHg" = "hPa";
  if (parsed.altimeter) {
    qnh = parsed.altimeter.value;
    qnhUnit = parsed.altimeter.unit === "inHg" ? "inHg" : "hPa";
  }

  return {
    raw,
    flightCategory,
    flightCategoryLabel: getFlightCategoryLabel(flightCategory),
    temperature: parsed.temperature ?? null,
    dewPoint: parsed.dewPoint ?? null,
    windSpeedKts: parsed.wind?.speed ?? null,
    windDegrees: parsed.wind?.degrees ?? null,
    windDirection: parsed.wind?.direction ?? null,
    visibilityKm: visKm,
    visibilityDisplay: visibilityDisplay(parsed.visibility),
    ceilingFt: ceiling,
    qnh,
    qnhUnit,
    condition: condition(parsed),
    parsed,
  };
}
