import {type AppState, Auth0Provider} from "@auth0/auth0-react";
import { useNavigate } from "react-router-dom";
import React from "react";

/**
 * Shown when /config.js did not load or did not define the runtime config.
 * Signing in is impossible in that state, and guessing at credentials is worse
 * than saying so plainly.
 */
function AuthConfigUnavailable() {
  return (
    <div
      role="alert"
      style={{
        minHeight: "100dvh",
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: "0.75rem",
        padding: "2rem",
        textAlign: "center",
        background: "#04100f",
        color: "#fff",
        fontFamily: "IBM Plex Sans, system-ui, sans-serif",
      }}
    >
      <h1 style={{ fontSize: "1.25rem", fontWeight: 600, margin: 0 }}>
        FlightStrips is not available right now
      </h1>
      <p style={{ margin: 0, maxWidth: "38rem", color: "#8ea3a3", lineHeight: 1.6 }}>
        The application could not load its configuration, so signing in is unavailable. This is
        usually a deployment in progress — reload in a moment.
      </p>
      <button
        type="button"
        onClick={() => window.location.reload()}
        style={{
          marginTop: "0.5rem",
          padding: "0.6rem 1.25rem",
          border: "1px solid rgba(255,255,255,0.25)",
          background: "transparent",
          color: "#a0dae4",
          cursor: "pointer",
          font: "inherit",
        }}
      >
        Reload
      </button>
    </div>
  );
}

export const Auth0ProviderWithNavigate = ({ children }: React.PropsWithChildren) => {
  const navigate = useNavigate();
  const domain = "auth.flightstrips.dk";
  // No fallbacks. These used to default to the development client, audience and
  // connection, so a deployment that failed to serve config.js would quietly
  // authenticate against the wrong tenant and hand users an opaque
  // "Service not found" from Auth0. Missing config is now a visible failure.
  const clientId = window.__APP_CONFIG__?.clientId;
  const audience = window.__APP_CONFIG__?.audience;
  const connection = window.__APP_CONFIG__?.connection;
  const redirectUri = window.location.origin;

  if (!clientId || !audience || !connection) {
    return <AuthConfigUnavailable />;
  }

  const onRedirectCallback = (appState?: AppState) => {
    navigate(appState?.returnTo || window.location.pathname);
  };

  if (!(domain && clientId && redirectUri)) {
    return null;
  }

  return (
    <Auth0Provider
      domain={domain}
      clientId={clientId}
      authorizationParams={{
        redirect_uri: redirectUri,
        scope: "openid profile email offline_access",
        audience: audience,
        connection: connection
      }}
      onRedirectCallback={onRedirectCallback}
      cacheLocation="localstorage"
      useRefreshTokens={true}
    >
      {children}
    </Auth0Provider>
  );
};