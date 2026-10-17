import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketClient } from "@/api/websocket";
import { createWebSocketStore } from "@/store/store";
import { WebSocketStoreContext } from "@/store/store-context";
import TowerGroundLayout from "./TowerGroundLayout";

describe("arrival bay resizing", () => {
  beforeEach(() => {
    sessionStorage.clear();
    HTMLElement.prototype.scrollIntoView = vi.fn();
    vi.stubGlobal("ResizeObserver", class {
      observe = vi.fn();
      unobserve = vi.fn();
      disconnect = vi.fn();
    });
    vi.stubGlobal("PointerEvent", MouseEvent);
  });

  afterEach(() => vi.unstubAllGlobals());

  it.each(["TWTE", "TWRGND"] as const)("uses the arrival column for the TWY ARR handle in %s", (variant) => {
    const store = createWebSocketStore(new WebSocketClient("ws://localhost"));
    render(
      <WebSocketStoreContext.Provider value={store}>
        <TowerGroundLayout variant={variant} />
      </WebSocketStoreContext.Provider>,
    );

    const header = screen.getByText("TWY ARR").closest(".bay-col-header");
    expect(header).not.toBeNull();
    const handle = header!.querySelector<HTMLElement>('[title="Drag to resize, double-click to reset"]')!;
    expect(handle.parentElement).toBe(header);
    const column = header!.parentElement!;
    const flexBay = column.querySelector<HTMLElement>(":scope > .flex-1")!;
    vi.spyOn(column, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 1000));
    vi.spyOn(header!, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 40));
    vi.spyOn(flexBay, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 400));
    handle.setPointerCapture = vi.fn();

    for (const [endY, expected] of [[110, "21%"], [90, "19%"]] as const) {
      fireEvent.doubleClick(handle);
      fireEvent.pointerDown(handle, { button: 0, pointerId: 1, clientY: 100 });
      fireEvent.pointerMove(handle, { pointerId: 1, clientY: endY });
      fireEvent.pointerUp(handle, { pointerId: 1 });
      expect(column.style.getPropertyValue("--bay-h-rwyArr")).toBe(expected);
      expect(column.style.getPropertyValue("--bay-h-final")).toBe("35%");
      expect(JSON.parse(sessionStorage.getItem("tower-bay-heights-arr")!)).toEqual({
        final: 35,
        rwyArr: Number.parseFloat(expected),
      });
    }
  });
});
