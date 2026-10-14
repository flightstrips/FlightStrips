import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import AppRouter from "./AppRouter";

const state = vi.hoisted(() => ({
  displayedLayout: "AAAD",
  setDisplayedLayout: vi.fn(),
  readOnly: false,
  positionAvailable: true,
  layoutResetNonce: 0,
}));

vi.mock("@/store/store-hooks", () => ({
  useWebSocketStore: (selector: (value: typeof state) => unknown) => selector(state),
}));
vi.mock("@/routes/ekch/CLX", () => ({ default: () => null }));
vi.mock("@/routes/ekch/AAAD", () => ({ default: () => null }));
vi.mock("@/routes/ekch/AA", () => ({ default: () => null }));
vi.mock("@/routes/ekch/AD", () => ({ default: () => null }));
vi.mock("@/routes/ekch/EST", () => ({ default: () => null }));
vi.mock("@/routes/ekch/SEQPLN", () => ({ default: () => null }));
vi.mock("@/routes/ekch/GEGW", () => ({ default: () => null }));
vi.mock("@/routes/ekch/TWTE", () => ({ default: () => null }));
vi.mock("@/routes/ekch/TWRGND", () => ({ default: () => null }));
vi.mock("@/routes/ekch/AMAN", () => ({ default: () => null }));
vi.mock("@/components/ChooseLayoutScreen", () => ({ default: () => null }));
vi.mock("@/components/ObserverInvalidFrequencyScreen", () => ({ default: () => null }));

const originalURL = window.location.href;
beforeEach(() => vi.clearAllMocks());
afterEach(() => window.history.replaceState(null, "", originalURL));

describe("companion window startup", () => {
  it("selects EST when requested in the URL", () => {
    window.history.replaceState(null, "", "/app?layout=EST");
    const view = render(<AppRouter />);
    expect(state.setDisplayedLayout).toHaveBeenCalledExactlyOnceWith("EST");
    view.rerender(<AppRouter />);
    expect(state.setDisplayedLayout).toHaveBeenCalledOnce();
  });

  it("preserves normal scope selection without the EST parameter", () => {
    window.history.replaceState(null, "", "/app");
    render(<AppRouter />);
    expect(state.setDisplayedLayout).not.toHaveBeenCalled();
  });

  it("does not select arbitrary layouts from the URL", () => {
    window.history.replaceState(null, "", "/app?layout=UNKNOWN");
    render(<AppRouter />);
    expect(state.setDisplayedLayout).not.toHaveBeenCalled();
  });
});
