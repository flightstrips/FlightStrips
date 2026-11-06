export interface Atis {
  callsign: string;
  code: string;
  text: string[];
  frequency: string;
  last_updated: string;
  stale: boolean;
}

export function parseAtisWeather(text: string) {
  const normalized = text.toUpperCase().replace(/\s+/g, ' ');
  const wind = normalized.match(/\bWIND\s+(VRB|VARIABLE|[0-3]\d{2})(?:\s*(?:DEG(?:REES)?|\/))?\s+(\d{1,3})(?:\s*G(?:UST(?:ING)?)?\s*(\d{1,3}))?\s*(KT|KTS|MPS)\b/)
    ?? normalized.match(/\bWIND\s+(VRB|VARIABLE|[0-3]\d{2})\s*\/\s*(\d{1,3})(?:G(\d{1,3}))?\s*(KT|KTS|MPS)\b/)
    ?? normalized.match(/\b(VRB|[0-3]\d{2})(\d{2,3})(?:G(\d{2,3}))?(KT|MPS)\b/)
    ?? normalized.match(/\bWIND\s+(CALM)\b/);
  const direction = wind?.[1];
  const temperature = normalized.match(/\b(?:TEMPERATURE|TEMP|T)\s+(MINUS\s+|M|-)?(\d{1,2})(?!\d)/)
    ?? normalized.match(/\b(M?)(\d{2})\/(?:M?\d{2}|\/\/)(?=\s|$)/);
  const qnh = normalized.match(/\bQNH\s*(\d{4})(?!\d)/) ?? normalized.match(/\bQ(\d{4})\b/);
  return {
    windDirection: direction === 'VARIABLE' ? 'VRB' : direction && (direction === 'VRB' || direction === 'CALM' || Number(direction) <= 360) ? direction : null,
    windStrength: wind?.[2] ? `${Number(wind[2])}${wind[3] ? `G${Number(wind[3])}` : ''} ${wind[4] === 'MPS' ? 'MPS' : 'KT'}` : direction === 'CALM' ? 'CALM' : null,
    temperature: temperature ? Number(temperature[2]) * (temperature[1] ? -1 : 1) : null,
    qnh: qnh ? Number(qnh[1]) : null,
  };
}

// The caller supplies its authenticated backend fetch. Reading this cache
// never downloads or indexes the global VATSIM feed in a browser.
export async function fetchCachedAtis(airport: string, departure: boolean, request: (path: string, init?: RequestInit) => Promise<unknown>, signal?: AbortSignal): Promise<Atis> {
  const query = new URLSearchParams({ airport: airport.trim().toUpperCase(), departure: String(departure) });
  const entry = await request(`/api/weather/atis?${query}`, { signal });
  if (!entry || typeof entry !== "object" || !("callsign" in entry) || typeof entry.callsign !== "string"
    || !("code" in entry) || typeof entry.code !== "string" || !/^[A-Z]$/.test(entry.code)
    || !("text" in entry) || !Array.isArray(entry.text) || !entry.text.every((line: unknown) => typeof line === "string")
    || !("frequency" in entry) || typeof entry.frequency !== "string"
    || !("last_updated" in entry) || typeof entry.last_updated !== "string"
    || !("stale" in entry) || typeof entry.stale !== "boolean") throw new Error("Invalid cached ATIS observation");
  return entry as Atis;
}
