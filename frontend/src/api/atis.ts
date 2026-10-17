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

export async function fetchVatsimAtis(airport: string, departure: boolean, signal?: AbortSignal): Promise<Atis> {
  const callsign = `${airport.trim().toUpperCase()}_${departure ? 'D' : 'A'}_ATIS`;
  const response = await fetch('https://data.vatsim.net/v3/afv-atis-data.json', { signal });
  if (!response.ok) throw new Error(`ATIS feed unavailable (${response.status})`);
  const entries: unknown = await response.json();
  if (!Array.isArray(entries)) throw new Error('Invalid ATIS feed');
  const entry: unknown = entries.find((value: unknown) => (
    typeof value === 'object' && value !== null && 'callsign' in value && value.callsign === callsign
  ));
  if (!entry) throw new Error(`${callsign} is offline`);
  if (typeof entry !== 'object' || !('atis_code' in entry) || typeof entry.atis_code !== 'string'
    || !/^[A-Z]$/.test(entry.atis_code) || !('text_atis' in entry) || !Array.isArray(entry.text_atis)
    || entry.text_atis.length === 0 || !entry.text_atis.every((line: unknown) => typeof line === 'string')
    || !('frequency' in entry) || typeof entry.frequency !== 'string'
    || !('last_updated' in entry) || typeof entry.last_updated !== 'string'
    || !Number.isFinite(Date.parse(entry.last_updated))) {
    throw new Error(`Invalid ATIS data for ${callsign}`);
  }
  return {
    callsign, code: entry.atis_code, text: entry.text_atis, frequency: entry.frequency,
    last_updated: entry.last_updated, stale: Date.now() - Date.parse(entry.last_updated) > 5 * 60_000,
  };
}
