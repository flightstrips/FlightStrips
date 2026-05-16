/// <reference types="vite/client" />
/// <reference types="vite-plugin-pwa/client" />

interface AppConfig {
  deploymentVersion?: string;
  wsUrl?: string;
  apiBaseUrl?: string;
  clientId?: string;
  audience?: string;
  connection?: string;
}

interface Window {
  __APP_CONFIG__?: AppConfig;
}

declare const __APP_VERSION__: string;
