import { useAuth0 } from "@auth0/auth0-react";

/**
 * The landing page's only tie to the application.
 *
 * When the public site is split out, this is the file that changes: a
 * standalone marketing site has no Auth0 provider and would return
 * `{ isAuthenticated: false, signIn: () => location.assign(APP_URL) }`.
 * Nothing else in this folder imports Auth0.
 */
export type LandingAuth = {
  isAuthenticated: boolean;
  signIn: () => void;
  signOut: () => void;
};

export function useLandingAuth(): LandingAuth {
  const { isAuthenticated, loginWithRedirect, logout } = useAuth0();

  return {
    isAuthenticated,
    signIn: () => void loginWithRedirect({ appState: { returnTo: "/app" } }),
    signOut: () => logout({ logoutParams: { returnTo: window.location.origin } }),
  };
}
