import { fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, Bay, CommunicationType } from "@/api/models";
import { PushbackMapDialog } from "./PushbackMapDialog";

const state = vi.hoisted(() => ({
  strip: { communication_type: "T" },
  sendFrequencyMessage: vi.fn(), openTextPilotMenu: vi.fn(), openValidationDialog: vi.fn(),
  setReleasePoint: vi.fn(), move: vi.fn(),
}));
vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (s: typeof state) => unknown) => selector(state),
  useStrip: () => state.strip,
}));
vi.mock("./MapDialogShell", () => ({
  MAP_BTN_BASE: {}, MapEraseControls: () => null,
  MapDialogShell: ({ open, onSelect, onOpenChange }: { open: boolean; onSelect: (s: string) => void; onOpenChange: (s: boolean) => void }) => open && <>
    <button onClick={() => onSelect("J4")}>J4</button>
    <button onClick={() => onOpenChange(false)}>Close</button>
  </>,
}));
beforeEach(() => {
  vi.clearAllMocks();
  state.strip.communication_type = CommunicationType.Text;
  state.sendFrequencyMessage.mockResolvedValue({ prepared: true, submitted: false, applied_actions: 0 });
});
describe("pushback text preparation", () => {
  it("waits for backend validation before opening the composer, without moving the strip", async () => {
    let resolve!: (value: unknown) => void;
    state.sendFrequencyMessage.mockReturnValueOnce(new Promise(r => { resolve = r; }));
    render(<PushbackMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);
    fireEvent.click(screen.getByText("J4"));
    expect(state.openTextPilotMenu).not.toHaveBeenCalled();
    expect(state.sendFrequencyMessage).toHaveBeenCalledWith("SAS123", "", [{ type: ActionType.FrontendMove, callsign: "SAS123", bay: Bay.Push }], false, true);
    await act(async () => resolve({ prepared: true }));
    expect(state.openTextPilotMenu).toHaveBeenCalledWith(expect.objectContaining({ variant: "push", point: "J4" }));
    expect(state.move).not.toHaveBeenCalled();
    expect(state.setReleasePoint).not.toHaveBeenCalled();
  });
  it("opens validation instead of the composer until the backend accepts a retry", async () => {
    const onOpenChange = vi.fn();
    state.sendFrequencyMessage.mockResolvedValueOnce({ prepared: false, validation_blocked: true });
    const view = render(<PushbackMapDialog open onOpenChange={onOpenChange} callsign="SAS123" />);
    fireEvent.click(screen.getByText("J4"));
    await waitFor(() => expect(state.openValidationDialog).toHaveBeenCalledWith("SAS123"));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(state.openTextPilotMenu).not.toHaveBeenCalled();
    view.rerender(<PushbackMapDialog open={false} onOpenChange={onOpenChange} callsign="SAS123" />);
    view.rerender(<PushbackMapDialog open onOpenChange={onOpenChange} callsign="SAS123" />);
    fireEvent.click(screen.getByText("J4"));
    await waitFor(() => expect(state.openTextPilotMenu).toHaveBeenCalledOnce());
  });
  it("does not open a composer when the map is closed during validation", async () => {
    let resolve!: (value: unknown) => void;
    state.sendFrequencyMessage.mockReturnValueOnce(new Promise(r => { resolve = r; }));
    render(<PushbackMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);
    fireEvent.click(screen.getByText("J4"));
    fireEvent.click(screen.getByText("Close"));
    await act(async () => resolve({ prepared: true }));
    expect(state.openTextPilotMenu).not.toHaveBeenCalled();
  });
  it("keeps the existing voice pilot move flow", () => {
    state.strip.communication_type = CommunicationType.Voice;
    render(<PushbackMapDialog open onOpenChange={vi.fn()} callsign="SAS123" />);
    fireEvent.click(screen.getByText("J4"));
    expect(state.setReleasePoint).toHaveBeenCalledWith("SAS123", "J4");
    expect(state.move).toHaveBeenCalledWith("SAS123", Bay.Push);
    expect(state.sendFrequencyMessage).not.toHaveBeenCalled();
  });
});
