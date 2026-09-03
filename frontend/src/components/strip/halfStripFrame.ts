import type { HalfStripVariant } from "./types";
import {
  COLOR_ARR_YELLOW,
  COLOR_BTN_BLUE,
  COLOR_DEP_STRIP_BG,
  getStripFrameColor,
} from "./shared";

export const HALF_STRIP_VARIANT_BG: Record<HalfStripVariant, string> = {
  "APN-PUSH": "var(--color-strip-push-bg)",
  "APN-ARR": COLOR_ARR_YELLOW,
  "LOCKED-DEP": COLOR_DEP_STRIP_BG,
  "LOCKED-ARR": COLOR_ARR_YELLOW,
  "MESSAGES": "#285A5C",
  "MEM-AID": COLOR_BTN_BLUE,
  "LAND-START": "#DD6A12",
  "CROSSING": "#FCC800",
};

const FREE_TEXT_VARIANTS = new Set<HalfStripVariant>([
  "MESSAGES",
  "MEM-AID",
  "LAND-START",
  "CROSSING",
]);

export function getHalfStripFrameColor(variant: HalfStripVariant, arrival: boolean): string {
  return FREE_TEXT_VARIANTS.has(variant)
    ? HALF_STRIP_VARIANT_BG[variant]
    : getStripFrameColor(arrival);
}
