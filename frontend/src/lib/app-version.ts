const configuredVersion = typeof __APP_VERSION__ === "string" ? __APP_VERSION__.trim() : "";

export const FRONTEND_VERSION = configuredVersion || "0.0.0";
