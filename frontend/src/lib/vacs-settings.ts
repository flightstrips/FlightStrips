const VACS_ENABLED_KEY = "flightstrips.vacs.enabled";

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
