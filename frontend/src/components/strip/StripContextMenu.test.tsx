import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StripContextMenu } from "./StripContextMenu";

const menuState = vi.hoisted(() => ({
  strip: { callsign: "SAS123", owner: "118.105", ob: true, origin: "ESSA", destination: "EKCH" },
  controllers: [{ position: "118.105", identifier: "TE" }],
  localHidden: false,
  setHidden: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => menuState.controllers,
  useAirport: () => "EKCH",
  useIsClrDel: () => false,
  useStrip: () => menuState.strip,
  useStripTransfers: () => ({}),
  useWebSocketStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      position: "121.905",
      forceAssumeStrip: vi.fn(),
      cancelTransfer: vi.fn(),
      updateStrip: vi.fn(),
    }),
}));

vi.mock("@/store/localHiddenStrips", () => ({
  useIsLocallyHidden: () => menuState.localHidden,
  useLocalHiddenStrips: (selector: (state: { setHidden: ReturnType<typeof vi.fn> }) => unknown) =>
    selector({ setHidden: menuState.setHidden }),
}));

describe("StripContextMenu", () => {
  beforeEach(() => {
    menuState.strip = { callsign: "SAS123", owner: "118.105", ob: true, origin: "ESSA", destination: "EKCH" };
    menuState.controllers = [{ position: "118.105", identifier: "TE" }];
    menuState.localHidden = false;
    menuState.setHidden.mockClear();
  });

  it("shows the identifier for a frequency owner instead of OB or IB", () => {
    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);

    expect(screen.getByText("TE")).toBeInTheDocument();
    expect(screen.queryByText("118.105")).not.toBeInTheDocument();
    expect(screen.queryByText("OB")).not.toBeInTheDocument();
    expect(screen.queryByText("IB")).not.toBeInTheDocument();
  });

  it("does not show a raw frequency or OB/IB when the matching controller is unavailable", () => {
    menuState.controllers = [];

    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);

    expect(screen.getByText("—")).toBeInTheDocument();
    expect(screen.queryByText("118.105")).not.toBeInTheDocument();
    expect(screen.queryByText("OB")).not.toBeInTheDocument();
    expect(screen.queryByText("IB")).not.toBeInTheDocument();
  });

  it("keeps the OB/IB toggle for a non-frequency owner", () => {
    menuState.strip = { callsign: "SAS123", owner: "", ob: false, origin: "ESSA", destination: "EKCH" };

    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);

    expect(screen.getByText("OB")).toBeInTheDocument();
  });

  it("offers LCL HIDE for arrivals and hides them locally", () => {
    const onClose = vi.fn();
    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={onClose} />);

    fireEvent.click(screen.getByText("LCL HIDE"));

    expect(menuState.setHidden).toHaveBeenCalledWith("SAS123", true);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("keeps LCL MINI for departures and offers UNHIDE for an already hidden arrival", () => {
    menuState.strip = { callsign: "SAS123", owner: "", ob: false, origin: "EKCH", destination: "ESSA" };
    const departure = render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);
    expect(screen.getByText("LCL MINI")).toBeInTheDocument();
    departure.unmount();

    menuState.strip = { callsign: "SAS123", owner: "", ob: false, origin: "ESSA", destination: "EKCH" };
    menuState.localHidden = true;
    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);
    expect(screen.getByText("UNHIDE")).toBeInTheDocument();
  });
});
