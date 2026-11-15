import { useEffect } from "react";
import { useVacsSettings } from "@/hooks/useVacsSettings";
import { useWebSocketStore } from "@/store/store-hooks";

export function VacsProvider({ children }: { children: React.ReactNode }) {
  const { vacsEnabled } = useVacsSettings();
  const client = useWebSocketStore(state => state.vacsClient);
  const ready = useWebSocketStore(state => state.ownPluginReady && !state.readOnly);
  useEffect(() => {
    if (vacsEnabled && ready) client.start();
    else client.stop();
    return () => client.stop();
  }, [client, ready, vacsEnabled]);
  return children;
}
