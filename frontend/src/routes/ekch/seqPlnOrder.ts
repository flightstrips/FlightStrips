import type { AnyStrip } from "@/api/models";
import { orderStripsByTsat } from "@/lib/tsatOrder";

/** The earliest TSAT sits nearest the command bar. A live TSAT update reorders the bay. */
export function orderSeqPlnStartup(strips: AnyStrip[], now = new Date()): AnyStrip[] {
  return orderStripsByTsat(strips, "descending", now);
}
