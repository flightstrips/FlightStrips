import { Bay } from "@/api/models";

interface RwyArrRunwayColorOptions {
  runwayCleared: boolean;
  runwayConfirmed: boolean;
}

export function getRwyArrRunwayColor({
  runwayCleared,
  runwayConfirmed,
}: RwyArrRunwayColorOptions): string | undefined {
  if (!runwayCleared) return undefined;
  return runwayConfirmed ? "var(--color-runway-confirmed)" : "var(--color-runway-closed)";
}

export function canConfirmRwyArrRunway(
  callsign: string,
  strips: ReadonlyArray<{ callsign: string; bay: string; runway_cleared: boolean; runway_confirmed: boolean }>,
): boolean {
  return !strips.some(strip =>
    strip.callsign !== callsign &&
    strip.bay === Bay.RwyArr &&
    strip.runway_cleared &&
    strip.runway_confirmed
  );
}
