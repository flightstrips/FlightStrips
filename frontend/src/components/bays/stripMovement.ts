import { Bay, isFlight, type AnyStrip } from "@/api/models";

export function isArrivalFlight(strip: AnyStrip, airport: string): boolean {
  return isFlight(strip) && strip.destination === airport && strip.origin !== airport;
}

/**
 * Non-cleared strips may only move between visual bays that map to NOT CLEARED.
 * All other visible workflow bays accept all strips except NOT CLEARED, which
 * is reserved for non-arrival flight strips. Ownership and validation locks are
 * enforced separately by the drag surface and backend.
 */
export function canStripMoveToBay(strip: AnyStrip | undefined, targetBay: Bay, airport: string): boolean {
  if (!strip) return true;
  if (targetBay === Bay.NotCleared) return isFlight(strip) && !isArrivalFlight(strip, airport);
  return !isFlight(strip) || strip.bay !== Bay.NotCleared;
}

export function allBayTransferRules(bayIds: string[]): Record<string, string[]> {
  return Object.fromEntries(bayIds.map((source) => [source, bayIds.filter((target) => target !== source)]));
}
