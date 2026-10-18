import { Bay, type FrontendStrip, type TacticalStrip } from "@/api/models";

export function hasOtherRwyArrStrip(
  callsign: string,
  strips: readonly Pick<FrontendStrip, "callsign" | "bay">[],
  tacticalStrips: readonly Pick<TacticalStrip, "bay">[],
): boolean {
  return strips.some(strip => strip.callsign !== callsign && strip.bay === Bay.RwyArr) ||
    tacticalStrips.some(strip => strip.bay === Bay.RwyArr);
}
