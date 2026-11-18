import { useEffect } from "react";
import { useLocation } from "react-router";
import { startAppUpdateMonitoring } from "@/lib/app-update";

const OPERATIONAL_PATHS = [
  "/app",
  "/dashboard",
  "/pilot",
  "/efb",
  "/cdm",
  "/stand",
  "/aman-replay",
  "/test",
];

/** Keep offline-cache downloads and automatic deployment reloads off public pages. */
export function AppUpdateMonitoring() {
  const { pathname } = useLocation();
  useEffect(() => {
    if (
      OPERATIONAL_PATHS.some(
        (path) => pathname === path || pathname.startsWith(`${path}/`),
      )
    ) {
      startAppUpdateMonitoring();
    }
  }, [pathname]);
  return null;
}
