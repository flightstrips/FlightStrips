import { useRef, useState } from "react";
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
  isSigningIn: boolean;
  signInError: string | null;
  signIn: () => Promise<void>;
  signOut: () => void;
};

export function useLandingAuth(): LandingAuth {
  const { isAuthenticated, loginWithRedirect, logout } = useAuth0();

  const [isSigningIn, setIsSigningIn] = useState(false);
  const [signInError, setSignInError] = useState<string | null>(null);
  const redirectPending = useRef(false);

  async function signIn() {
    if (redirectPending.current) return;
    redirectPending.current = true;
    setIsSigningIn(true);
    setSignInError(null);

    try {
      await loginWithRedirect({ appState: { returnTo: "/app" } });
      // Keep the action pending until the browser leaves for sign-in.
    } catch {
      redirectPending.current = false;
      setIsSigningIn(false);
      setSignInError("Could not open VATSIM sign-in. Check your connection and try again.");
    }
  }

  return {
    isAuthenticated,
    isSigningIn,
    signInError,
    signIn,
    signOut: () => logout({ logoutParams: { returnTo: window.location.origin } }),
  };
}
