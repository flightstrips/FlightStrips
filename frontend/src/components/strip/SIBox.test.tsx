import { fireEvent, render } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SIBox } from "./SIBox";

const transferStrip = vi.hoisted(() => vi.fn());

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => [],
  useWebSocketStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    transferStrip,
    assumeStrip: vi.fn(),
    cancelTransfer: vi.fn(),
    acceptTagRequest: vi.fn(),
  }),
}));

describe("startup SI transfer", () => {
  it("waits for the TSAT window and transfers to the stand route target", () => {
    const allowed = vi.fn(() => false);
    const { container } = render(
      <SIBox
        callsign="SAS123"
        bay="CLEARED"
        owner="121.905"
        nextControllers={["121.730"]}
        myPosition="121.905"
        transferAllowed={allowed}
      />,
    );
    const si = container.firstElementChild!;
    fireEvent.click(si);
    expect(allowed).toHaveBeenCalledOnce();
    expect(transferStrip).not.toHaveBeenCalled();

    allowed.mockReturnValue(true);
    fireEvent.click(si);
    expect(transferStrip).toHaveBeenCalledWith("SAS123", "121.730");
  });
});
