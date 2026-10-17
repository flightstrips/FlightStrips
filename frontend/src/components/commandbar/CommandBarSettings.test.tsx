import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import CommandBarSettings from "./CommandBarSettings";

const state = vi.hoisted(() => ({ masterCallsign: "EKCH_A_TWR" }));
vi.mock("@/store/store-hooks", () => ({
  useLocalIp: () => "",
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));

describe("FS master in Settings", () => {
  it("updates the callsign while open and handles loss of the master", () => {
    const { rerender } = render(<CommandBarSettings />);
    fireEvent.click(screen.getByRole("button", { name: "Settings" }));
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master: EKCH_A_TWR");
    state.masterCallsign = "EKDK_FMP";
    rerender(<CommandBarSettings />);
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master: EKDK_FMP");
    state.masterCallsign = "";
    rerender(<CommandBarSettings />);
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master: No active master");
  });
});
