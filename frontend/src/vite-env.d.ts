/// <reference types="vite/client" />

interface AppConfig {
  wsUrl?: string;
  clientId?: string;
  audience?: string;
  connection?: string;
}

interface Window {
  __APP_CONFIG__?: AppConfig;
}
