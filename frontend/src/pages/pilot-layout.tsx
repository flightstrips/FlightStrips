import { useEffect } from "react";
import { Outlet, useLocation } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { PublicFooter } from "@/components/public/PublicFooter";
import { PublicNavigation } from "@/components/public/PublicNavigation";

export default function PilotLayout() {
  const { isAuthenticated, isLoading, loginWithRedirect } = useAuth0();
  const location = useLocation();

  useEffect(() => {
    if (isLoading || isAuthenticated) {
      return;
    }

    void loginWithRedirect({
      appState: {
        returnTo: location.pathname + location.search,
      },
    });
  }, [isAuthenticated, isLoading, location.pathname, location.search, loginWithRedirect]);

  if (isLoading || !isAuthenticated) {
    return (
      <div className="min-h-svh bg-cream dark:bg-background text-navy dark:text-foreground flex items-center justify-center text-lg font-medium">
        Loading pilot tools...
      </div>
    );
  }

  return (
    <div className="min-h-svh bg-cream dark:bg-background text-navy dark:text-foreground flex flex-col">
      <PublicNavigation />
      <main className="mx-auto flex w-full max-w-5xl flex-1 flex-col px-4 pb-12 pt-28 sm:px-6 lg:px-8">
        <Outlet />
      </main>
      <PublicFooter />
    </div>
  );
}
