import { useEffect } from "react";
import { Outlet, useLocation } from "react-router";
import { useAuth0 } from "@auth0/auth0-react";
import { PublicFooter } from "@/components/public/PublicFooter";
import { PublicNavigation } from "@/components/public/PublicNavigation";
import { cn } from "@/lib/utils";
import { PUBLIC_NAV_INDUSTRY_CLASS, PUBLIC_PAGE_SHELL_CLASS } from "@/lib/public-page-style";

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
      <div
        className={cn(
          PUBLIC_PAGE_SHELL_CLASS,
          "items-center justify-center text-sm font-medium text-neutral-600 dark:text-neutral-400",
        )}
      >
        Loading pilot tools…
      </div>
    );
  }

  return (
    <div className={PUBLIC_PAGE_SHELL_CLASS}>
      <PublicNavigation linkTone="industrial" className={PUBLIC_NAV_INDUSTRY_CLASS} />
      <main className="mx-auto flex w-full max-w-[1400px] flex-1 flex-col px-6 pb-16 pt-28 sm:px-10">
        <Outlet />
      </main>
      <PublicFooter tone="industrial" />
    </div>
  );
}
