import {fireEvent, render, screen, within} from "@testing-library/react";
import {expect, it, vi} from "vitest";
import type {FrontendStrip} from "@/api/models";
import {RADISFlightPlan} from "./RADISFlightPlan";

const state = vi.hoisted(() => ({
  readOnly: false,
  boardContext: null,
  boardDefinition: null,
  transitionAltitude: 5000,
  annotateStrip: vi.fn(),
}));
vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

it.each([
  [5350, 5000, "5350"],
  [3050, 3000, "3050"],
  [7000, 5000, "FL070"],
  [5000, 5000, "5000"],
  [3000, 5000, "3000"],
  [0, 5000, ""],
])("preserves a %ift clearance with a %ift transition altitude", (feet, transition, display) => {
  state.transitionAltitude = transition;
  const strip = {id: 1, callsign: "TEST1", origin: "EKBI", destination: "EHAM", cleared_altitude: feet, requested_altitude: 0, has_fp: true} as FrontendStrip;
  const edit = vi.fn();
  render(<RADISFlightPlan strip={strip} onClose={vi.fn()} edit={edit}/>);
  const label = screen.getByText("ALT", {selector: ".radis-fpl-clearance span"}).parentElement!;
  const altitude = within(label).getByRole("button");
  expect(altitude.textContent).toBe(display);
  fireEvent.click(altitude);
  expect(edit).toHaveBeenCalledWith(strip, "altitude", "ALT", String(feet));
});
