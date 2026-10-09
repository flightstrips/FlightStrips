import type { StripRef } from "@/api/models";
import { useWebSocketStore } from "@/store/store-hooks.ts";

const PLATFORM_BY_BAY_ID: Record<string, string> = {
  "DE-ICE-B": "B",
  "DE-ICE-V": "V",
};

/**
 * DE-ICE B and DE-ICE V share one backend bay and are split by the strip's de-ice platform,
 * so dropping a strip into one of them must also assign that platform.
 */
export function useDeiceBayDrop() {
  const cdmDeicePlatformUpdate = useWebSocketStore((s) => s.cdmDeicePlatformUpdate);
  const strips = useWebSocketStore((s) => s.strips);

  return (ref: StripRef, targetBayId: string) => {
    const platform = PLATFORM_BY_BAY_ID[targetBayId];
    if (!platform || ref.kind !== "flight" || !ref.callsign) return;

    const strip = strips.find((s) => s.callsign === ref.callsign);
    const current = strip?.deice_platform?.trim().toUpperCase() ?? "";
    // The B bay also shows platform A strips; only reassign when the strip would otherwise appear in the other bay.
    if (current === platform || (platform === "B" && current === "A")) return;

    cdmDeicePlatformUpdate(ref.callsign, platform);
  };
}
