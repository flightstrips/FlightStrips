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

  it("draws the blue box around the whole row on hover and when selected", () => {
    render(createElement(AMANHoldingSequenceList, {flights, onCommand: vi.fn()}));
    const row = screen.getByLabelText("B sequence position 2");

    expect(row).toHaveClass("[&:hover]:border-white", "[&:hover]:bg-[#5174b8]");
    expect(row).not.toHaveClass("bg-[#5174b8]");

    fireEvent.pointerDown(row, {pointerId: 1});
    expect(row).toHaveClass("border-white", "bg-[#5174b8]");
    expect(row).toHaveAttribute("data-selected", "true");
    expect(screen.getByLabelText("A sequence position 1")).not.toHaveClass("bg-[#5174b8]");

    // Releasing ends the selection, so the row does not stay blue.
    const original = document.elementFromPoint;
    document.elementFromPoint = vi.fn(() => row);
    try {
      fireEvent.pointerUp(row, {pointerId: 1});
    } finally {
      document.elementFromPoint = original;
    }
    expect(row).not.toHaveAttribute("data-selected");
    expect(row).not.toHaveClass("bg-[#5174b8]");
  });

  it("shows the EuroScope CFL between the callsign and the gain/lose", () => {
    const cleared = [
      {...flights[0], cleared_altitude: 9000},
      {...flights[1], cleared_altitude: 4000},
      {...flights[2], cleared_altitude: 1},
      flights[3],
    ] as AMANFlight[];
    render(createElement(AMANHoldingSequenceList, {flights: cleared, onCommand: vi.fn(), transitionAltitude: 5000}));

    const cfl = (label: string) => screen.getByLabelText(label).querySelector("[data-field='cfl']");
    expect(cfl("A sequence position 1")).toHaveTextContent("FL90");
    expect(cfl("B sequence position 2")).toHaveTextContent("4000");
    expect(cfl("C sequence position 3")).toHaveTextContent("ILS");
    expect(cfl("D sequence position 4")).toHaveTextContent(/^$/);
    // Column order: callsign, CFL, gain/lose, EAT.
    expect(screen.getByLabelText("A sequence position 1")).toHaveTextContent(/A.*FL90.*G01/);
  });
});
