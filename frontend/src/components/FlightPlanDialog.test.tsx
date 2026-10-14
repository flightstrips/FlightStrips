import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import FlightPlanDialog from "./FlightPlanDialog";

const state = vi.hoisted(() => ({
  strip: {
    callsign: "SAS123", aircraft_type: "B738/M", aircraft_category: "M", destination: "ENGM",
    eobt: "1450", tobt: "1505", tsat: "1500", ttot: "1515", ctot: "", status: "",
    bay: "NOT_CLEARED", runway: "22R", sid: "LANGO2C", heading: 0, remarks: "", route: "",
    assigned_squawk: "1000", release_point: "J4", pdc_state: "NONE", deice_platform: "",
    requested_altitude: 34000,
  },
  cdmReady: vi.fn(),
  clxUpdateTobt: vi.fn(),
  updateStrip: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useStrip: () => state.strip,
  useInitialCflByRunway: () => ({}),
  useTransitionAltitude: () => 5000,
  useAvailableSids: () => [],
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));
vi.mock("@/components/strip/ArrStandDialog", () => ({ ArrStandDialog: () => null }));
vi.mock("@/components/strip/AltSelectDialog", () => ({ AltSelectDialog: () => null }));
vi.mock("@/components/strip/HdgSelectDialog", () => ({
  HdgSelectDialog: ({ open, onSelect }: { open: boolean; onSelect: (heading: number | undefined) => void }) =>
    open ? <button type="button" onClick={() => onSelect(180)}>Select heading</button> : null,
}));
vi.mock("@/components/strip/SidRnavDialog", () => ({
  SidRnavDialog: ({ open, onSelectSid }: { open: boolean; onSelectSid: (sid: string) => void }) =>
    open ? <button type="button" onClick={() => onSelectSid("LANGO2C")}>Select SID</button> : null,
}));
vi.mock("@/components/strip/RunwayDialog", () => ({ RunwayDialog: () => null }));
vi.mock("@/components/MandatoryRouteDialog", () => ({ MandatoryRouteDialog: () => null }));
vi.mock("@/components/ManualPdcBypassDialog", () => ({ ManualPdcBypassDialog: () => null }));
vi.mock("@/components/est/EstDeIceDialog", () => ({ default: () => null }));

beforeEach(() => {
  vi.clearAllMocks();
  state.strip.status = "";
  state.strip.bay = Bay.NotCleared;
  state.strip.sid = "LANGO2C";
  state.strip.heading = 0;
});

describe("clearance SID, heading, and REA controls", () => {
  it("clears the SID when a heading is selected", () => {
    render(<FlightPlanDialog callsign="SAS123" open />);
    fireEvent.click(screen.getByRole("button", { name: "HDG" }));
    fireEvent.click(screen.getByRole("button", { name: "Select heading" }));

    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { heading: 180, sid: "" });
  });

  it("clears the heading when a SID is selected", () => {
    state.strip.sid = "";
    state.strip.heading = 180;
    render(<FlightPlanDialog callsign="SAS123" open />);
    fireEvent.click(screen.getByRole("button", { name: "SID" }));
    fireEvent.click(screen.getByRole("button", { name: "Select SID" }));

    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { sid: "LANGO2C", heading: 0 });
  });

  it("allows REA to be clicked again after it has been recorded", () => {
    const { rerender } = render(<FlightPlanDialog callsign="SAS123" open />);
    fireEvent.click(screen.getByRole("button", { name: "REA" }));
    state.strip.status = "REA";
    rerender(<FlightPlanDialog callsign="SAS123" open />);
    fireEvent.click(screen.getByRole("button", { name: "REA" }));

    expect(state.cdmReady).toHaveBeenNthCalledWith(1, "SAS123", true);
    expect(state.cdmReady).toHaveBeenNthCalledWith(2, "SAS123", true);
  });
});
