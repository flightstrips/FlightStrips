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
  mockUseVacsSettings.mockReturnValue({
    vacsEnabled: true,
    setVacsEnabled: vi.fn(),
    vacsHost: "",
    setVacsHost: vi.fn(),
  });
});

describe("VACSBTN", () => {
  it("renders grayed when unavailable", () => {
    mockUseVacs.mockReturnValue({
      state: { status: "unavailable" },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dialClient: vi.fn(),
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
        ownClientId: "1",
      },
      actions: {
        acceptCall,
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dialClient: vi.fn(),
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
        ownClientId: "1",
      } as VacsState,
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dialClient: vi.fn(),
      },
    });
    render(<VACSBTN />);
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("shows ringing style when outgoing", () => {
    mockUseVacs.mockReturnValue({
      state: {
        status: "outgoing",
        callId: "call-out",
        peer: { id: "2", displayName: "EKCH_APP", frequency: "120.2", positionId: "EKCH_APP" },
        clients: [],
        ownPositionId: "TWR",
        ownClientId: "1",
      },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dialClient: vi.fn(),
      },
    });
    render(<VACSBTN />);
    const btn = screen.getByRole("button", { name: /vacs voice/i });
    expect(btn.className).toContain("FF8C00");
    expect(btn.className).toContain("animate-vacs-pulse");
  });

  it("ends call on click when connected and flashes red", async () => {
    const endCall = vi.fn().mockResolvedValue(undefined);
    mockUseVacs.mockReturnValue({
      state: {
        status: "connected",
        callId: "call-1",
        peer: { id: "2", displayName: "EKCH_APP", frequency: "120.2" },
      },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall,
        dialClient: vi.fn(),
      },
    });
    render(<VACSBTN />);
    const btn = screen.getByRole("button", { name: /vacs voice/i });
    expect(btn.className).toContain("1BFF16");
    fireEvent.click(btn);
    expect(btn.className).toContain("FF4444");
    expect(endCall).toHaveBeenCalledWith("call-1");
    expect(screen.queryByText("End call")).toBeNull();
  });

  it("is hidden when integration disabled", () => {
    mockUseVacsSettings.mockReturnValue({
      vacsEnabled: false,
      setVacsEnabled: vi.fn(),
      vacsHost: "",
      setVacsHost: vi.fn(),
    });
    mockUseVacs.mockReturnValue({
      state: { status: "unavailable" },
      actions: {
        acceptCall: vi.fn(),
        rejectCall: vi.fn(),
        endCall: vi.fn(),
        dialClient: vi.fn(),
      },
    });
    const { container } = render(<VACSBTN />);
    expect(container.querySelector("button")).toBeNull();
  });
});
