import {type AppState, Auth0Provider} from "@auth0/auth0-react";
import { useNavigate } from "react-router-dom";
import React from "react";

export const Auth0ProviderWithNavigate = ({ children }: React.PropsWithChildren) => {
  const navigate = useNavigate();
  const domain = "auth.flightstrips.dk";
  const clientId = window.__APP_CONFIG__?.clientId ?? "mIjRYlbKHpTwnNAkhcu9plQP541Klwvn";
  const audience = window.__APP_CONFIG__?.audience ?? "backend-dev";
  const connection = window.__APP_CONFIG__?.connection ?? "vatsim-dev";
  const redirectUri = window.location.origin;

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