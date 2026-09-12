import {render, screen} from "@testing-library/react";
import {describe, expect, it} from "vitest";
import {RunwayClosureOverlay} from "./RunwayClosureOverlay";

const range = {startMs: Date.parse("2026-07-22T10:00:00Z"), endMs: Date.parse("2026-07-22T11:00:00Z")};
describe("runway closure overlay", () => {
  it("shows finite and indefinite server state with non-color and stale cues", () => {
    render(<RunwayClosureOverlay range={range} runway="ARRIVAL-22" status="stale" closures={[
      {id: "finite", start: "2026-07-22T10:10:00Z", end: "2026-07-22T10:20:00Z", reason: "inspection", created_at: "2026-07-22T09:00:00Z", created_by: "fmp"},
      {id: "indefinite", start: "2026-07-22T10:30:00Z", end: null, reason: "works", created_at: "2026-07-22T09:00:00Z", created_by: "fmp"},
    ]} />);
    expect(screen.getByLabelText(/inspection.*end exclusive.*stale state/)).toHaveTextContent("RWY CLOSED");
    expect(screen.getByLabelText(/works.*indefinite until removed/)).toHaveTextContent("INDEFINITE");
  });
});
