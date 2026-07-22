import {render, screen} from "@testing-library/react";
import {beforeEach, describe, expect, it, vi} from "vitest";

import type {WebSocketState} from "@/store/store";
import AMAN from "./AMAN";

const {controlsSpy, storeState} = vi.hoisted(() => ({
  controlsSpy: vi.fn(),
  storeState: {
    amanState: null,
    amanPresentationStatus: "empty",
    amanError: null,
    amanConnectionState: "connected",
  },
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: WebSocketState) => unknown) => selector(storeState as WebSocketState),
}));

vi.mock("@/components/aman/AMANBoard", () => ({
  AMANBoardView: () => <div>AMAN board</div>,
}));

vi.mock("@/components/aman/AMANControls", () => ({
  AMANControls: (props: {hasFMPAuthority: boolean}) => {
    controlsSpy(props);
    return <div>AMAN controls</div>;
  },
}));

vi.mock("@/lib/aman-performance", () => ({
  markAMANStateReceived: vi.fn(),
  measureAMANStatePaint: vi.fn(() => () => undefined),
}));

describe("AMAN route authorization", () => {
  beforeEach(() => controlsSpy.mockClear());

  it("keeps FMP controls unauthorized without a server-backed capability", () => {
    render(<AMAN />);

    expect(screen.getByText("AMAN board")).toBeInTheDocument();
    expect(screen.getByText("AMAN controls")).toBeInTheDocument();
    expect(controlsSpy).toHaveBeenCalledWith(expect.objectContaining({hasFMPAuthority: false}));
  });
});
