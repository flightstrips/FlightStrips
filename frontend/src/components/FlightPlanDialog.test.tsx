import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
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
  state.strip.tobt = "1505";
  state.strip.sid = "LANGO2C";
  state.strip.heading = 0;
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("flight-plan TSAT and REA controls", () => {
  it("makes TSAT read-only with the same disabled styling as TTOT", () => {
    render(<FlightPlanDialog callsign="SAS123" open />);
    const tsat = screen.getByRole("textbox", { name: "TSAT" });
    const ttot = screen.getByRole("textbox", { name: "TTOT" });
    expect(tsat).toBeDisabled();
    expect(tsat).toHaveValue("1500");
    expect(tsat.className).toBe(ttot.className);
    fireEvent.click(tsat);
    expect(state.clxUpdateTobt).not.toHaveBeenCalled();
    expect(state.cdmReady).not.toHaveBeenCalled();
  });

  it("uses a white REA clickspot and renders the persisted REA status instead of a release point", () => {
    const { rerender } = render(<FlightPlanDialog callsign="SAS123" open />);
    const rea = screen.getByRole("button", { name: "REA" });
    expect(rea).toHaveTextContent("");
    expect(rea.style.backgroundColor).toBe("rgb(252, 252, 252)");
    fireEvent.click(rea);
    expect(state.cdmReady).toHaveBeenCalledExactlyOnceWith("SAS123", true);
    expect(state.clxUpdateTobt).not.toHaveBeenCalled();
    state.strip.status = "REA";
    rerender(<FlightPlanDialog callsign="SAS123" open />);
    const reaAgain = screen.getByRole("button", { name: "REA" });
    expect(reaAgain).toHaveTextContent("REA");
    fireEvent.click(reaAgain);
    expect(state.cdmReady).toHaveBeenCalledTimes(2);
    expect(state.cdmReady).toHaveBeenLastCalledWith("SAS123", true);
  });

  it("clears the SID when a heading is selected", () => {
    render(<FlightPlanDialog callsign="SAS123" open />);

    fireEvent.click(screen.getByRole("button", { name: "HDG" }));
    fireEvent.click(screen.getByRole("button", { name: "Select heading" }));

    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { heading: 180, sid: "" });
  });

  it("clears the heading when a SID is selected with no current SID", () => {
    state.strip.sid = "";
    state.strip.heading = 180;
    render(<FlightPlanDialog callsign="SAS123" open />);

    fireEvent.click(screen.getByRole("button", { name: "SID" }));
    fireEvent.click(screen.getByRole("button", { name: "Select SID" }));

    expect(state.updateStrip).toHaveBeenCalledWith("SAS123", { sid: "LANGO2C", heading: 0 });
  });
});

describe("flight-plan TOBT warning", () => {
  it.each([
    ["clearance", "clearance", "0959", true],
    ["clearance", "clearance", "1046", true],
    ["clearance", "clearance", "1045", false],
    ["flight-plan", "view", "1000", false],
  ] as const)("colors an unrealistic TOBT in the %s dialog", (_name, mode, tobtValue, shouldWarn) => {
    vi.spyOn(Date, "now").mockReturnValue(Date.UTC(2026, 6, 14, 10, 0));
    state.strip.tobt = tobtValue;

    render(<FlightPlanDialog callsign="SAS123" open mode={mode} />);
    const tobt = screen.getByDisplayValue(tobtValue);

    expect(tobt.style.backgroundColor).toBe(shouldWarn ? "rgb(220, 38, 38)" : "");
  });

  it("removes the warning as soon as the edited TOBT is within limits", () => {
    vi.spyOn(Date, "now").mockReturnValue(Date.UTC(2026, 6, 14, 10, 0));
    state.strip.tobt = "1046";

    render(<FlightPlanDialog callsign="SAS123" open />);
    const tobt = screen.getByDisplayValue("1046");
    expect(tobt.style.backgroundColor).toBe("rgb(220, 38, 38)");

    fireEvent.focus(tobt);
    fireEvent.change(tobt, { target: { value: "1045" } });
    expect(tobt.style.backgroundColor).toBe("");
  });
});
