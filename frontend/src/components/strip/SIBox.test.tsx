import { fireEvent, render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SIBox } from "./SIBox";

const transferStrip = vi.hoisted(() => vi.fn());
const startRequestAndTransfer = vi.hoisted(() => vi.fn().mockResolvedValue(true));
const openStripContextMenu = vi.hoisted(() => vi.fn());

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useWebSocketStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    transferStrip,
    startRequestAndTransfer,
    assumeStrip: vi.fn(),
    openStripContextMenu,
    acceptTagRequest: vi.fn(),
  }),
}));

describe("startup SI transfer", () => {
  beforeEach(() => {
    transferStrip.mockClear();
    startRequestAndTransfer.mockClear();
    openStripContextMenu.mockClear();
  });

  it("waits for the TSAT window and sends a startup request with the transfer", () => {
    const allowed = vi.fn(() => false);
    const { container } = render(
      <SIBox
        callsign="SAS123"
        bay="CLEARED"
        owner="121.905"
        nextControllers={["121.730"]}
        myPosition="121.905"
        transferAllowed={allowed}
        startRequestTransfer
      />,
    );
    const si = container.firstElementChild!;
    fireEvent.click(si);
    expect(allowed).toHaveBeenCalledOnce();
    expect(transferStrip).not.toHaveBeenCalled();
    expect(startRequestAndTransfer).not.toHaveBeenCalled();

    allowed.mockReturnValue(true);
    fireEvent.click(si);
    expect(startRequestAndTransfer).toHaveBeenCalledWith("SAS123");
    expect(transferStrip).not.toHaveBeenCalled();
  });

  it("keeps ordinary SI transfers on the normal action", () => {
    const { container } = render(
      <SIBox callsign="SAS123" bay="CLEARED" owner="121.905" nextControllers={["121.730"]} myPosition="121.905" />,
    );

    fireEvent.click(container.firstElementChild!);
    expect(transferStrip).toHaveBeenCalledWith("SAS123", "121.730");
    expect(startRequestAndTransfer).not.toHaveBeenCalled();
  });

  it("opens the callsign menu instead of changing handover state while a transfer is active", () => {
    const { container } = render(
      <SIBox
        callsign="SAS123"
        bay="CLEARED"
        owner="121.905"
        myPosition="121.905"
        transferFrom="121.905"
        transferringTo="121.730"
      />,
    );

    fireEvent.click(container.firstElementChild!, { clientX: 120, clientY: 80 });

    expect(openStripContextMenu).toHaveBeenCalledWith("SAS123", { x: 120, y: 80 });
    expect(transferStrip).not.toHaveBeenCalled();
  });
});
