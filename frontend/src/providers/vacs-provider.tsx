import { useEffect } from "react";
import { buildResolvedVacsWsUrl } from "@/lib/vacs-settings";
import { useVacsSettings } from "@/hooks/useVacsSettings";
import { useLocalIp } from "@/store/store-hooks";
import { getVacsClient } from "@/vacs/vacs-client";

export function VacsProvider({ children }: { children: React.ReactNode }) {
  const { vacsEnabled, vacsHost } = useVacsSettings();
  const localIp = useLocalIp();

  useEffect(() => {
    const client = getVacsClient();

    client.updateUrl(buildResolvedVacsWsUrl(localIp));
    if (vacsEnabled) {
      client.start();
    } else {
      client.stop();
    }

    return () => {
      client.stop();
    };
  }, [localIp, vacsEnabled, vacsHost]);

  return children;
}
