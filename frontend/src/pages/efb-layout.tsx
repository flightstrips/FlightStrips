import { useEffect } from "react";
import { useAuth0 } from "@auth0/auth0-react";
import { Outlet, useLocation } from "react-router";

export default function EfbLayout() {
  const { isAuthenticated, isLoading, loginWithRedirect } = useAuth0();
  const location = useLocation();

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      void loginWithRedirect({ appState: { returnTo: location.pathname + location.search } });
    }
  }, [isAuthenticated, isLoading, location.pathname, location.search, loginWithRedirect]);

  if (isLoading || !isAuthenticated) {
    return <div className="flex min-h-screen items-center justify-center bg-[#1d293d] font-mono text-cyan-100">LOADING EFB…</div>;
  }
  return <Outlet />;
}
