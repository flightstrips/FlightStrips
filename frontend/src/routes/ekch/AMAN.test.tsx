import {fireEvent, render, screen} from "@testing-library/react";
import {beforeEach, describe, expect, it, vi} from "vitest";

import type {WebSocketState} from "@/store/store";
import type {AMANState} from "@/api/aman";
import AMAN from "./AMAN";

const {boardSpy, controlsSpy, detailSpy, holdingSpy, tmtSpy, storeState} = vi.hoisted(() => ({
  boardSpy: vi.fn(),
  controlsSpy: vi.fn(),
  detailSpy: vi.fn(),
  holdingSpy: vi.fn(),
  tmtSpy: vi.fn(),
  storeState: {
    amanState: null as AMANState | null,
    amanPresentationStatus: "empty",
    amanError: null,
    amanConnectionState: "connected",
    amanFMPAuthority: false,
    amanWarnings: {items: [], snapshot: "available"},
  },
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: WebSocketState) => unknown) => selector(storeState as unknown as WebSocketState),
}));

vi.mock("@/components/aman/AMANBoard", () => ({
  AMANBoardView: (props: {state: AMANState | null; selectedFlightID: string | null; onOpenControls?: () => void; onOpenFlightDetails?: (flightID: string) => void}) => {
    boardSpy(props);
    return <><button onClick={props.onOpenControls} type="button">AMAN board</button><button onClick={() => props.onOpenFlightDetails?.("flight-123")} type="button">Open target</button></>;
  },
}));

vi.mock("@/components/aman/AMANFlightDetailDialog", () => ({
  AMANFlightDetailDialog: (props: {airport: string; flightID: string; onClose: () => void}) => {
    detailSpy(props);
    return <button onClick={props.onClose} type="button">Close mocked detail</button>;
  },
}));

vi.mock("@/components/aman/AMANControls", () => ({
  AMANControls: (props: {hasFMPAuthority: boolean}) => {
    controlsSpy(props);
    return <div>AMAN controls</div>;
  },
}));

vi.mock("@/components/aman/AMANWarningPanel", () => ({
  AMANWarningPanel: ({onNavigateToFlight}: {onNavigateToFlight: (flightID: string) => boolean}) => <div>AMAN warnings<button onClick={() => onNavigateToFlight("flight-123")} type="button">Warning primary</button><button onClick={() => onNavigateToFlight("flight-456")} type="button">Warning related</button><button onClick={(event) => { if (!onNavigateToFlight("missing")) event.currentTarget.focus(); }} type="button">Warning missing</button></div>,
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
    boardSpy.mockClear();
    holdingSpy.mockClear();
    tmtSpy.mockClear();
    detailSpy.mockClear();
    storeState.amanState = null;
    storeState.amanFMPAuthority = false;
  });

  it("keeps FMP controls unauthorized without a server-backed capability", () => {
    render(<AMAN />);

    expect(screen.getByRole("main", {name: "Arrival management workspace"})).toBeInTheDocument();
    expect(screen.getByRole("region", {name: "MAESTRO sequence workspace"})).toContainElement(screen.getByText("AMAN board"));
    expect(screen.getByRole("complementary", {name: "TMT analysis area"})).toContainElement(screen.getByText("AMAN controls"));
    expect(screen.getByText("AMAN board")).toBeInTheDocument();
    expect(screen.getByText("AMAN controls")).toBeInTheDocument();
    expect(screen.getByText("AMAN warnings")).toBeInTheDocument();
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

  it("opens and closes the existing detail view for the activated target", () => {
    storeState.amanState = {airport: "EKCH", revision: 1, generated_at: "2026-07-22T20:44:00.000Z", flights: [{flight_id: "flight-123"}]} as unknown as AMANState;
    render(<AMAN />);

    fireEvent.click(screen.getByRole("button", {name: "Open target"}));
    expect(detailSpy).toHaveBeenCalledWith(expect.objectContaining({airport: "EKCH", flightID: "flight-123"}));
    fireEvent.click(screen.getByRole("button", {name: "Close mocked detail"}));
    expect(screen.queryByRole("button", {name: "Close mocked detail"})).not.toBeInTheDocument();
  });

  it("selects primary and related warning flights by authoritative identity without a command", () => {
    storeState.amanState = {airport: "EKCH", revision: 1, generated_at: "2026-07-22T20:44:00.000Z", flights: [{flight_id: "flight-123"}, {flight_id: "flight-456"}]} as unknown as AMANState;
    render(<AMAN />);

    fireEvent.click(screen.getByRole("button", {name: "Warning primary"}));
    expect(boardSpy).toHaveBeenLastCalledWith(expect.objectContaining({selectedFlightID: "flight-123"}));
    fireEvent.click(screen.getByRole("button", {name: "Warning related"}));
    expect(boardSpy).toHaveBeenLastCalledWith(expect.objectContaining({selectedFlightID: "flight-456"}));
  });

  it("leaves selection and focus stable when a warning references an absent flight", () => {
    storeState.amanState = {airport: "EKCH", revision: 1, generated_at: "2026-07-22T20:44:00.000Z", flights: [{flight_id: "flight-123"}]} as unknown as AMANState;
    render(<AMAN />);
    const missing = screen.getByRole("button", {name: "Warning missing"});
    missing.focus();
    fireEvent.click(missing);

    expect(missing).toHaveFocus();
    expect(boardSpy).toHaveBeenLastCalledWith(expect.objectContaining({selectedFlightID: "flight-123"}));
  });
});
