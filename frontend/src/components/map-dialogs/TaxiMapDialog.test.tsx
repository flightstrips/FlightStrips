import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { CommunicationType } from "@/api/models";
import { TaxiMapDialog } from "./TaxiMapDialog";

const state = vi.hoisted(() => ({
  strip: { communication_type: "V", aircraft_type: "A35K", aircraft_category: "H", origin: "EKCH" },
  openTextPilotMenu: vi.fn(),
  setReleasePoint: vi.fn(),
  acknowledgeUnexpectedChange: vi.fn(),
  move: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  controllerOwnsTowerSectors: () => false,
  useWebSocketStore: (selector: (value: typeof state & Record<string, unknown>) => unknown) => selector({
    ...state,
    displayedLayout: "",
    layout: "",
    airport: "EKCH",
    controllers: [],
  }),
  useRunwaySetup: () => ({ departure: ["22R"], arrival: [] }),
  useApronOnline: () => false,
  useTwrOnline: () => false,
  useIsTwr: () => true,
  useMyPosition: () => undefined,
  useStrip: () => state.strip,
}));

vi.mock("./MapDialogShell", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./MapDialogShell")>();
  return {
    ...actual,
    MapDialogShell: ({ open, points, onSelect, isPointDisabled, children }: {
      open: boolean;
      points: Array<{ label: string | ((context: never) => string); left: string; top: string; type?: "hp" | "cl" }>;
      onSelect: (label: string, point: { type?: "hp" | "cl" }) => void;
      isPointDisabled: (label: string, point: { type?: "hp" | "cl" }) => boolean;
      children: React.ReactNode;
    }) => open && <>
      {points
        .filter((point) => point.label === "A1")
        .map((point) => <button
          key={point.left}
          disabled={isPointDisabled("A1", point)}
          onClick={() => onSelect("A1", point)}
        >
          A1 chart point
        </button>)}
      {children}
    </>,
  };
});

beforeEach(() => {
  vi.clearAllMocks();
  state.strip.communication_type = CommunicationType.Voice;
});

describe("TaxiMapDialog free-text holding points", () => {
  it("blocks manual entry of a holding point when its chart point is restricted", () => {
    render(<TaxiMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);

    expect(screen.getByRole("button", { name: "A1 chart point" })).toBeDisabled();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "A1" } });
    const okButton = screen.getByRole("button", { name: "OK" });
    expect(okButton).toBeDisabled();
    fireEvent.click(okButton);

    expect(state.setReleasePoint).not.toHaveBeenCalled();
  });

  it("allows manual entry of an unrestricted holding point", () => {
    render(<TaxiMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);

    fireEvent.change(screen.getByRole("textbox"), { target: { value: "F1" } });
    const okButton = screen.getByRole("button", { name: "OK" });
    expect(okButton).toBeEnabled();
    fireEvent.click(okButton);

    expect(state.setReleasePoint).toHaveBeenCalledWith("SAS123", "F1");
  });
});
