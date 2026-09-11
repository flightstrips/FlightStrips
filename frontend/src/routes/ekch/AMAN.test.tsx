import {render, screen} from "@testing-library/react";
import {beforeEach, describe, expect, it, vi} from "vitest";

import type {WebSocketState} from "@/store/store";
import type {AMANState} from "@/api/aman";
import AMAN from "./AMAN";

const {controlsSpy, holdingSpy, tmtSpy, storeState} = vi.hoisted(() => ({
  controlsSpy: vi.fn(),
  holdingSpy: vi.fn(),
  tmtSpy: vi.fn(),
  storeState: {
    amanState: null as AMANState | null,
    amanPresentationStatus: "empty",
    amanError: null,
    amanConnectionState: "connected",
    amanFMPAuthority: false,
  },
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: WebSocketState) => unknown) => selector(storeState as WebSocketState),
}));

vi.mock("@/components/aman/AMANBoard", () => ({
  AMANBoardView: ({onOpenControls}: {onOpenControls?: () => void}) => <button onClick={onOpenControls} type="button">AMAN board</button>,
}));

vi.mock("@/components/aman/AMANControls", () => ({
  AMANControls: (props: {hasFMPAuthority: boolean}) => {
    controlsSpy(props);
    return <div>AMAN controls</div>;
  },
}));

vi.mock("@/components/aman/TMTTrafficPrediction", () => ({
  TMTTrafficPrediction: (props: {prediction: unknown}) => {
    tmtSpy(props);
    return <div>TMT traffic</div>;
  },
}));

vi.mock("@/components/aman/TMTHoldingGraph", () => ({
  TMTHoldingGraph: (props: {entries: unknown}) => {
    holdingSpy(props);
    return <div>TMT holding</div>;
  },
}));

vi.mock("@/lib/aman-performance", () => ({
  markAMANStateReceived: vi.fn(),
  measureAMANStatePaint: vi.fn(() => () => undefined),
}));

describe("AMAN route authorization", () => {
  beforeEach(() => {
    controlsSpy.mockClear();
    holdingSpy.mockClear();
    tmtSpy.mockClear();
    storeState.amanState = null;
    storeState.amanFMPAuthority = false;
  });

  it("keeps FMP controls unauthorized without a server-backed capability", () => {
    render(<AMAN />);

    expect(screen.getByText("AMAN board")).toBeInTheDocument();
    expect(screen.getByText("AMAN controls")).toBeInTheDocument();
    expect(controlsSpy).toHaveBeenCalledWith(expect.objectContaining({hasFMPAuthority: false}));
  });

  it("enables FMP controls from the server-backed session capability", () => {
    storeState.amanFMPAuthority = true;
    render(<AMAN />);

    expect(controlsSpy).toHaveBeenCalledWith(expect.objectContaining({hasFMPAuthority: true}));
  });

  it("mounts TMT through the focused authoritative read-model seam", () => {
    const trafficPrediction = {status: "ready"};
    const holdingInformation = [{flight_id: "holding-1"}];
    storeState.amanState = {airport: "EKCH", revision: 1, generated_at: "2026-07-22T20:44:00.000Z", flights: [], traffic_prediction: trafficPrediction, holding_information: holdingInformation} as unknown as AMANState;
    render(<AMAN />);

    expect(screen.getByText("TMT traffic")).toBeInTheDocument();
    expect(screen.getByText("TMT holding")).toBeInTheDocument();
    expect(tmtSpy).toHaveBeenCalledWith({prediction: trafficPrediction});
    expect(holdingSpy).toHaveBeenCalledWith({entries: holdingInformation});
  });
});
