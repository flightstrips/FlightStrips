import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, type ValidationStatus } from "@/api/models";
import { ApnArrStrip } from "./ApnArrStrip";
import { ApnPushStrip } from "./ApnPushStrip";
import { ApnTaxiDepStrip } from "./ApnTaxiDepStrip";
import { FinalArrStrip } from "./FinalArrStrip";
import { GroundStrip } from "./GroundStrip";
import { HalfStrip } from "./HalfStrip";
import { TwyDepStrip } from "./TwyDepStrip";
import { COLOR_CLOSED_RWY, COLOR_MANUAL_BLUE, SELECTION_COLOR } from "./shared";

const state = vi.hoisted(() => ({
  selectedCallsign: null as string | null,
  strips: [] as { callsign: string; validation_status?: ValidationStatus }[],
  localMarked: {},
  standAssignments: [],
  displayedLayout: "AA",
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useSelectedCallsign: () => state.selectedCallsign,
  useSelectStrip: () => vi.fn(),
  useTagRequestArmed: () => false,
  useMarkArmed: () => false,
  useStripTransfers: () => ({ SAS123: { from: "121.730", to: "118.580", isTagRequest: true } }),
  useRunwaySetup: () => ({ runway_status: { "04L-22R": "CLOSED" } }),
  useIsTwr: () => false,
  useTransitionAltitude: () => 5000,
  useInitialCflByRunway: () => ({}),
  useWebSocketStore: (selector: (store: typeof state) => unknown) => selector(state),
}));
vi.mock("@/components/FlightPlanDialog", () => ({ default: () => null }));
vi.mock("./DepartureAwareFlightPlanDialog", () => ({ DepartureAwareFlightPlanDialog: () => null }));
vi.mock("./ArrStandDialog", () => ({ ArrStandDialog: () => null }));
vi.mock("./RunwayDialog", () => ({ RunwayDialog: () => null }));
vi.mock("./AltSelectDialog", () => ({ AltSelectDialog: () => null }));
vi.mock("./HdgSelectDialog", () => ({ HdgSelectDialog: () => null }));
vi.mock("./ValidationStatusDialog", () => ({ ValidationStatusDialog: () => null }));
vi.mock("./InboundCtotFlag", () => ({ InboundCtotFlag: () => null }));
vi.mock("@/components/map-dialogs/PushbackMapDialog", () => ({ PushbackMapDialog: () => null }));
vi.mock("@/components/map-dialogs/ApronTaxiMapDialog", () => ({ ApronTaxiMapDialog: () => null }));
vi.mock("@/components/map-dialogs/TaxiMapDialog", () => ({ TaxiMapDialog: () => null }));
vi.mock("@/components/map-dialogs/HoldingPointDialog", () => ({ HoldingPointDialog: () => null }));

const layouts = { ApnArrStrip, ApnPushStrip, ApnTaxiDepStrip, FinalArrStrip, GroundStrip, TwyDepStrip };

describe.each(Object.entries(layouts))("%s tag request contrast", (_, Component) => {
  beforeEach(() => {
    vi.useFakeTimers();
    state.selectedCallsign = "SAS123";
    state.strips = [{ callsign: "SAS123", validation_status: {
      issue_type: "STAND ASSIGNMENT", message: "Advisory", owning_position: "121.730", active: true, activation_key: "key",
    } }];
  });
  afterEach(() => vi.useRealTimers());

  it("preserves selection behind validation blinking without recoloring the lower row", () => {
    render(<Component callsign="SAS123" runway="04L" selectable bay={Bay.Cleared} pdcStatus="CLEARED"
      owner="121.730" myPosition="121.730" nextControllers={["118.580"]} nextDisplay={{ label: "GW", frequency: "118.580" }} />);
    const row = screen.getByText("SAS123").parentElement!;
    const column = row.parentElement!;
    expect(row.style.color).toBe("black");
    expect(row.style.getPropertyValue("--validation-blink-background")).toBe(SELECTION_COLOR);
    expect(column.style.backgroundColor).toBe(COLOR_CLOSED_RWY);
    expect(column.style.color).toBe("white");
    act(() => vi.advanceTimersByTime(1000));
    expect(column.style.color).toBe("white");
    expect(row.style.color).toBe("black");
  });
});

describe.each(Object.entries({ ApnArrStrip, ApnPushStrip, HalfStrip }))("%s manual tag request contrast", (_, Component) => {
  beforeEach(() => {
    vi.useFakeTimers();
    state.selectedCallsign = null;
    state.strips = [];
  });
  afterEach(() => vi.useRealTimers());

  it.each([
    { name: "closed runway", runway: "04L", pdcStatus: "NONE" as const },
    { name: "PDC clearance", runway: undefined, pdcStatus: "CLEARED" as const },
  ])("keeps manual callsigns white on the $name background in both blink phases", ({ runway, pdcStatus }) => {
    render(<Component callsign="SAS123" isManual runway={runway} bay={Bay.Cleared} pdcStatus={pdcStatus} />);
    const callsign = screen.getByText("SAS123");
    expect(callsign.style.color).not.toBe(COLOR_MANUAL_BLUE);
    expect(getComputedStyle(callsign).color).toBe("rgb(255, 255, 255)");
    act(() => vi.advanceTimersByTime(1000));
    expect(callsign.style.color).not.toBe(COLOR_MANUAL_BLUE);
    expect(getComputedStyle(callsign).color).toBe("rgb(255, 255, 255)");
  });

  it("preserves manual blue on a light callsign background", () => {
    render(<Component callsign="SAS123" isManual bay={Bay.Cleared} />);
    const callsign = screen.getByText("SAS123");
    expect(callsign.style.color).toBe(COLOR_MANUAL_BLUE);
    act(() => vi.advanceTimersByTime(1000));
    expect(callsign.style.color).toBe(COLOR_MANUAL_BLUE);
  });
});
