import type {AMANHoldingEntry} from "@/api/aman";

/** Figma-preferred order for the five established EKCH holding cards. */
export const EKCH_TMT_HOLDING_ORDER: readonly string[] = ["TIDVU", "OLPIB", "LUGAS", "ROSBI", "ERNOV"];

/** Keep the reference order without hiding holdings introduced by authoritative data. */
export function orderedEKCHTMTHoldings(entries: readonly Pick<AMANHoldingEntry, "holding">[]): string[] {
  const observed = [...new Set(entries.map(({holding}) => holding))];
  return [
    ...EKCH_TMT_HOLDING_ORDER,
    ...observed.filter((holding) => !EKCH_TMT_HOLDING_ORDER.includes(holding)),
  ];
}
