import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MessageReceived } from "@/api/models";
import { MessageStrip } from "./MessageStrip";

const { acknowledgeMessage } = vi.hoisted(() => ({ acknowledgeMessage: vi.fn() }));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: { position: string; dismissMessage: () => void; acknowledgeMessage: typeof acknowledgeMessage }) => unknown) =>
    selector({ position: "121.630", dismissMessage: vi.fn(), acknowledgeMessage }),
}));

beforeEach(() => acknowledgeMessage.mockClear());

describe("message text format", () => {
  it.each([
    [undefined, false, ["APRON ARR"], "1.04vw"],
    ["AUTO", false, ["APRON ARR"], "1.04vw"],
    ["AUTO", false, ["APRON ARR", "APRON DEP"], "0.73vw"],
    ["AUTO", true, [], "0.73vw"],
    ["SMALL", false, ["APRON ARR"], "0.73vw"],
    ["LARGE", true, [], "1.04vw"],
  ] as const)("uses %s formatting for broadcast=%s and recipients=%s", (format, is_broadcast, recipients, fontSize) => {
    const msg: MessageReceived = { id: 1, sender: "121.730", text: "Test message", is_broadcast, recipients: [...recipients], format };
    render(<MessageStrip msg={msg} />);
    expect(screen.getByText("Test message").parentElement?.style.fontSize).toBe(fontSize);
  });
});

describe("message SI", () => {
  const personalMessage: MessageReceived = {
    id: 7,
    sender: "119.905",
    text: "Test message",
    is_broadcast: false,
    recipients: ["APRON ARR"],
    receipt_positions: ["121.630"],
  };

  it("shows a split SI and lets the recipient acknowledge receipt", () => {
    render(<MessageStrip msg={personalMessage} />);

    const si = screen.getByRole("button", { name: "Acknowledge message receipt" });
    expect(si.style.background).toContain("linear-gradient");
    fireEvent.click(si);
    expect(acknowledgeMessage).toHaveBeenCalledWith(7);
  });

  it("shows orange after receipt is acknowledged", () => {
    render(<MessageStrip msg={{ ...personalMessage, acknowledged: true }} />);

    expect(screen.queryByRole("button", { name: "Acknowledge message receipt" })).toBeNull();
    expect(screen.getByText("Test message").parentElement?.parentElement?.firstElementChild?.getAttribute("style")).toContain("var(--color-si-unconcerned)");
  });

  it.each([
    { is_broadcast: true, recipients: [] },
    { is_broadcast: false, recipients: ["APRON ARR", "APRON DEP"] },
  ])("shows an orange SI without receipt for broadcast or multiple recipients: %s", overrides => {
    render(<MessageStrip msg={{ ...personalMessage, ...overrides }} />);

    expect(screen.queryByRole("button", { name: "Acknowledge message receipt" })).toBeNull();
    expect(screen.getByText("Test message").parentElement?.parentElement?.firstElementChild?.getAttribute("style")).toContain("var(--color-si-unconcerned)");
  });

  it("keeps system messages purple", () => {
    render(<MessageStrip msg={{ ...personalMessage, sender: "SYSTEM", receipt_positions: [] }} />);

    expect(screen.getByText("Test message").parentElement?.parentElement?.firstElementChild?.getAttribute("style")).toContain("var(--color-si-concerned)");
  });
});
