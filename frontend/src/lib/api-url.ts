export function getConfiguredApiBaseUrl(): string {
  const configuredBaseUrl = window.__APP_CONFIG__?.apiBaseUrl?.trim();
  if (configuredBaseUrl) {
    return configuredBaseUrl.replace(/\/$/, "");
  }

  const configuredWsUrl = window.__APP_CONFIG__?.wsUrl?.trim();
  if (configuredWsUrl) {
    try {
      const wsUrl = new URL(configuredWsUrl, window.location.origin);
      const httpProtocol = wsUrl.protocol === "wss:" ? "https:" : "http:";
      return `${httpProtocol}//${wsUrl.host}`;
    } catch {
      // Fall back to same-origin requests when the configured WebSocket URL is invalid.
    }
  }

  return window.location.origin.replace(/\/$/, "");
}

export function getApiUrl(path: string): string {
  return `${getConfiguredApiBaseUrl()}${path}`;
}
