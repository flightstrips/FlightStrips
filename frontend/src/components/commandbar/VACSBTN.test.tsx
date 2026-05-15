import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import VACSBTN from "./VACSBTN";
import type { VacsState } from "@/vacs/types";

vi.mock("@/hooks/useVacs", () => ({
  useVacs: vi.fn(),
}));

vi.mock("@/hooks/useVacsSettings", () => ({
  useVacsSettings: vi.fn(),
}));

vi.mock("./VacsDialModal", () => ({
  default: () => null,
}));

import { useVacs } from "@/hooks/useVacs";
import { useVacsSettings } from "@/hooks/useVacsSettings";

const mockUseVacs = vi.mocked(useVacs);
const mockUseVacsSettings = vi.mocked(useVacsSettings);

beforeEach(() => {
  mockUseVacsSettings.mockReturnValue({ vacsEnabled: true, setVacsEnabled: vi.fn() });
});

describe("VACSBTN", () => {
  it("renders grayed when unavailable", () => {
    mockUseVacs.mockReturnValue({
      state: { status: "unavailable" },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dial: vi.fn(),
        dialByPosition: vi.fn(),
      },
    });
    render(<VACSBTN />);
    const btn = screen.getByRole("button", { name: /vacs voice/i });
    expect(btn).toBeDisabled();
    expect(btn.className).toContain("opacity-50");
  });

  it("accepts oldest incoming call on click", async () => {
    const acceptCall = vi.fn().mockResolvedValue(undefined);
    mockUseVacs.mockReturnValue({
      state: {
        status: "incoming",
        calls: [
          {
            callId: "oldest",
            source: { clientId: "2", positionId: "APP" },
            target: {},
            prio: false,
          },
        ],
        clients: [],
        ownPositionId: "TWR",
      },
      actions: {
        acceptCall,
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dial: vi.fn(),
        dialByPosition: vi.fn(),
      },
    });
    render(<VACSBTN />);
    fireEvent.click(screen.getByRole("button", { name: /vacs voice/i }));
    expect(acceptCall).toHaveBeenCalledWith("oldest");
  });

  it("shows badge when multiple incoming", () => {
    mockUseVacs.mockReturnValue({
      state: {
        status: "incoming",
        calls: [
          { callId: "a", source: { clientId: "1" }, target: {}, prio: false },
          { callId: "b", source: { clientId: "2" }, target: {}, prio: false },
        ],
        clients: [],
        ownPositionId: "TWR",
      } as VacsState,
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dial: vi.fn(),
        dialByPosition: vi.fn(),
      },
    });
    render(<VACSBTN />);
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("is hidden when integration disabled", () => {
    mockUseVacsSettings.mockReturnValue({ vacsEnabled: false, setVacsEnabled: vi.fn() });
    mockUseVacs.mockReturnValue({
      state: { status: "unavailable" },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dial: vi.fn(),
        dialByPosition: vi.fn(),
      },
    });
    const { container } = render(<VACSBTN />);
    expect(container.querySelector("button")).toBeNull();
  });
});
