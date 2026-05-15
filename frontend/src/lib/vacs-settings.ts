const VACS_ENABLED_KEY = "flightstrips.vacs.enabled";
const VACS_HOST_KEY = "flightstrips.vacs.host";
const VACS_WS_PORT = 9600;

export function isVacsIntegrationEnabled(): boolean {
  const stored = localStorage.getItem(VACS_ENABLED_KEY);
  if (stored === null) {
    return false;
  }
  return stored === "true";
}

export function setVacsIntegrationEnabled(enabled: boolean): void {
  localStorage.setItem(VACS_ENABLED_KEY, String(enabled));
}

export function getVacsHost(): string {
  const stored = localStorage.getItem(VACS_HOST_KEY);
  return stored?.trim() ?? "";
}

/** Strip ws(s) URLs and port/path so only hostname or IP is stored. */
export function normalizeVacsHostInput(raw: string): string {
  let value = raw.trim();
  if (!value) {
    return "";
  }

  value = value.replace(/^wss?:\/\//i, "");
  const slash = value.indexOf("/");
  if (slash >= 0) {
    value = value.slice(0, slash);
  }

  if (value.startsWith("[")) {
    const end = value.indexOf("]");
    if (end >= 0) {
      value = value.slice(0, end + 1);
    }
  } else {
    const colon = value.lastIndexOf(":");
    if (colon > 0 && value.indexOf(":") === colon) {
      const after = value.slice(colon + 1);
      if (/^\d+$/.test(after)) {
        value = value.slice(0, colon);
      }
    }
  }

  return value.replace(/[\s/\\]/g, "");
}

export function setVacsHost(value: string): void {
  const normalized = normalizeVacsHostInput(value);
  if (normalized) {
    localStorage.setItem(VACS_HOST_KEY, normalized);
  } else {
    localStorage.removeItem(VACS_HOST_KEY);
  }
}

export function buildVacsWsUrl(host?: string): string {
  const h = (host ?? getVacsHost()).trim();
  const hostname = h.length > 0 ? h : "localhost";
  return `ws://${hostname}:${VACS_WS_PORT}/ws`;
}
