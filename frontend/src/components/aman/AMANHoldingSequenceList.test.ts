import {fireEvent, render, screen} from "@testing-library/react";
import {createElement} from "react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlight} from "@/api/aman";

import {moveIntentFor, AMANHoldingSequenceList} from "./AMANHoldingSequenceList";

const flights = [["A", 60], ["B", -120], ["C", 0], ["D", 300]].map(([callsign, gain]) => ({
  callsign, gain_loss_seconds: gain, runway_group_id: "ARRIVAL-22L",
})) as unknown as AMANFlight[];

describe("holding sequence moves", () => {
  it("builds before/after move commands", () => {
    expect(moveIntentFor(flights, 0, 2)).toMatchObject({type: "aman.move_flight", callsign: "A", after_callsign: "C"});
    expect(moveIntentFor(flights, 3, 1)).toMatchObject({callsign: "D", before_callsign: "B"});
    expect(moveIntentFor(flights, 1, 1)).toBeNull();
  });

  it("keeps gain/loss server-confirmed while a move is pending or rejected", () => {
    const onCommand = vi.fn(() => null);
    render(createElement(AMANHoldingSequenceList, {flights, onCommand}));

    fireEvent.keyDown(screen.getByLabelText("A sequence position 1"), {key: "ArrowUp"});

    expect(onCommand).toHaveBeenCalledWith({type: "aman.move_flight", callsign: "A", runway_group_id: "ARRIVAL-22L", after_callsign: "B"});
    expect(screen.getByLabelText("A sequence position 1")).toHaveTextContent("G01");
    expect(screen.getByLabelText("B sequence position 2")).toHaveTextContent("L02");
  });
});
