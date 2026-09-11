import {readFileSync} from "node:fs";
import {resolve} from "node:path";
import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";

import type {AMANFlight, AMANStateEvent} from "@/api/aman";
import {ACCTimeline} from "./ACCTimeline";

const golden = JSON.parse(readFileSync(
  resolve(process.cwd(), "../backend/pkg/events/frontend/testdata/aman-state-v1.json"),
  "utf8",
)) as AMANStateEvent;
const range = {startMs: Date.parse("2026-07-22T10:00:00.000Z"), endMs: Date.parse("2026-07-22T10:30:00.000Z")};

function flight(id: string, order: number, time: string): AMANFlight {
  const value = structuredClone(golden.data.flights[0]);
  value.flight_id = id;
  value.callsign = id.toUpperCase();
  value.order = order;
  value.slot = {...value.slot!, sequence: order, time};
  return value;
}

describe("ACC timeline", () => {
  it("keeps every in-horizon flight in authoritative order and at its published time", () => {
    render(<ACCTimeline
      clockMs={range.startMs}
      currentPosition={100}
      flights={[
        flight("second", 2, "2026-07-22T10:10:00.000Z"),
        flight("clipped", 3, "2026-07-22T10:31:00.000Z"),
        flight("first", 1, "2026-07-22T10:20:00.000Z"),
      ]}
      range={range}
      renderTarget={(value) => <button type="button">{value.callsign}</button>}
      status="fresh"
    />);

    const items = screen.getAllByRole("listitem");
    expect(items.map((item) => item.dataset.sequence)).toEqual(["1", "2"]);
    expect(items.map((item) => item.dataset.markerTime)).toEqual([
      "2026-07-22T10:20:00.000Z",
      "2026-07-22T10:10:00.000Z",
    ]);
    expect(screen.queryByText("CLIPPED")).not.toBeInTheDocument();
  });
});
