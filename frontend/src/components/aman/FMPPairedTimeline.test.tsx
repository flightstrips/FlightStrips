import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {render, screen, within} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import type {AMANFlight, AMANStateEvent} from "@/api/aman";
import {FMPPairedTimeline} from "./FMPPairedTimeline";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;

function flight(id: string, family: string, order: number, slot: string): AMANFlight {
  return {
    ...structuredClone(golden.data.flights[0]),
    flight_id: id,
    callsign: id.toUpperCase(),
    star_family: family,
    order,
    operational_teta: "2026-07-23T00:24:00.000Z",
    raw_teta: "2026-07-23T00:29:00.000Z",
    slot: {...golden.data.flights[0].slot!, sequence: order, time: slot},
  };
}

describe("FMP paired feeder timelines", () => {
  it("uses configured placement and preserves authoritative reading order, time, and the unused side across midnight", () => {
    const flights = [
      flight("tespi-second", "TESPI", 2, "2026-07-23T00:01:30.000Z"),
      flight("tespi-first", "TESPI", 1, "2026-07-23T00:02:00.000Z"),
      flight("tudlo", "TUDLO", 3, "2026-07-23T00:03:00.000Z"),
      flight("monak", "MONAK", 4, "2026-07-23T00:08:00.000Z"),
      flight("tidvu-clipped", "TIDVU", 5, "2026-07-23T00:26:00.000Z"),
      flight("ernov", "ERNOV", 6, "2026-07-23T00:20:00.000Z"),
    ];
    const range = {startMs: Date.parse("2026-07-22T23:55:00.000Z"), endMs: Date.parse("2026-07-23T00:25:00.000Z")};

    render(<FMPPairedTimeline
      clockMs={range.startMs}
      currentPosition={100}
      flights={flights}
      mappings={[
        {id: 1, left: "TESPI", right: "TUDLO"},
        {id: 2, left: "MONAK", right: "TIDVU"},
        {id: 3, left: "ERNOV", right: null},
      ]}
      range={range}
      renderTarget={(value) => <span>{value.callsign}</span>}
      status="fresh"
    />);

    expect(screen.getByRole("region", {name: "Timeline 1: TESPI left, TUDLO right"})).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "Timeline 3: ERNOV left, unused right"})).toBeInTheDocument();
    expect(screen.getByLabelText("Unused right side")).toBeInTheDocument();
    expect(within(screen.getByRole("list", {name: "TESPI arrivals"})).getAllByRole("listitem").map((item) => item.textContent))
      .toEqual(["TESPI-FIRST", "TESPI-SECOND"]);
    expect(screen.getByLabelText(/TESPI-FIRST at/)).toHaveAttribute("data-marker-time", "2026-07-23T00:02:00.000Z");
    expect(screen.queryByText("TIDVU-CLIPPED")).not.toBeInTheDocument();
    expect(screen.getAllByRole("img", {name: /23:55 to 00:25 \+1d/})).toHaveLength(3);
  });
});
