import {type AppState, Auth0Provider} from "@auth0/auth0-react";
import { useNavigate } from "react-router-dom";
import React from "react";

export const Auth0ProviderWithNavigate = ({ children }: React.PropsWithChildren) => {
  const navigate = useNavigate();
  const domain = "dev-xd0uf4sd1v27r8tg.eu.auth0.com";
  const clientId = "DL0v1w0GCPmGImJ3Ia3giz9SiLfH28EW";
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
      }}
      onRedirectCallback={onRedirectCallback}
      cacheLocation="localstorage"
      useRefreshTokens={true}
    >
      {children}
    </Auth0Provider>
  );
};