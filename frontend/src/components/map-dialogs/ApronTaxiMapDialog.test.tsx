import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Bay, CommunicationType } from "@/api/models";
import { ApronTaxiMapDialog } from "./ApronTaxiMapDialog";

const state = vi.hoisted(() => ({
  strip: {
    bay: "TAXI_LWR",
    communication_type: "V",
    aircraft_type: "A320",
    aircraft_category: "M",
    origin: "EKCH",
    destination: "EGLL",
    release_point: "B",
  },
  openTextPilotMenu: vi.fn(),
  setReleasePoint: vi.fn(),
  acknowledgeUnexpectedChange: vi.fn(),
  move: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state & Record<string, unknown>) => unknown) => selector({
    ...state,
    displayedLayout: "AD",
    layout: "AD",
    airport: "EKCH",
  }),
  useApronOnline: () => false,
  useTwrOnline: () => false,
  useIsTwr: () => false,
  useMyPosition: () => undefined,
  useStrip: () => state.strip,
}));

vi.mock("./MapDialogShell", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./MapDialogShell")>();
  return {
    ...actual,
    MapDialogShell: ({ open, onSelect, children }: {
      open: boolean;
      onSelect: (label: string, point: { type?: "hp" | "cl" }) => void;
      children: React.ReactNode;
    }) => open && <>
      <button onClick={() => onSelect("R/V", { type: "hp" })}>R/V chart point</button>
      {children}
    </>,
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  state.strip.bay = Bay.TaxiLwr;
  state.strip.communication_type = CommunicationType.Voice;
});

describe("ApronTaxiMapDialog lower-bay routing", () => {
  it("keeps a lower-bay strip in the lower bay when its route changes to a non-final hold point", () => {
    render(<ApronTaxiMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);

    fireEvent.click(screen.getByRole("button", { name: "R/V chart point" }));

    expect(state.setReleasePoint).toHaveBeenCalledWith("SAS123", "R/V");
    expect(state.move).toHaveBeenCalledWith("SAS123", Bay.TaxiLwr);
  });

  it("continues routing upper-bay strips to upper for non-final hold points", () => {
    state.strip.bay = Bay.Taxi;
    render(<ApronTaxiMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);

    fireEvent.click(screen.getByRole("button", { name: "R/V chart point" }));

    expect(state.move).toHaveBeenCalledWith("SAS123", Bay.Taxi);
  });
});
