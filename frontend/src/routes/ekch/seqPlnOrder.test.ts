import { describe, expect, it } from "vitest";
import { Bay, isFlight, type FrontendStrip } from "@/api/models";
import { orderSeqPlnStartup } from "./seqPlnOrder";

function cleared(callsign: string, tsat: string, sequence: number): FrontendStrip {
  return { callsign, tsat, sequence, bay: Bay.Cleared } as FrontendStrip;
}

describe("SEQ PLN startup order", () => {
  it("places the earliest TSAT at the bottom and updates when a TSAT changes", () => {
    const now = new Date("2026-09-28T12:00:00Z");
    const strips = [cleared("LATE", "1215", 1), cleared("EARLY", "1205", 2), cleared("NO-TIME", "", 3)];
    expect(orderSeqPlnStartup(strips, now).filter(isFlight).map(strip => strip.callsign)).toEqual(["NO-TIME", "LATE", "EARLY"]);

    const updated = [strips[0], { ...strips[1], tsat: "1220" }, strips[2]];
    expect(orderSeqPlnStartup(updated, now).filter(isFlight).map(strip => strip.callsign)).toEqual(["NO-TIME", "EARLY", "LATE"]);
  });

  it("orders across midnight", () => {
    const now = new Date("2026-09-28T23:55:00Z");
    expect(orderSeqPlnStartup([
      cleared("TOMORROW", "0005", 1),
      cleared("TONIGHT", "2358", 2),
    ], now).filter(isFlight).map(strip => strip.callsign)).toEqual(["TOMORROW", "TONIGHT"]);
  });
});
