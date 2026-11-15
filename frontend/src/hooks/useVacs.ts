import { useMemo, useSyncExternalStore } from "react";
import { toast } from "sonner";
import { useWebSocketStore } from "@/store/store-hooks";
import type { VacsActions, VacsState } from "@/vacs/types";

export function useVacs(): { state: VacsState; actions: VacsActions } {
  const client = useWebSocketStore(state => state.vacsClient);
  const state = useSyncExternalStore(client.subscribe, client.getState);
  const actions = useMemo(() => {
    const report = async (action: () => Promise<void>) => {
      try { await action(); }
      catch (error) { toast.error(error instanceof Error ? error.message : "VACS action failed"); throw error; }
    };
    return {
      acceptCall: (id: string) => report(() => client.acceptCall(id)),
      rejectCall: (id: string) => report(() => client.rejectCall(id)),
      endCall: (id: string) => report(() => client.endCall(id)),
      dialClient: (peer: Parameters<VacsActions["dialClient"]>[0]) => report(() => client.dialClient(peer)),
    };
  }, [client]);
  return {state, actions};
}
