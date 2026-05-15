import { useEffect } from "react";
import { buildVacsWsUrl, isVacsIntegrationEnabled } from "@/lib/vacs-settings";
import { getVacsClient } from "@/vacs/vacs-client";

export function VacsProvider({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const client = getVacsClient();

    const sync = () => {
      queueMicrotask(() => {
        client.updateUrl(buildVacsWsUrl());
        if (isVacsIntegrationEnabled()) {
          client.start();
        } else {
          client.stop();
        }
      });
    };

    sync();
    const onSettingsChange = () => sync();
    window.addEventListener("vacs-settings-changed", onSettingsChange);
    return () => {
      window.removeEventListener("vacs-settings-changed", onSettingsChange);
      client.stop();
    };
  }, []);

  return children;
}
