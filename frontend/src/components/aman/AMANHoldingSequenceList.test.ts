import {describe, expect, it} from "vitest";

import type {AMANFlight} from "@/api/aman";

import {moveIntentFor, reorderWithSlotGainLoss} from "./AMANHoldingSequenceList";

const flights = [["A", 60], ["B", -120], ["C", 0], ["D", 300]].map(([callsign, gain]) => ({
  callsign, gain_loss_seconds: gain, runway_group_id: "ARRIVAL-22L",
})) as unknown as AMANFlight[];

describe("holding sequence moves", () => {
  it("swaps gain/loss when moving one place", () => {
    const {order, gainLoss} = reorderWithSlotGainLoss(flights, 0, 1);
    expect(order.map((f) => f.callsign)).toEqual(["B", "A", "C", "D"]);
    expect(gainLoss.get("B")).toBe(60);
    expect(gainLoss.get("A")).toBe(-120);
  });

  it("rotates gain/loss through every passed flight when moving several places", () => {
    const {order, gainLoss} = reorderWithSlotGainLoss(flights, 0, 2);
    expect(order.map((f) => f.callsign)).toEqual(["B", "C", "A", "D"]);
    expect(gainLoss.get("B")).toBe(60);
    expect(gainLoss.get("C")).toBe(-120);
    expect(gainLoss.get("A")).toBe(0);
    expect(gainLoss.get("D")).toBe(300);
  });

  it("builds before/after move commands", () => {
    expect(moveIntentFor(flights, 0, 2)).toMatchObject({type: "aman.move_flight", callsign: "A", after_callsign: "C"});
    expect(moveIntentFor(flights, 3, 1)).toMatchObject({callsign: "D", before_callsign: "B"});
    expect(moveIntentFor(flights, 1, 1)).toBeNull();
  });
});
