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
