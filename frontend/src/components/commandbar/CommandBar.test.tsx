import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterAll, afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import CommandBar from "./CommandBar";
import type { RunwayConfiguration } from "@/api/models";

const runway = vi.hoisted(() => ({ departure: [], arrival: [] } as RunwayConfiguration));

const state = vi.hoisted(() => ({
  displayedLayout: "AAAD",
  setDisplayedLayout: vi.fn(),
  resetLayout: vi.fn(),
  readOnly: false,
  positionAvailable: true,
  masterCallsign: "EKCH_A_TWR",
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
  useMetar: () => null,
  useAtisCode: () => "",
  useRunwaySetup: () => runway,
  useSelectedCallsign: () => null,
  useSelectStrip: () => vi.fn(),
  useStrips: () => [],
  useMarkArmed: () => false,
  useSetMarkArmed: () => vi.fn(),
  useStrip: () => undefined,
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
vi.mock("@/components/Time", () => ({ default: () => null }));
vi.mock("@/components/MetarHelper", () => ({ default: () => null }));
vi.mock("./MRKBTN", () => ({ default: () => null }));
vi.mock("./VACSBTN", () => ({ default: () => null }));
vi.mock("./CommandBarSettings", () => ({ default: () => null }));
vi.mock("./TRFBRN", () => ({ default: () => null }));
vi.mock("./REQBTN", () => ({ default: () => null }));
vi.mock("./ATIS", () => ({ default: () => null }));
vi.mock("./CDMSIM", () => ({ default: () => null }));
vi.mock("@/lib/app-version", () => ({ FRONTEND_VERSION: "test" }));

function openScopes() {
  fireEvent.click(screen.getByRole("button", { name: "AA + AD" }));
}

const fullscreenDescriptors = {
  enabled: Object.getOwnPropertyDescriptor(document, "fullscreenEnabled"),
  element: Object.getOwnPropertyDescriptor(document, "fullscreenElement"),
  exit: Object.getOwnPropertyDescriptor(document, "exitFullscreen"),
  request: Object.getOwnPropertyDescriptor(document.documentElement, "requestFullscreen"),
};

beforeEach(() => {
  vi.clearAllMocks();
  runway.departure = [];
  runway.arrival = [];
  runway.departure_mismatch = false;
  runway.arrival_mismatch = false;
  state.masterCallsign = "EKCH_A_TWR";
  Object.defineProperty(document, "fullscreenEnabled", { configurable: true, get: () => true });
  Object.defineProperty(document, "fullscreenElement", { configurable: true, get: () => null });
  Object.defineProperty(document, "exitFullscreen", { configurable: true, value: vi.fn().mockResolvedValue(undefined) });
  Object.defineProperty(document.documentElement, "requestFullscreen", { configurable: true, value: vi.fn().mockResolvedValue(undefined) });
});

afterEach(() => vi.restoreAllMocks());
afterAll(() => {
  for (const [target, key, descriptor] of [
    [document, "fullscreenEnabled", fullscreenDescriptors.enabled],
    [document, "fullscreenElement", fullscreenDescriptors.element],
    [document, "exitFullscreen", fullscreenDescriptors.exit],
    [document.documentElement, "requestFullscreen", fullscreenDescriptors.request],
  ] as const) {
    if (descriptor) Object.defineProperty(target, key, descriptor);
    else Reflect.deleteProperty(target, key);
  }
});

describe("scope selector controls", () => {
  it("identifies the master and expected runways from either mismatch indicator", () => {
    runway.departure = ["22R"];
    runway.arrival = ["22L"];
    runway.departure_mismatch = true;
    runway.arrival_mismatch = true;
    const { rerender } = render(<CommandBar />);
    fireEvent.click(screen.getByRole("button", { name: /Departure runway mismatch/ }));
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master EKCH_A_TWR");
    expect(screen.getByRole("dialog")).toHaveTextContent("Expected DEP 22R / ARR 22L");
    state.masterCallsign = "EKDK_FMP";
    rerender(<CommandBar />);
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master EKDK_FMP");
    fireEvent.click(screen.getByRole("button", { name: "ESC" }));
    fireEvent.click(screen.getByRole("button", { name: /Arrival runway mismatch/ }));
    expect(screen.getByRole("dialog")).toHaveTextContent("FS Master EKDK_FMP");
  });
  it("enters fullscreen and synchronizes external exits", async () => {
    const request = vi.spyOn(document.documentElement, "requestFullscreen");
    render(<CommandBar />);
    openScopes();
    fireEvent.click(screen.getByRole("button", { name: "Enter fullscreen" }));
    await waitFor(() => expect(request).toHaveBeenCalledOnce());

    vi.spyOn(document, "fullscreenElement", "get").mockReturnValue(document.documentElement);
    act(() => document.dispatchEvent(new Event("fullscreenchange")));
    expect(screen.getByRole("button", { name: "Exit fullscreen" })).toHaveAttribute("aria-pressed", "true");

    vi.spyOn(document, "fullscreenElement", "get").mockReturnValue(null);
    act(() => document.dispatchEvent(new Event("fullscreenchange")));
    expect(screen.getByRole("button", { name: "Enter fullscreen" })).toHaveAttribute("aria-pressed", "false");
  });

  it("exits fullscreen on a second click", async () => {
    vi.spyOn(document, "fullscreenElement", "get").mockReturnValue(document.documentElement);
    const exit = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(document, "exitFullscreen", { configurable: true, value: exit });
    render(<CommandBar />);
    openScopes();
    fireEvent.click(screen.getByRole("button", { name: "Exit fullscreen" }));
    await waitFor(() => expect(exit).toHaveBeenCalledOnce());
  });

  it("disables unsupported fullscreen", () => {
    vi.spyOn(document, "fullscreenEnabled", "get").mockReturnValue(false);
    render(<CommandBar />);
    openScopes();
    expect(screen.getByRole("button", { name: "Enter fullscreen" })).toBeDisabled();
  });

  it("reports rejected fullscreen requests", async () => {
    Object.defineProperty(document.documentElement, "requestFullscreen", {
      configurable: true,
      value: vi.fn().mockRejectedValue(new Error("Permission denied")),
    });
    render(<CommandBar />);
    openScopes();
    fireEvent.click(screen.getByRole("button", { name: "Enter fullscreen" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Could not change fullscreen mode", {
      description: "Permission denied",
    }));
  });

  it("keeps tower placeholders disabled while apron scopes remain selectable", () => {
    render(<CommandBar />);
    openScopes();
    const expand = screen.getByRole("button", { name: "Show more scopes" });
    expect(expand).toHaveClass("w-1/2");
    expect(screen.getByRole("button", { name: "Enter fullscreen" })).toHaveClass("w-1/2");
    expect(expand.parentElement?.parentElement?.style.width).toBe("6.6vw");
    fireEvent.click(expand);
    for (const label of ["TOWER EAST", "TOWER WEST"]) {
      const button = screen.getByRole("button", { name: label });
      expect(button).toBeDisabled();
      expect(button.style.background).toBe("rgb(214, 214, 214)");
      expect(button.style.color).toBe("rgb(153, 153, 153)");
      fireEvent.click(button);
    }
    expect(state.setDisplayedLayout).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "APRON ARR" }));
    expect(state.setDisplayedLayout).toHaveBeenCalledWith("AA");
  });

  it("opens EST in a fresh window without changing the current scope", () => {
    const newWindow = { opener: window };
    const open = vi.spyOn(window, "open").mockReturnValue(newWindow as Window);
    render(<CommandBar />);
    openScopes();
    fireEvent.click(screen.getByRole("button", { name: "Show more scopes" }));
    fireEvent.click(screen.getByRole("button", { name: /EST.*NEW WINDOW/ }));
    expect(open).toHaveBeenCalledWith(`${window.location.origin}/app?layout=EST`, "_blank", "popup");
    expect(newWindow.opener).toBeNull();
    expect(state.setDisplayedLayout).not.toHaveBeenCalled();
  });

  it("reports blocked EST popups", () => {
    vi.spyOn(window, "open").mockReturnValue(null);
    render(<CommandBar />);
    openScopes();
    fireEvent.click(screen.getByRole("button", { name: "Show more scopes" }));
    fireEvent.click(screen.getByRole("button", { name: /EST.*NEW WINDOW/ }));
    expect(toast.error).toHaveBeenCalledWith("Could not open EST", {
      description: "Allow popups for FlightStrips and try again.",
    });
  });
});
