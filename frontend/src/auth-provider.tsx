import {type AppState, Auth0Provider} from "@auth0/auth0-react";
import { useNavigate } from "react-router-dom";
import React from "react";

export const Auth0ProviderWithNavigate = ({ children }: React.PropsWithChildren) => {
  const navigate = useNavigate();
  const domain = "auth.flightstrips.dk";
  const clientId = "mIjRYlbKHpTwnNAkhcu9plQP541Klwvn";
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
        audience: "backend-dev"
      }}
      onRedirectCallback={onRedirectCallback}
      cacheLocation="localstorage"
      useRefreshTokens={true}
    >
      {children}
    </Auth0Provider>
  );
};