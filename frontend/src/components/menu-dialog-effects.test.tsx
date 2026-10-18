import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MessageComposeDialog } from "./MessageComposeDialog";
import { MemaidDialog } from "./strip/MemaidDialog";
import { MENU_BUTTON_BEVEL, MENU_INPUT_BEVEL } from "./ui/menuControlStyle";

const actions = vi.hoisted(() => ({
  createTacticalStrip: vi.fn(),
  sendMessage: vi.fn(),
  controllers: [{ callsign: "EKCH_A_GND", position: "121.630", identifier: "AA", section: "GND", owned_sectors: ["AA"] }],
  position: "119.905",
  callsign: "EKCH_DEL",
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: typeof actions) => unknown) => selector(actions),
  useSelectedCallsign: () => "SAS123",
}));

function expectControlBevels() {
  for (const button of screen.getAllByRole("button").filter(button => button.textContent?.trim() !== "Close")) {
    expect(button.style.boxShadow).toBe(MENU_BUTTON_BEVEL.boxShadow);
    expect(button.style.borderTopColor).toBe("rgb(206, 206, 206)");
    expect(button.style.borderRightColor).toBe("rgb(57, 57, 57)");
  }
  for (const input of screen.getAllByRole("textbox")) {
    expect(input.style.boxShadow).toBe(MENU_INPUT_BEVEL.boxShadow);
    expect(input.style.borderTopColor).toBe("rgb(57, 57, 57)");
    expect(input.style.borderRightColor).toBe("rgb(206, 206, 206)");
  }
}

beforeEach(() => vi.clearAllMocks());

describe("menu dialog bevels", () => {
  it("styles every MEM AID control while preserving preset submission", () => {
    const onOpenChange = vi.fn();
    render(<MemaidDialog open bay="TWY-DEP" onOpenChange={onOpenChange} />);
    expectControlBevels();
    fireEvent.click(screen.getByRole("button", { name: "TAXI VIA K3" }));
    expect(screen.getByRole("textbox")).toHaveValue("TAXI VIA K3");
    expectControlBevels();
    fireEvent.click(screen.getByRole("button", { name: "OK" }));
    expect(actions.createTacticalStrip).toHaveBeenCalledWith("MEMAID", "TWY-DEP", "TAXI VIA K3", "SAS123");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("styles every MESSAGES control while preserving recipient and message selection", () => {
    const onClose = vi.fn();
    render(<MessageComposeDialog open onClose={onClose} />);
    expectControlBevels();
    fireEvent.click(screen.getByRole("button", { name: "APRON ARR" }));
    fireEvent.click(screen.getByRole("button", { name: "CLOSING POSITION SOON" }));
    expectControlBevels();
    fireEvent.click(screen.getByRole("button", { name: "OK" }));
    expect(actions.sendMessage).toHaveBeenCalledWith("CLOSING POSITION SOON", ["APRON ARR"], "AUTO");
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("erases the last word of the composed message", () => {
    render(<MessageComposeDialog open onClose={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "CLOSING POSITION SOON" }));
    fireEvent.click(screen.getByRole("button", { name: "Erase last word" }));
    expect(screen.getByRole("textbox", { name: "Message to send" })).toHaveValue("CLOSING POSITION");
    expect(actions.sendMessage).not.toHaveBeenCalled();
  });
});
