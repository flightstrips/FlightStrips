import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay } from "@/api/models";
import { ApnArrStrip } from "./ApnArrStrip";
import { FinalArrStrip } from "./FinalArrStrip";

const store = vi.hoisted(() => ({
  strips: [],
  inboundDepartures: null,
  runwayClearance: vi.fn(),
  runwayConfirmation: vi.fn(),
  resetRunwayClearance: vi.fn(),
  acknowledgeUnexpectedChange: vi.fn(),
  acknowledgeStandAssignment: vi.fn(),
  standAssignments: [],
  satEnabled: false,
}));

vi.mock("@/store/store-hooks", () => ({
  useStripTransfers: () => ({}),
  useWebSocketStore: (selector: (state: typeof store) => unknown) => selector(store),
}));

vi.mock("./shared", () => ({
  SEMI_BOLD_STROKE: "0px",
  AircraftTypeLabel: () => null,
  FONT: "Arial",
  SELECTION_COLOR: "#ffffff",
  COLOR_UNEXPECTED_YELLOW: "#ffff00",
  COLOR_MANUAL_BLUE: "#0000ff",
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
  getStripOwnership: (myPosition?: string, owner?: string) => ({ isUnconcerned: false, isAssumed: !!myPosition && myPosition === owner }),
  getCellTextColor: () => "#000000",
  useStripBg: () => ({ bg: "#ffff00", textWhite: false }),
  getValidationBlinkStyle: () => ({}),
  useNextFrequencyDisplay: () => "",
}));

vi.mock("./SIBox", () => ({ SIBox: () => null }));
vi.mock("./ArrStandDialog", () => ({ ArrStandDialog: () => null }));
vi.mock("./ValidationStatusDialog", () => ({ ValidationStatusDialog: () => null }));
vi.mock("@/components/FlightPlanDialog", () => ({ default: () => null }));
vi.mock("@/components/map-dialogs/TaxiMapDialog", () => ({ TaxiMapDialog: () => null }));
vi.mock("@/components/map-dialogs/ApronTaxiMapDialog", () => ({ ApronTaxiMapDialog: () => null }));
vi.mock("./RunwayDialog", () => ({
  RunwayDialog: ({ open }: { open: boolean }) =>
    open ? <div role="dialog" aria-label="Runway menu" /> : null,
}));

beforeEach(() => {
  vi.clearAllMocks();
});

describe.each([ApnArrStrip, FinalArrStrip])("%s runway field", Strip => {
  it("removes green clearance and requests it again on the next click", () => {
    const props = { callsign: "SAS123", bay: Bay.RwyArr, runway: "22L", owner: "TWR", myPosition: "TWR" };
    const { rerender } = render(<Strip {...props} runwayCleared runwayConfirmed />);

    fireEvent.click(screen.getByText("22L"));
    expect(store.resetRunwayClearance).toHaveBeenCalledWith("SAS123");

    rerender(<Strip {...props} runwayCleared={false} runwayConfirmed={false} />);
    fireEvent.click(screen.getByText("22L"));

    expect(store.runwayConfirmation).toHaveBeenCalledWith("SAS123");
    expect(store.runwayClearance).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog", { name: "Runway menu" })).not.toBeInTheDocument();
  });

  it("does not change RWY ARR clearance for a non-owning controller", () => {
    render(<Strip callsign="SAS456" bay={Bay.RwyArr} runway="22L" owner="TWR" myPosition="APN" runwayCleared runwayConfirmed />);

    fireEvent.click(screen.getByText("22L"));

    expect(store.resetRunwayClearance).not.toHaveBeenCalled();
    expect(store.runwayConfirmation).not.toHaveBeenCalled();
  });

});
