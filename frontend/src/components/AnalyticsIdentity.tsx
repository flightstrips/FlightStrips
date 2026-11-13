import { useEffect } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import { cidFromSubject, identifyAnalyticsUser, onConsentChanged, resetAnalyticsUser } from "@/lib/analytics";

/** Links analytics sessions to the logged-in VATSIM user so reported issues can be looked up by CID. */
export function AnalyticsIdentity() {
  const { isAuthenticated, isLoading, user } = useAuth0();
  const sub = user?.sub;

  useEffect(() => {
    if (isLoading) return;
    if (!isAuthenticated || !sub) {
      resetAnalyticsUser();
      return;
    }
    const identify = () => identifyAnalyticsUser({ id: sub, cid: cidFromSubject(sub) });
    identify();
    // Identify immediately when consent is granted later in the session.
    return onConsentChanged((consent) => {
      if (consent.status === "granted") identify();
    });
  }, [isAuthenticated, isLoading, sub]);

  return null;
}
