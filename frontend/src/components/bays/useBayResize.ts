import { useState, type CSSProperties, type PointerEvent } from "react";

const MIN_BAY_PCT = 5;
const MIN_FLEX_BAY_PCT = 5;

type BayHeights = Record<string, number>;

function loadHeights(storageKey: string, defaults: BayHeights): BayHeights {
  try {
    const parsed = JSON.parse(sessionStorage.getItem(storageKey) ?? "null") as BayHeights | null;
    if (!parsed) return defaults;
    const merged = { ...defaults };
    for (const key of Object.keys(defaults)) {
      if (typeof parsed[key] === "number" && Number.isFinite(parsed[key])) merged[key] = parsed[key];
    }
    return merged;
  } catch {
    return defaults;
  }
}

/**
 * Resizable bays for one column. Every sized bay listed in `defaults` (in column order) has a height
 * (% of the column); the remaining bay at the bottom keeps `flex-1` and absorbs the difference.
 * `handleProps(key)` belongs on the header directly below bay `key`. Dragging it moves the boundary
 * between bay `key` (above) and the bay owning that header (below); every other bay keeps its size.
 * Use `h-[var(--bay-h-<key>)]` on each sized bay.
 */
export function useBayResize(storageKey: string, defaults: BayHeights) {
  const [stored, setHeights] = useState<BayHeights>(() => loadHeights(storageKey, defaults));
  // Bays added to `defaults` after the state was created (e.g. hot reload) fall back to their default.
  const heights = { ...defaults, ...stored };
  const keys = Object.keys(defaults);

  const columnStyle = Object.fromEntries(
    Object.entries(heights).map(([key, value]) => [`--bay-h-${key}`, `${value}%`]),
  ) as CSSProperties;

  const handleProps = (key: string) => {
    // Undefined when the bay below the header is the flex bay.
    const belowKey = keys[keys.indexOf(key) + 1] as string | undefined;

    return {
      onPointerDown: (event: PointerEvent<HTMLElement>) => {
        // Handle -> header -> column.
        const column = event.currentTarget.parentElement?.parentElement;
        if (!column || event.button !== 0) return;
        event.preventDefault();
        const target = event.currentTarget;
        target.setPointerCapture(event.pointerId);

        const columnHeight = column.getBoundingClientRect().height;
        const startY = event.clientY;
        const start = heights;
        let latest = heights;

        // The flex bay is the only direct child that fills the leftover space.
        const flexBay = column.querySelector<HTMLElement>(":scope > .flex-1");
        const flexPct = flexBay ? (flexBay.getBoundingClientRect().height / columnHeight) * 100 : 0;

        // Positive delta moves the boundary up: the bay above shrinks, the bay below grows.
        const maxDelta = start[key] - MIN_BAY_PCT;
        const minDelta = belowKey !== undefined
          ? -(start[belowKey] - MIN_BAY_PCT)
          : -(flexPct - MIN_FLEX_BAY_PCT);

        const onMove = (moveEvent: globalThis.PointerEvent) => {
          const rawDelta = ((startY - moveEvent.clientY) / columnHeight) * 100;
          const delta = Math.min(Math.max(rawDelta, Math.min(minDelta, 0)), Math.max(maxDelta, 0));
          latest = { ...start, [key]: start[key] - delta };
          if (belowKey !== undefined) latest[belowKey] = start[belowKey] + delta;
          setHeights(latest);
        };
        const onEnd = () => {
          target.removeEventListener("pointermove", onMove);
          target.removeEventListener("pointerup", onEnd);
          target.removeEventListener("pointercancel", onEnd);
          sessionStorage.setItem(storageKey, JSON.stringify(latest));
        };
        target.addEventListener("pointermove", onMove);
        target.addEventListener("pointerup", onEnd);
        target.addEventListener("pointercancel", onEnd);
      },
      onDoubleClick: () => {
        const next = { ...heights, [key]: defaults[key] };
        if (belowKey !== undefined) next[belowKey] = defaults[belowKey];
        setHeights(next);
        sessionStorage.setItem(storageKey, JSON.stringify(next));
      },
    };
  };

  return { columnStyle, handleProps };
}