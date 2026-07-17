import { useEffect, useState } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import { getApiUrl } from "@/lib/api-url";

export function useTestToolsAvailability() {
  const { getAccessTokenSilently, isAuthenticated } = useAuth0();
  const [available, setAvailable] = useState(false);

  useEffect(() => {
    if (!isAuthenticated) {
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const token = await getAccessTokenSilently();
        const response = await fetch(getApiUrl("/api/test/status"), {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!cancelled) {
          setAvailable(response.ok);
        }
      } catch {
        if (!cancelled) {
          setAvailable(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [getAccessTokenSilently, isAuthenticated]);

  return isAuthenticated && available;
}
