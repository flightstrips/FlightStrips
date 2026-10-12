import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import FlightPlanDialog from "./FlightPlanDialog";

const state = vi.hoisted(() => ({
  strip: {
    callsign: "SAS123", aircraft_type: "B738/M", aircraft_category: "M", destination: "ENGM",
    eobt: "1450", tobt: "1505", tsat: "1500", ttot: "1515", ctot: "", status: "",
    bay: "NOT_CLEARED", runway: "22R", sid: "LANGO2C", remarks: "", route: "",
    assigned_squawk: "1000", release_point: "J4", pdc_state: "NONE", deice_platform: "",
    requested_altitude: 34000,
  },
  cdmReady: vi.fn(),
  clxUpdateTobt: vi.fn(),
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
vi.mock("@/components/strip/HdgSelectDialog", () => ({ HdgSelectDialog: () => null }));
vi.mock("@/components/strip/SidRnavDialog", () => ({ SidRnavDialog: () => null }));
vi.mock("@/components/strip/RunwayDialog", () => ({ RunwayDialog: () => null }));
vi.mock("@/components/MandatoryRouteDialog", () => ({ MandatoryRouteDialog: () => null }));
vi.mock("@/components/ManualPdcBypassDialog", () => ({ ManualPdcBypassDialog: () => null }));
vi.mock("@/components/est/EstDeIceDialog", () => ({ default: () => null }));

beforeEach(() => {
  vi.clearAllMocks();
  state.strip.status = "";
  state.strip.bay = Bay.NotCleared;
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
    expect(screen.getByRole("button", { name: "REA" })).toHaveTextContent("REA");
  });
});
