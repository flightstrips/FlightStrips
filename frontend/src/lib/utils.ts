import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function getSimpleAircraftType(actype: string | null | undefined) {
  return actype?.split("/")[0]
}

export function getAircraftTypeWithWtc(actype: string | null | undefined, wtc: string | null | undefined) {
  const simple = getSimpleAircraftType(actype);
  return wtc ? `${simple}/${wtc}` : simple;
}

/** Format an altitude (feet) for display using the airport transition altitude.
 *  Values above the transition altitude are shown as FL (e.g. FL70);
 *  values at or below are shown as feet (e.g. 4000). */
export function formatAltitude(feet: number, transitionAltitude: number): string {
  return feet > transitionAltitude
    ? `FL${Math.floor(feet / 100)}`
    : String(feet);
}
