import { useCallback } from "react";
import { getVacsClient } from "@/vacs/vacs-client";
import { useVacsStore } from "@/vacs/vacs-store";
import type { VacsActions, VacsState } from "@/vacs/types";

export function useVacs(): { state: VacsState; actions: VacsActions } {
  const state = useVacsStore((s) => s.state);
  const client = getVacsClient();

  const acceptCall = useCallback(
    (callId: string) => client.acceptCall(callId),
    [client],
  );
  const rejectCall = useCallback(
    (callId: string) => client.rejectCall(callId),
    [client],
  );
  const endCall = useCallback(
    (callId: string) => client.endCall(callId),
    [client],
  );
  const dial = useCallback(
    (targetCid: string) => client.dial(targetCid),
    [client],
  );
  const dialByPosition = useCallback(
    (position: string) => client.dialByPosition(position),
    [client],
  );

  return {
    state,
    actions: { acceptCall, rejectCall, endCall, dial, dialByPosition },
  };
}
