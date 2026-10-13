import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { TransferControllerButton } from "./TransferControllerButton";
import { TRANSFER_CONTROLLER_STATIONS } from "@/lib/transferControllerStations";

const state = vi.hoisted(() => ({
  selectedCallsign: "SAS123" as string | null,
  position: "119.905",
  callsign: "EKCH_DEL",
  strip: { owner: "119.905", next_controllers: ["121.830"] } as { owner: string; next_controllers: string[] } | undefined,
  controllers: [{ callsign: "EKCH_GW_TWR", position: "119.999", identifier: "GW", section: "GND", owned_sectors: ["GWD"] }],
  transferStrip: vi.fn(),
  selectNextController: vi.fn(),
}));

vi.mock("@/store/store-hooks", () => ({
  useCallsign: () => state.callsign,
  useControllers: () => state.controllers,
  useMyPosition: () => state.position,
  useSelectedCallsign: () => state.selectedCallsign,
  useStrip: () => state.strip,
  useWebSocketStore: (selector: (store: typeof state) => unknown) => selector(state),
}));

describe("TransferControllerButton", () => {
  beforeEach(() => {
    state.selectedCallsign = "SAS123";
    state.position = "119.905";
    state.callsign = "EKCH_DEL";
    state.strip = { owner: "119.905", next_controllers: ["121.830"] };
    state.controllers = [{
      callsign: "EKCH_GW_TWR",
      position: "119.999",
      identifier: "GW",
      section: "GND",
      owned_sectors: ["GWD"],
    }];
    state.transferStrip.mockClear();
    state.selectNextController.mockClear();
  });

  it("selects the next-controller sector without initiating a transfer", () => {
    render(
      <TransferControllerButton
        label="GW"
        station={TRANSFER_CONTROLLER_STATIONS.GW}
        className="bay-tab-btn"
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "GW" }));

    expect(state.selectNextController).toHaveBeenCalledWith("SAS123", "GW");
    expect(state.transferStrip).not.toHaveBeenCalled();
  });

  it("disables the shortcut unless a selected strip is owned here and the target is online", () => {
    state.strip = { owner: "another-position", next_controllers: ["121.830"] };
    const { rerender } = render(
      <TransferControllerButton
        label="GW"
        station={TRANSFER_CONTROLLER_STATIONS.GW}
        className="bay-tab-btn"
      />,
    );

    expect(screen.getByRole("button", { name: "GW" })).toBeDisabled();

    state.strip = { owner: "119.905", next_controllers: ["121.830"] };
    state.controllers = [];
    rerender(
      <TransferControllerButton
        label="GW"
        station={TRANSFER_CONTROLLER_STATIONS.GW}
        className="bay-tab-btn"
      />,
    );

    expect(screen.getByRole("button", { name: "GW" })).toBeDisabled();
  });

  it.each([
    { nextControllers: ["121.830"] },
    { nextControllers: [] },
  ])("retains its colors and existing SI route $nextControllers when the controller is offline", ({ nextControllers }) => {
    state.controllers = [];
    state.strip = { owner: "119.905", next_controllers: nextControllers };
    render(
      <TransferControllerButton
        label="GW"
        station={TRANSFER_CONTROLLER_STATIONS.GW}
        className="bay-tab-btn"
      />,
    );

    const button = screen.getByRole("button", { name: "GW" });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("class", "bay-tab-btn");
    fireEvent.click(button);

    expect(state.transferStrip).not.toHaveBeenCalled();
    expect(state.selectNextController).not.toHaveBeenCalled();
    expect(state.strip.next_controllers).toEqual(nextControllers);
  });
});
