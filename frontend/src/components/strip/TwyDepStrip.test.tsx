import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import { TwyDepStrip } from "./TwyDepStrip";

const store = vi.hoisted(() => ({
  updateStrip: vi.fn(),
  runwayClearance: vi.fn(),
  runwayConfirmation: vi.fn(),
  acknowledgeUnexpectedChange: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useStripTransfers: () => ({}),
  useTransitionAltitude: () => 5000,
  useInitialCflByRunway: () => ({ "22R": 4000 }),
  useWebSocketStore: (selector: (state: typeof store) => unknown) => selector(store),
}));

vi.mock("./shared", () => ({
  SEMI_BOLD_STROKE: "0px",
  AircraftTypeLabel: () => null,
  COLOR_ARR_YELLOW: "#ffff00",
  COLOR_BTN_BLUE: "#0000ff",
  COLOR_DEP_STRIP_BG: "#00ffff",
  COLOR_UNEXPECTED_YELLOW: "#ffff00",
  getValidationBlockedCursor: () => undefined,
  useStripCallsignInteraction: () => ({
    isSelected: false,
    isValidationActive: false,
    handleClick: vi.fn(),
    handleContextMenu: vi.fn(),
    guardValidationAction: (event: { stopPropagation: () => void }, action: () => void) => {
      event.stopPropagation();
      action();
    },
    validationDialogOpen: false,
    setValidationDialogOpen: vi.fn(),
    validationStatus: undefined,
  }),
  getCellBorderColor: () => "#000000",
  getFlatStripBorderStyle: () => ({}),
  getStripFrameColor: () => "#000000",
  SELECTION_COLOR: "#ffffff",
  FONT: "Arial",
  getStripOwnership: () => ({ isUnconcerned: false, isAssumed: false }),
  getCellTextColor: () => "#000000",
  useStripBg: () => ({ bg: "#b3ffff", textWhite: false }),
  getValidationBlinkStyle: () => ({}),
  useNextFrequencyDisplay: () => "",
}));

vi.mock("@/hooks/useCTOTColor", () => ({
  useCTOTColor: () => ({ ctotBg: "", ctotColor: "", showCtot: false }),
}));
vi.mock("../map-dialogs/TaxiMapDialog", () => ({ TaxiMapDialog: () => null }));
vi.mock("../map-dialogs/HoldingPointDialog", () => ({ HoldingPointDialog: () => null }));
vi.mock("./SIBox", () => ({ SIBox: () => null }));
vi.mock("./ValidationStatusDialog", () => ({ ValidationStatusDialog: () => null }));
vi.mock("./DepartureAwareFlightPlanDialog", () => ({ DepartureAwareFlightPlanDialog: () => null }));
vi.mock("./AltSelectDialog", () => ({
  AltSelectDialog: ({ open, onSelect }: { open: boolean; onSelect: (altitude: number) => void }) =>
    open ? (
      <div role="dialog" aria-label="Altitude menu">
        <button type="button" onClick={() => onSelect(7000)}>Set altitude</button>
      </div>
    ) : null,
}));
vi.mock("./HdgSelectDialog", () => ({
  HdgSelectDialog: ({ open, onSelect }: { open: boolean; onSelect: (heading: number) => void }) =>
    open ? (
      <div role="dialog" aria-label="Heading menu">
        <button type="button" onClick={() => onSelect(270)}>Set heading</button>
      </div>
    ) : null,
}));

beforeEach(() => {
  vi.clearAllMocks();
});

function renderStrip() {
  return render(
    <TwyDepStrip
      callsign="SAS123"
      bay={Bay.Depart}
      pdcStatus="NONE"
      runway="22R"
      clearedAltitude={5000}
      heading={180}
    />,
  );
}

describe("TWY DEP strip altitude and heading controls", () => {
  it("opens the altitude menu and sends the selected altitude to the strip", () => {
    renderStrip();

    fireEvent.click(screen.getByRole("button", { name: "Cleared altitude" }));
    fireEvent.click(screen.getByRole("button", { name: "Set altitude" }));

    expect(store.updateStrip).toHaveBeenCalledWith("SAS123", { altitude: 7000 });
  });

  it("opens the heading menu and sends the selected heading to the strip", () => {
    renderStrip();

    fireEvent.click(screen.getByRole("button", { name: "Heading" }));
    fireEvent.click(screen.getByRole("button", { name: "Set heading" }));

    expect(store.updateStrip).toHaveBeenCalledWith("SAS123", { heading: 270 });
  });
});
