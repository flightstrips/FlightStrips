/// <reference types="vite/client" />

interface AppConfig {
  wsUrl?: string;
}

interface Window {
  __APP_CONFIG__?: AppConfig;
}
