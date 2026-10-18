import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StripContextMenu } from "./StripContextMenu";

const menuState = vi.hoisted(() => ({
  strip: { callsign: "SAS123", owner: "118.105", ob: true },
  controllers: [{ position: "118.105", identifier: "TE" }],
}));

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => menuState.controllers,
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
  useIsLocallyHidden: () => false,
  useLocalHiddenStrips: (selector: (state: { setHidden: ReturnType<typeof vi.fn> }) => unknown) =>
    selector({ setHidden: vi.fn() }),
}));

describe("StripContextMenu", () => {
  beforeEach(() => {
    menuState.strip = { callsign: "SAS123", owner: "118.105", ob: true };
    menuState.controllers = [{ position: "118.105", identifier: "TE" }];
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
    menuState.strip = { callsign: "SAS123", owner: "", ob: false };

    render(<StripContextMenu callsign="SAS123" position={{ x: 0, y: 0 }} onClose={vi.fn()} />);

    expect(screen.getByText("OB")).toBeInTheDocument();
  });
});
