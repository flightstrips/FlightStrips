import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MessageComposeDialog } from "./MessageComposeDialog";

const state = vi.hoisted(() => ({
  sendMessage: vi.fn(),
  createTacticalStrip: vi.fn(),
  assignRunway: vi.fn(),
  controllers: [
    { callsign: "EKCH_A_GND", position: "121.630", identifier: "AA", section: "GND", owned_sectors: ["AA", "AD"] },
    { callsign: "EKCH_A_TWR", position: "118.105", identifier: "TW", section: "TWR", owned_sectors: ["TW", "TE", "GWA"] },
  ],
  position: "119.905",
  callsign: "EKCH_DEL",
}));
vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
  useSelectedCallsign: () => null,
}));
beforeEach(() => vi.clearAllMocks());

function click(name: string) { fireEvent.click(screen.getByRole("button", { name })); }
function preview() { return screen.getByRole("textbox", { name: "Message to send" }); }
function time(kind: "Start" | "End", digits: string) { for (const digit of digits) click(`${kind} time ${digit}`); }

describe("message composition", () => {
  it("combines presets, runway, status, free text and UTC timings into a read-only preview", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("RWY");
    click("04R");
    click("+ CLOSED");
    click("CLOSING POSITION SOON");
    fireEvent.change(screen.getByRole("textbox", { name: "Additional free text" }), { target: { value: "Expect delays" } });
    time("Start", "1450");
    time("End", "1600");
    expect(preview()).toHaveValue("RWY 04R Closed CLOSING POSITION SOON Expect delays STARTING AT 1450, until 1600");
    expect(preview()).toHaveAttribute("readonly");
    click("BROADCAST");
    click("OK");
    expect(state.sendMessage).toHaveBeenCalledWith("RWY 04R Closed CLOSING POSITION SOON Expect delays STARTING AT 1450, until 1600", [], "AUTO");
  });

  it("selects only one taxiway and commits it only on OK", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("TWY");
    const picker = screen.getByRole("dialog", { name: "Select taxiway" });
    fireEvent.click(within(picker).getByRole("button", { name: "A1" }));
    fireEvent.click(within(picker).getByRole("button", { name: "B2" }));
    expect(within(picker).getByRole("button", { name: "A1" })).toHaveAttribute("aria-pressed", "false");
    expect(within(picker).getByRole("button", { name: "B2" })).toHaveAttribute("aria-pressed", "true");
    fireEvent.click(within(picker).getByRole("button", { name: "OK" }));
    expect(preview()).toHaveValue("TWY B2");
    click("+ OUT OF USE");
    expect(preview()).toHaveValue("TWY B2 Out of use");
    click("TWY");
    fireEvent.click(within(screen.getByRole("dialog", { name: "Select taxiway" })).getByRole("button", { name: "ESC" }));
    expect(preview()).toHaveValue("TWY B2 Out of use");
  });

  it("disables offline stations and enables cross-coupled coverage, like manual transfer", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    expect(screen.getByRole("button", { name: "GND EAST" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "CLR-DEL" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "SEQ-PLN" })).toBeDisabled();
    for (const name of ["APRON ARR", "APRON DEP", "TWR WEST", "TWR EAST", "GND WEST"]) {
      expect(screen.getByRole("button", { name })).toBeEnabled();
    }
  });

  it.each(["SMALL", "AUTO", "LARGE"])("sends the %s format choice with the selected recipient", (format) => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("APRON ARR");
    click("CLOSING POSITION SOON");
    click(format);
    click("OK");
    expect(state.sendMessage).toHaveBeenCalledWith("CLOSING POSITION SOON", ["APRON ARR"], format);
  });

  it("broadcast clears recipient selection", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("APRON ARR");
    click("BROADCAST");
    expect(screen.getByRole("button", { name: "APRON ARR" })).toHaveAttribute("aria-pressed", "false");
    click("CLOSING POSITION SOON");
    click("OK");
    expect(state.sendMessage).toHaveBeenCalledWith("CLOSING POSITION SOON", [], "AUTO");
  });

  it("ERASE removes the last word, DELETE MESSAGE clears all content and times, and ESC does not send", () => {
    const onClose = vi.fn();
    render(<MessageComposeDialog open onClose={onClose} />);
    click("CLOSING POSITION SOON");
    fireEvent.change(screen.getByRole("textbox", { name: "Additional free text" }), { target: { value: "Expect delays" } });
    click("Erase last word");
    expect(preview()).toHaveValue("CLOSING POSITION SOON Expect");
    click("Erase last word");
    click("Erase last word");
    expect(preview()).toHaveValue("CLOSING POSITION");
    time("Start", "1450");
    click("End time UFN");
    click("DELETE MESSAGE");
    expect(preview()).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "Start time" })).toHaveValue("");
    expect(screen.getByRole("textbox", { name: "End time" })).toHaveValue("");
    expect(screen.getByRole("button", { name: "OK" })).toBeDisabled();
    click("ESC");
    expect(onClose).toHaveBeenCalledOnce();
    expect(state.sendMessage).not.toHaveBeenCalled();
  });

  it("supports NOW, UFN and erasing each time independently", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-10-12T14:50:00Z"));
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("CLOSING POSITION SOON");
    click("Start time NOW");
    click("End time UFN");
    expect(preview()).toHaveValue("CLOSING POSITION SOON STARTING AT 1450, until UFN");
    click("Start time clear");
    expect(preview()).toHaveValue("CLOSING POSITION SOON until UFN");
    click("End time clear");
    expect(preview()).toHaveValue("CLOSING POSITION SOON");
    vi.useRealTimers();
  });

  it.each(["14", "2460", "2360"])("reports invalid time %s instead of sending", (digits) => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("CLOSING POSITION SOON");
    time("Start", digits);
    expect(screen.getByRole("alert")).toHaveTextContent("Enter complete UTC times");
    expect(screen.getByRole("button", { name: "OK" })).toBeDisabled();
    expect(state.sendMessage).not.toHaveBeenCalled();
  });

  it("does not silently truncate combinations exceeding the message limit", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    click("CLOSING POSITION SOON");
    fireEvent.change(screen.getByRole("textbox", { name: "Additional free text" }), { target: { value: "a".repeat(120) } });
    expect(screen.getByRole("alert")).toHaveTextContent("Message exceeds 120 characters");
    expect(screen.getByRole("button", { name: "OK" })).toBeDisabled();
  });
});
