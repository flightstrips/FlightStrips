import {act, render, screen} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";

import {
  AMANAxisTopPercent,
  AMAN_HORIZON_PREFERENCE_KEY,
  AMANTimelineAxis,
  buildAMANAxisRange,
  formatAMANAxisLabel,
  readAMANHorizon,
  useAMANTimelineAxis,
  validateAMANHorizon,
  writeAMANHorizon,
} from "./AMANTimelineAxis";

function AxisHarness({time, status = "fresh"}: {time: string; status?: "fresh" | "stale" | "disconnected"}) {
  const axis = useAMANTimelineAxis(time);
  return <AMANTimelineAxis clockMs={axis.clockMs} range={axis.range} status={status} />;
}

describe("AMAN UTC timeline axis", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.useFakeTimers();
    vi.setSystemTime("2026-07-22T10:00:00.000Z");
  });

  afterEach(() => vi.useRealTimers());

  it("validates and persists only a browser-local 35–90 minute horizon", () => {
    expect([validateAMANHorizon(2), validateAMANHorizon(75.4), validateAMANHorizon(120)]).toEqual([35, 75, 90]);
    expect(validateAMANHorizon("60")).toBe(35);
    expect(readAMANHorizon()).toBe(35);
    writeAMANHorizon(120);
    expect(readAMANHorizon()).toBe(90);
    expect(JSON.parse(localStorage.getItem(AMAN_HORIZON_PREFERENCE_KEY)!)).toEqual({version: 1, minutes: 90});
  });

  it.each([60, 90])("renders a %i-minute horizon from authoritative backend time", (minutes) => {
    writeAMANHorizon(minutes);
    render(<AxisHarness time="2026-07-22T10:00:00.000Z" />);
    expect(screen.getByRole("img", {name: new RegExp(`09:55 to ${minutes === 60 ? "11:00" : "11:30"}, fresh`)})).toBeInTheDocument();
  });

  it("defaults to 35 minutes above and 5 minutes below the current time", () => {
    render(<AxisHarness time="2026-07-22T10:00:00.000Z" />);
    expect(screen.getByRole("img", {name: /09:55 to 10:35, fresh/})).toBeInTheDocument();
  });

  it("advances only the presentation clock on six-second ticks", () => {
    render(<AxisHarness time="2026-07-22T10:00:00.000Z" />);
    const marker = screen.getByTestId("aman-visual-clock");
    expect(Number.parseFloat(marker.style.top)).toBeCloseTo(100 - 5 / 40 * 100, 6);

    act(() => vi.advanceTimersByTime(6_000));

    expect(Number.parseFloat(marker.style.top)).toBeCloseTo(100 - 5.1 / 40 * 100, 6);
  });

  it("formats UTC midnight rollover deterministically", () => {
    const range = buildAMANAxisRange(Date.parse("2026-07-22T23:30:00.000Z"), 60);
    expect(formatAMANAxisLabel(range.endMs, range.startMs)).toBe("00:30 +1d");
    expect(AMANAxisTopPercent("2026-07-23T00:00:00.000Z", range)).toBeCloseTo(100 - 35 / 65 * 100, 8);
  });

  it("renders one-minute ticks and labels five-minute UTC intervals", () => {
    render(<AxisHarness time="2026-07-22T10:00:00.000Z" />);
    expect(screen.getAllByTestId("aman-axis-tick")).toHaveLength(41);
    expect(screen.getAllByTestId("aman-axis-tick").filter((tick) => tick.dataset.major === "true")).toHaveLength(9);
    expect(screen.queryByText("10:01")).not.toBeInTheDocument();
  });

  it.each([60, 90])("places the current-time line five minutes above the bottom of a %i-minute horizon", (minutes) => {
    const clockMs = Date.parse("2026-07-22T10:00:00.000Z");
    const range = buildAMANAxisRange(clockMs, minutes);
    render(<AMANTimelineAxis clockMs={clockMs} range={range} status="fresh" />);
    expect(Number.parseFloat(screen.getByTestId("final-ten-minute-region").style.top)).toBeCloseTo(100 - 5 / (minutes + 5) * 100, 3);
  });

  it.each(["stale", "disconnected"] as const)("keeps the axis visible and labelled while %s", (status) => {
    writeAMANHorizon(60);
    render(<AxisHarness status={status} time="2026-07-22T23:30:00.000Z" />);
    const axis = screen.getByRole("img", {name: new RegExp(`UTC timeline, 23:25 to 00:30 \\+1d, ${status}`)});
    expect(axis).toHaveAttribute("data-status", status);
    expect(screen.getByText(status)).toBeVisible();
  });
});
