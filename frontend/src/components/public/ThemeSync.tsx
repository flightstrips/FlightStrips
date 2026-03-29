import { useLocation } from "react-router";
import { useEffect } from "react";
import {
  getStoredPublicTheme,
  applyPublicThemeToDocument,
  clearPublicThemeFromDocument,
} from "@/lib/public-theme";

const SELECTABLE_PUBLIC_PATHS = new Set(["/", "/about", "/contact"]);

/**
 * Syncs document theme with stored public theme. On /app we always remove dark
 * so the app UI is never affected by the public site theme toggle.
 */
export function ThemeSync() {
  const { pathname } = useLocation();

  useEffect(() => {
    const root = document.getElementById("root");
    const isSelectablePublicPage = SELECTABLE_PUBLIC_PATHS.has(pathname);

    document.documentElement.classList.toggle("public-content-page", isSelectablePublicPage);
    document.body.classList.toggle("public-content-page", isSelectablePublicPage);
    root?.classList.toggle("public-content-page", isSelectablePublicPage);

    if (pathname.startsWith("/app")) {
      clearPublicThemeFromDocument();
    } else {
      const theme = getStoredPublicTheme();
      applyPublicThemeToDocument(theme);
    }
  }, [pathname]);

  return null;
}
