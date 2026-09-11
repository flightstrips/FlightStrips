import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {fireEvent, render, screen} from "@testing-library/react";
import {describe, expect, it, vi} from "vitest";

import type {AMANFlight, AMANState, AMANStateEvent} from "@/api/aman";
import {RWYPairedTimeline} from "./RWYPairedTimeline";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;
const range = {startMs: Date.UTC(2026, 6, 22, 10), endMs: Date.UTC(2026, 6, 22, 10, 30)};

function flight(id: string, runway: string, order: number, minute: number): AMANFlight {
  const value = structuredClone(golden.data.flights[0]);
  value.flight_id = id;
  value.callsign = id.toUpperCase();
  value.runway_group_id = runway;
  value.order = order;
  value.slot = {...value.slot!, runway_group_id: runway, sequence: order, time: new Date(Date.UTC(2026, 6, 22, 10, minute)).toISOString()};
  return value;
}

function runwayState(ids: string[]): AMANState {
  const value = structuredClone(golden.data);
  value.runway_groups = ids.map((id) => ({id}));
  value.active_runway_groups = ids;
  value.flights = ids.flatMap((id, index) => Array.from({length: ids.length - index}, (_, flightIndex) =>
    flight(`${id}-${flightIndex}`, id, flightIndex + 1, 5 + index * 2 + flightIndex),
  ));
  return value;
}

function renderTimeline(state: AMANState, onSelect = vi.fn()) {
  render(<RWYPairedTimeline
    clockMs={range.startMs}
    currentPosition={100}
    range={range}
    renderTarget={(value) => <button onClick={() => onSelect(value.flight_id)} type="button">{value.callsign}</button>}
    state={state}
    status="fresh"
  />);
  return onSelect;
}

describe("RWY paired timeline", () => {
  it.each([1, 2, 3, 4])("renders %i authoritative active runway lanes", (count) => {
    renderTimeline(runwayState(["A", "B", "C", "D"].slice(0, count)));
    expect(screen.getAllByRole("list")).toHaveLength(count);
  });

  it("limits the presentation to four runway lanes", () => {
    renderTimeline(runwayState(["A", "B", "C", "D", "E"]));
    expect(screen.getAllByRole("list")).toHaveLength(4);
    expect(screen.getByRole("status")).toHaveTextContent("Showing the first four active runway groups");
  });

  it("uses documented stable placement order by arrival count and configured ties", () => {
    const state = runwayState(["TIE-FIRST", "MOST", "TIE-SECOND", "FOURTH"]);
    state.flights = [
      flight("most-1", "MOST", 1, 5), flight("most-2", "MOST", 2, 6), flight("most-3", "MOST", 3, 7),
      flight("tie-first", "TIE-FIRST", 1, 8), flight("tie-second", "TIE-SECOND", 1, 9),
    ];
    renderTimeline(state);

    expect(screen.getByTestId("rwy-timeline-left-spacer")).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "Middle runway timeline"})).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "Right runway timeline"})).toBeInTheDocument();
    expect(screen.getByTestId("rwy-lane-MOST")).toHaveAttribute("data-placement", "middle-left");
    expect(screen.getByTestId("rwy-lane-TIE-FIRST")).toHaveAttribute("data-placement", "middle-right");
    expect(screen.getByTestId("rwy-lane-TIE-SECOND")).toHaveAttribute("data-placement", "right-left");
    expect(screen.getByTestId("rwy-lane-FOURTH")).toHaveAttribute("data-placement", "right-right");
  });

  it("preserves authoritative order and timing while clipping the local horizon", () => {
    const state = runwayState(["A"]);
    state.flights = [flight("second", "A", 2, 10), flight("clipped", "A", 3, 31), flight("first", "A", 1, 20)];
    renderTimeline(state);

    const items = screen.getAllByRole("listitem");
    expect(items.map((item) => item.getAttribute("aria-label"))).toEqual([
      "FIRST at 2026-07-22T10:20:00.000Z",
      "SECOND at 2026-07-22T10:10:00.000Z",
    ]);
    expect(items.map((item) => item.dataset.sequence)).toEqual(["1", "2"]);
  });

  it("keeps target keyboard activation in stable accessible lane order", () => {
    const onSelect = renderTimeline(runwayState(["A", "B"]));
    const buttons = screen.getAllByRole("button");
    expect(buttons.map((button) => button.textContent)).toEqual(["A-0", "A-1", "B-0"]);

    buttons[0].focus();
    fireEvent.click(buttons[0], {detail: 0});
    expect(buttons[0]).toHaveFocus();
    expect(onSelect).toHaveBeenCalledWith("A-0");
  });

  it("uses the legacy selected runway only when the optional active set is absent", () => {
    const compatible = runwayState(["A", "B"]);
    compatible.active_runway_groups = undefined;
    compatible.runway_groups[1].selected = true;
    renderTimeline(compatible);
    expect(screen.getByRole("list", {name: "B active runway arrivals"})).toBeInTheDocument();
    expect(screen.queryByRole("list", {name: "A active runway arrivals"})).not.toBeInTheDocument();
  });

  it.each([
    ["empty", []],
    ["unknown", ["NOT-CONFIGURED"]],
    ["malformed", "A"],
  ])("shows unavailable for an %s published active set without inventing lanes", (_label, active) => {
    const state = runwayState(["A"]);
    state.active_runway_groups = active as string[];
    renderTimeline(state);
    expect(screen.getByRole("status")).toHaveTextContent("Active runway data unavailable");
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
  });
});
