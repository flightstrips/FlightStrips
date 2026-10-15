import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ActionType, Bay } from "@/api/models";
import { TextPilotDialog } from "./TextPilotDialog";
import type { TextPilotDraft } from "@/lib/text-pilot";

const state = vi.hoisted(() => ({
  textPilotDraft: null as TextPilotDraft | null,
  strips: [{ callsign: "SAS123", runway: "22R", communication_type: "T", owner: "121.905", stand: undefined as string | undefined }],
  metar: "EKCH 101220Z 24012KT CAVOK 12/10 Q1013",
  controllers: [{ callsign: "EKCH_AD_GND", position: "121.730", owned_sectors: ["AD"] }],
  position: "121.905", stripTransfers: {} as Record<string, { from?: string; to?: string }>,
  closeTextPilotMenu: vi.fn(), markTextPilotReadback: vi.fn(), sendFrequencyMessage: vi.fn(),
}));
vi.mock("@/store/store-hooks", () => ({ useWebSocketStore: (selector: (s: typeof state) => unknown) => selector(state) }));

beforeEach(() => {
  vi.clearAllMocks(); state.stripTransfers = {}; state.strips[0].stand = undefined;
  state.sendFrequencyMessage.mockResolvedValue({ submitted: true, applied_actions: 0 });
  state.textPilotDraft = { callsign: "SAS123", variant: "apron-out", point: "A/B", actions: [{ type: ActionType.FrontendMove, callsign: "SAS123", bay: Bay.Taxi }] };
});
describe("text pilot composer", () => {
  it("combines push letters and digits and toggles a direction off", () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "J4" };
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: "P" }));
    expect(screen.getByRole("textbox")).toHaveValue("Push and start approved, Release point P4.");
    fireEvent.click(screen.getByRole("button", { name: "2" }));
    expect(screen.getByRole("textbox")).toHaveValue("Push and start approved, Release point P2.");
    fireEvent.click(screen.getByRole("button", { name: "NORTH" }));
    expect(screen.getByRole("textbox")).toHaveValue("Push and start approved, Face North.");
    fireEvent.click(screen.getByRole("button", { name: "NORTH" }));
    expect(screen.getByRole("textbox")).toHaveValue("Push and start approved,");
  });
  it("sends the edited push point with the matching strip update", async () => {
    const move = { type: ActionType.FrontendMove, callsign: "SAS123", bay: Bay.Push } as const;
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "J4", actions: [
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "J4" }, move,
    ] };
    state.sendFrequencyMessage.mockResolvedValueOnce({ submitted: false, applied_actions: 1, error: "Input busy" });
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: "P" }));
    fireEvent.click(screen.getByRole("button", { name: "2" }));
    expect(state.sendFrequencyMessage).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Input busy");
    expect(screen.getByRole("button", { name: "J" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "2" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "NORTH" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "ERASE" })).toBeDisabled();
    expect(screen.getByRole("textbox")).toBeDisabled();
    expect(state.sendFrequencyMessage.mock.calls[0]).toEqual([
      "SAS123", "Push and start approved, Release point P2.",
      [{ type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "P2" }, move], false,
    ]);
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[1][2]).toEqual([move]);
  });
  it.each([["NORTH", "N"], ["EAST", "E"], ["WEST", "W"], ["SOUTH", "S"]])("stages %s using the map direction code", async (label, point) => {
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "J4", actions: [
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "J4" },
    ] };
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: label }));
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[0][2]).toEqual([
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: point },
    ]);
  });
  it("does not retain the original push point after its direction is deselected", async () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "N", actions: [
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "N" },
    ] };
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: "NORTH" }));
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[0][1]).toBe("Push and start approved,");
    expect(state.sendFrequencyMessage.mock.calls[0][2]).toEqual([
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "" },
    ]);
  });
  it("shows and removes the initial push letter and digit", () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "J4" };
    render(<TextPilotDialog />);
    expect(screen.getByRole("button", { name: "J" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "4" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(screen.getByRole("button", { name: "J" }));
    expect(screen.getByRole("textbox")).toHaveValue("Push and start approved, Release point 4.");
    expect(screen.getByRole("button", { name: "J" })).toHaveAttribute("aria-pressed", "false");
  });
  it("offers the apron-specific hold and continue instructions", () => {
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: "HOLD SHORT RWY 22L" }));
    expect(screen.getByRole("textbox")).toHaveProperty("value", expect.stringContaining("Hold short RWY 22L."));
    fireEvent.click(screen.getByRole("button", { name: "CONTINUE VIA" }));
    expect(screen.getByRole("textbox")).toHaveProperty("value", expect.stringContaining("Continue via"));
  });
  it("inserts taxiways before the selected limit and keeps edits", () => {
    render(<TextPilotDialog />);
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via TWY A, Hold short of TWY B.");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Taxi via custom TWY A, Hold short of TWY B." } });
    fireEvent.click(screen.getByRole("button", { name: "J" }));
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via custom TWY J, TWY A, Hold short of TWY B.");
    fireEvent.click(screen.getByRole("button", { name: "J" }));
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via custom TWY A, Hold short of TWY B.");
  });
  it("renders the tower-out export and distinguishes taxiways from holding points", () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "tower-out", point: "A1", holdingPoint: true };
    render(<TextPilotDialog />);
    expect(screen.getByRole("button", { name: "A1" })).toHaveAttribute("aria-pressed", "true");
    const k2 = screen.getAllByRole("button", { name: "K2" });
    fireEvent.click(k2[0]);
    expect(screen.getByRole("textbox")).toHaveProperty("value", expect.stringContaining("TWY K2,"));
    fireEvent.click(k2[1]);
    expect(screen.getByRole("textbox")).toHaveProperty("value", expect.stringContaining("for Holding point K2, Runway 22R."));
    expect(k2[0]).toHaveAttribute("aria-pressed", "true");
    expect(k2[1]).toHaveAttribute("aria-pressed", "true");
    const crossings = screen.getAllByRole("button", { name: "CROSS RWY 04R/22L" });
    fireEvent.click(crossings[0]);
    expect(crossings[1]).toHaveAttribute("aria-pressed", "true");
  });
  it("discards staged actions on ESC", () => {
    render(<TextPilotDialog />); fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    expect(state.closeTextPilotMenu).toHaveBeenCalledOnce(); expect(state.sendFrequencyMessage).not.toHaveBeenCalled();
  });
  it("retains text on failure and retries without already-applied actions", async () => {
    state.sendFrequencyMessage.mockResolvedValueOnce({ submitted: false, applied_actions: 1, error: "Input busy" });
    render(<TextPilotDialog />); fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Input busy");
    expect(state.closeTextPilotMenu).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[1][2]).toEqual([]);
  });
  it("highlights CONTACT with a station and clears it on deselection", () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "si" };
    render(<TextPilotDialog />);
    const contact = screen.getByText("CONTACT");
    expect(contact).toHaveAttribute("data-selected", "false");
    fireEvent.click(screen.getByRole("button", { name: /KASTRUP APRON.*121.730/ }));
    expect(contact).toHaveAttribute("data-selected", "true");
    fireEvent.click(screen.getByRole("button", { name: /KASTRUP APRON.*121.730/ }));
    expect(contact).toHaveAttribute("data-selected", "false");
  });
  it.each(["apron-in", "tower-in"] as const)("toggles the assigned stand in %s without losing manual text", variant => {
    state.textPilotDraft = { callsign: "SAS123", variant, stand: "A18" };
    render(<TextPilotDialog />);
    expect(screen.getByRole("button", { name: "A18" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "Taxi via custom to stand A18." } });
    fireEvent.click(screen.getByRole("button", { name: "A18" }));
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via custom");
    fireEvent.click(screen.getByRole("button", { name: "A18" }));
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via custom to stand A18.");
  });
  it("uses the strip stand when taxi-in opens from a hold-short map", () => {
    state.strips[0].stand = "A18";
    state.textPilotDraft = { callsign: "SAS123", variant: "apron-in", point: "A/B" };
    render(<TextPilotDialog />);
    expect(screen.getByRole("button", { name: "A18" })).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("textbox")).toHaveValue("Taxi via TWY A, Hold short of TWY B.");
    fireEvent.click(screen.getByRole("button", { name: "A18" }));
    expect(screen.getByRole("textbox")).toHaveProperty("value", expect.stringContaining("to stand A18."));
  });
  it("marks readback and transfers only after SEND succeeds", async () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "si", initialSelected: ["readback", "freq:121.730"], readbackKey: "EKCH_GND:SAS123" };
    state.sendFrequencyMessage.mockResolvedValueOnce({ submitted: false, applied_actions: 0, error: "Disconnected" });
    render(<TextPilotDialog />); expect(state.sendFrequencyMessage).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "SEND" })); await screen.findByRole("alert");
    expect(state.markTextPilotReadback).not.toHaveBeenCalled();
    expect(state.sendFrequencyMessage.mock.calls[0][2]).toEqual([{ type: ActionType.FrontendCoordinationTransferRequest, callsign: "SAS123", to: "121.730" }]);
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.markTextPilotReadback).toHaveBeenCalledWith("EKCH_GND:SAS123"));
  });
  it("locks station and message edits after an SI transfer succeeds but submission fails", async () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "si", initialSelected: ["readback", "freq:121.730"] };
    state.sendFrequencyMessage.mockResolvedValueOnce({ submitted: false, applied_actions: 1, error: "Input busy" });
    render(<TextPilotDialog />);
    const message = (screen.getByRole("textbox") as HTMLTextAreaElement).value;
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await screen.findByRole("alert");
    const otherStation = screen.getByRole("button", { name: /KASTRUP TOWER.*118.580/ });
    expect(otherStation).toBeDisabled();
    expect(screen.getByRole("button", { name: "REMAIN ON THIS FREQUENCY." })).toBeDisabled();
    expect(screen.getByRole("button", { name: "ERASE" })).toBeDisabled();
    expect(screen.getByRole("textbox")).toBeDisabled();
    fireEvent.click(otherStation);
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[1]).toEqual(["SAS123", message, [], false]);
  });
  it("allows instruction edits after a rejection that applied no actions", async () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "push", point: "J4", actions: [
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "J4" },
    ] };
    state.sendFrequencyMessage.mockResolvedValueOnce({ submitted: false, applied_actions: 0, error: "Disconnected" });
    render(<TextPilotDialog />);
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await screen.findByRole("alert");
    expect(screen.getByRole("textbox")).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "P" }));
    fireEvent.click(screen.getByRole("button", { name: "2" }));
    fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalledOnce());
    expect(state.sendFrequencyMessage.mock.calls[1][2]).toEqual([
      { type: ActionType.FrontendReleasePoint, callsign: "SAS123", release_point: "P2" },
    ]);
  });
  it("cancels an outgoing transfer only with REMAIN on SEND", async () => {
    state.stripTransfers.SAS123 = { from: "121.905", to: "121.730" };
    state.textPilotDraft = { callsign: "SAS123", variant: "si", initialSelected: ["remain"] };
    render(<TextPilotDialog />); fireEvent.click(screen.getByRole("button", { name: "SEND" }));
    await waitFor(() => expect(state.closeTextPilotMenu).toHaveBeenCalled());
    expect(state.sendFrequencyMessage.mock.calls[0][2]).toEqual([{ type: ActionType.FrontendCoordinationCancelTransferRequest, callsign: "SAS123" }]);
  });
  it("opens accepted green runway defaults and ESC sends no reversal", () => {
    state.textPilotDraft = { callsign: "SAS123", variant: "runway", arrival: true, confirmed: true };
    render(<TextPilotDialog />);
    expect(screen.getByRole("textbox")).toHaveValue("Wind 240 degrees, 12 knots. Cleared to land RWY 22R,");
    fireEvent.click(screen.getByRole("button", { name: "ESC" })); expect(state.sendFrequencyMessage).not.toHaveBeenCalled();
  });
});
