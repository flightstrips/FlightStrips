import { Bay, isFlight, type AnyStrip } from "@/api/models";

export function isArrivalFlight(strip: AnyStrip, airport: string): boolean {
  return isFlight(strip) && strip.destination === airport && strip.origin !== airport;
}

/**
 * All visible workflow bays accept all strips except NOT CLEARED, which is
 * reserved for non-arrival flight strips. Ownership and validation locks are
 * enforced separately by the drag surface and backend.
 */
export function canStripMoveToBay(strip: AnyStrip | undefined, targetBay: Bay, airport: string): boolean {
  if (!strip || targetBay !== Bay.NotCleared) return true;
  return isFlight(strip) && !isArrivalFlight(strip, airport);
}

export function allBayTransferRules(bayIds: string[]): Record<string, string[]> {
  return Object.fromEntries(bayIds.map((source) => [source, bayIds.filter((target) => target !== source)]));
}
