import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function getSimpleAircraftType(actype: string | null | undefined) {
  return actype?.split("/")[0]
}