import { fireEvent, render } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FrontendController } from "@/api/models";
import { SIBox } from "./SIBox";

const state = vi.hoisted(() => ({
  strips: [] as { callsign: string; communication_type: string }[],
  controllers: [] as FrontendController[],
  textPilotReadbacks: {} as Record<string, boolean>, layout: "", displayedLayout: "", callsign: "EKCH_GND",
}));
const openTextPilotMenu = vi.hoisted(() => vi.fn());
const assumeStrip = vi.hoisted(() => vi.fn());
const transferStrip = vi.hoisted(() => vi.fn());
const startRequestAndTransfer = vi.hoisted(() => vi.fn().mockResolvedValue(true));
const openStripContextMenu = vi.hoisted(() => vi.fn());

vi.mock("@/store/store-hooks", () => ({
  useControllers: () => state.controllers,
  useWebSocketStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    ...state,
    openTextPilotMenu,
    transferStrip,
    startRequestAndTransfer,
    assumeStrip,
    openStripContextMenu,
    acceptTagRequest: vi.fn(),
  }),
}));

describe("startup SI transfer", () => {
  beforeEach(() => {
    state.strips = []; state.controllers = []; state.layout = ""; state.textPilotReadbacks = {};
    openTextPilotMenu.mockClear(); assumeStrip.mockClear();
    transferStrip.mockClear();
    startRequestAndTransfer.mockClear();
    openStripContextMenu.mockClear();
  });

  it("uses the reduced font size for the SI indicator", () => {
    const { container } = render(<SIBox callsign="SAS123" />);

    expect((container.firstElementChild as HTMLElement).style.fontSize).toBe("0.98vw");
  });

  it("sends a startup request with the transfer for backend TSAT validation", () => {
    const { container } = render(
      <SIBox
        callsign="SAS123"
        bay="CLEARED"
        owner="121.905"
        nextControllers={["121.730"]}
        myPosition="121.905"
        startRequestTransfer
      />,
    );
    const si = container.firstElementChild!;
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

describe("text-pilot SI entry points", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    state.strips = [{ callsign: "SAS123", communication_type: "T" }];
    state.controllers = []; state.layout = ""; state.textPilotReadbacks = {};
  });
  it.each(["121.905", "121.730", ""])("opens the text menu with owner=%s without an immediate transfer", owner => {
    const { container } = render(<SIBox callsign="SAS123" owner={owner} myPosition="121.905" nextControllers={["121.730"]} />);
    fireEvent.click(container.firstElementChild!);
    expect(openTextPilotMenu).toHaveBeenCalledWith(expect.objectContaining({ variant: "si" }));
    expect(transferStrip).not.toHaveBeenCalled(); expect(assumeStrip).not.toHaveBeenCalled();
  });
  it("opens the callsign menu for an incoming handover without opening the text menu", () => {
    const { container } = render(<SIBox callsign="SAS123" owner="121.730" myPosition="121.905" transferFrom="121.730" transferringTo="121.905" />);
    fireEvent.click(container.firstElementChild!);
    expect(openStripContextMenu).toHaveBeenCalledWith("SAS123", { x: 0, y: 0 });
    expect(assumeStrip).not.toHaveBeenCalled();
    expect(openTextPilotMenu).not.toHaveBeenCalled();
  });
  it("opens the callsign menu for an outgoing text-pilot handover", () => {
    const { container } = render(<SIBox callsign="SAS123" owner="121.905" myPosition="121.905" transferFrom="121.905" transferringTo="121.730" />);
    fireEvent.click(container.firstElementChild!);
    expect(openStripContextMenu).toHaveBeenCalledWith("SAS123", { x: 0, y: 0 });
    expect(openTextPilotMenu).not.toHaveBeenCalled();
    expect(transferStrip).not.toHaveBeenCalled();
  });
  it("preselects readback only on the first SEQ/PLN submission", () => {
    state.layout = "SEQPLN";
    const props = { callsign: "SAS123", owner: "121.905", myPosition: "121.905", nextControllers: ["121.730"] };
    const { container, rerender } = render(<SIBox {...props} />);
    fireEvent.click(container.firstElementChild!);
    expect(openTextPilotMenu).toHaveBeenLastCalledWith(expect.objectContaining({ initialSelected: ["readback"], readbackKey: "EKCH_GND:SAS123" }));
    state.textPilotReadbacks["EKCH_GND:SAS123"] = true;
    rerender(<SIBox {...props} />); fireEvent.click(container.firstElementChild!);
    expect(openTextPilotMenu).toHaveBeenLastCalledWith(expect.objectContaining({ initialSelected: ["freq:121.730"] }));
  });
  it("preselects readback and covered apron in delivery's cleared bay", () => {
    state.layout = "CLX";
    state.controllers = [{ callsign: "EKCH_TWR", position: "118.105", owned_sectors: ["SEQ"] }] as FrontendController[];
    const { container } = render(<SIBox callsign="SAS123" bay="CLEARED" owner="121.905" myPosition="121.905" />);
    fireEvent.click(container.firstElementChild!);
    expect(openTextPilotMenu).toHaveBeenCalledWith(expect.objectContaining({ initialSelected: ["readback", "freq:121.905"] }));
  });
});
