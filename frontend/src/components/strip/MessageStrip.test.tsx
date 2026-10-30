import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { MessageReceived } from "@/api/models";
import { MessageStrip } from "./MessageStrip";

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (state: { position: string; dismissMessage: () => void }) => unknown) =>
    selector({ position: "119.905", dismissMessage: vi.fn() }),
}));

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
