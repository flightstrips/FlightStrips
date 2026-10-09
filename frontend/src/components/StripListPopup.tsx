import { useEffect, useRef, useState, type RefObject } from "react";
import { Strip, type HalfStripVariant } from "@/components/strip/Strip.tsx";
import type { FrontendStrip } from "@/api/models.ts";
import { ValidationStatusDialog } from "@/components/strip/ValidationStatusDialog";
import { isValidationBlockingForPosition, CLS_CMD_BEVEL } from "@/components/strip/shared";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { useWebSocketStore } from "@/store/store-hooks";

// StripListPopup color constants
const COLOR_POPUP_BG      = "#D5D5D5"; // main popup background
const COLOR_BADGE_BG      = "#C3C3C3"; // count badge background
const COLOR_BTN_BG        = "#B3B3B3"; // sort / dismiss button background
const COLOR_SORT_SELECTED = "#1BFF16"; // active sort mode highlight
const COLOR_SORT_BTN_BG   = "#D6D6D6"; // inactive sort mode button
const COLOR_DARK_BTN      = "#3F3F3F"; // OK / ESC dark buttons

// Always-visible, draggable scrollbar with arrow buttons that drives the list element.
function ListScrollbar({ listRef, itemCount }: { listRef: RefObject<HTMLDivElement | null>; itemCount: number }) {
  const trackRef = useRef<HTMLDivElement>(null);
  const drag = useRef<{ startY: number; startTop: number } | null>(null);
  const [m, setM] = useState({ top: 0, client: 0, scroll: 0, track: 0 });

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    const update = () => setM({ top: el.scrollTop, client: el.clientHeight, scroll: el.scrollHeight, track: trackRef.current?.clientHeight ?? 0 });
    update();
    el.addEventListener("scroll", update);
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => { el.removeEventListener("scroll", update); ro.disconnect(); };
  }, [listRef, itemCount]);

  const scrollable = m.scroll > m.client;
  const thumbH = scrollable ? Math.max(24, (m.client / m.scroll) * m.track) : m.track;
  const maxTop = Math.max(0, m.track - thumbH);
  const thumbTop = scrollable ? (m.top / (m.scroll - m.client)) * maxTop : 0;

  const onThumbDown = (e: React.PointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    drag.current = { startY: e.clientY, startTop: listRef.current?.scrollTop ?? 0 };
  };
  const onThumbMove = (e: React.PointerEvent<HTMLDivElement>) => {
    const el = listRef.current;
    if (!drag.current || !el || maxTop === 0) return;
    el.scrollTop = drag.current.startTop + ((e.clientY - drag.current.startY) * (m.scroll - m.client)) / maxTop;
  };
  const onTrackDown = (e: React.MouseEvent<HTMLDivElement>) => {
    const el = listRef.current;
    if (!el || e.target !== e.currentTarget) return;
    const y = e.clientY - e.currentTarget.getBoundingClientRect().top;
    el.scrollBy({ top: (y < thumbTop ? -1 : 1) * el.clientHeight * 0.8, behavior: "smooth" });
  };

  const ARROW_H = 18;
  const step = (dir: 1 | -1) => listRef.current?.scrollBy({ top: dir * 45, behavior: "smooth" });
  const arrow = (dir: 1 | -1) => (
    <button
      type="button"
      aria-label={dir === 1 ? "Scroll down one step" : "Scroll up one step"}
      className="flex shrink-0 items-center justify-center outline-none active:brightness-90"
      style={{ height: ARROW_H, background: "#B5B5B5", border: "1px solid #7A7A7A" }}
      onClick={() => step(dir)}
    >
      <svg width="10" height="6" viewBox="0 0 10 6" fill="none" stroke="#3A3A3A" strokeWidth="1.4">
        <path d={dir === 1 ? "M1 1 L5 5 L9 1" : "M1 5 L5 1 L9 5"} />
      </svg>
    </button>
  );

  return (
    <div className="flex shrink-0 select-none flex-col" style={{ width: 18 }}>
      {arrow(-1)}
      <div ref={trackRef} className="relative flex-1" style={{ background: "#A3A3A3" }} onMouseDown={onTrackDown}>
        <div
          className="absolute left-0 right-0"
          style={{
            top: thumbTop,
            height: thumbH,
            background: "#B5B5B5",
            border: "1px solid #7A7A7A",
            boxSizing: "border-box",
            cursor: "default",
            touchAction: "none",
          }}
          onPointerDown={onThumbDown}
          onPointerMove={onThumbMove}
          onPointerUp={() => { drag.current = null; }}
        />
      </div>
      {arrow(1)}
    </div>
  );
}
export type SortMode<T> = {
  key: string;
  label: string;
  compareFn: (a: T, b: T) => number;
};

interface StripListPopupProps<T extends FrontendStrip> {
  title?: string;
  strips: T[];
  sortModes: SortMode<T>[];
  onRowClick: (strip: T) => void;
  onDismiss: () => void;
  myPosition: string;
  rowHalfStripVariant?: HalfStripVariant;
}

export function StripListPopup<T extends FrontendStrip>({
  title,
  strips,
  sortModes,
  onRowClick,
  onDismiss,
  myPosition,
  rowHalfStripVariant = "LOCKED-ARR",
}: StripListPopupProps<T>) {
  const selectStrip = useWebSocketStore((state) => state.selectStrip);
  const listRef = useRef<HTMLDivElement>(null);
  const scrollList = (direction: 1 | -1) => {
    const el = listRef.current;
    if (el) el.scrollBy({ top: direction * el.clientHeight * 0.8, behavior: "smooth" });
  };
  const [currentSortKey, setCurrentSortKey] = useState(sortModes[0]?.key ?? "");
  const [sortDialogOpen, setSortDialogOpen] = useState(false);
  const [pendingSortKey, setPendingSortKey] = useState(currentSortKey);
  const [validationDialogCallsign, setValidationDialogCallsign] = useState<string | null>(null);
  const [validationDialogOpen, setValidationDialogOpen] = useState(false);
  const validationDialogStatus = useWebSocketStore((state) =>
    validationDialogCallsign
      ? state.strips.find((strip) => strip.callsign === validationDialogCallsign)?.validation_status
      : undefined
  );

  const currentSort = sortModes.find(m => m.key === currentSortKey) ?? sortModes[0];
  const sortedStrips = currentSort ? [...strips].sort(currentSort.compareFn) : strips;

  const handleSortOpen = () => {
    setPendingSortKey(currentSortKey);
    setSortDialogOpen(true);
  };

  const handleSortOk = () => {
    setCurrentSortKey(pendingSortKey);
    setSortDialogOpen(false);
  };

  const handleRowClick = (strip: T) => {
    if (isValidationBlockingForPosition(strip.validation_status, myPosition)) {
      setValidationDialogCallsign(strip.callsign);
      setValidationDialogOpen(true);
      return;
    }

    selectStrip(strip.callsign);
    onRowClick(strip);
  };

  const handleDismiss = () => {
    setValidationDialogOpen(false);
    setValidationDialogCallsign(null);
    onDismiss();
  };

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-50 flex items-center justify-center"
        style={{ background: "rgba(0,0,0,0.45)" }}
        onMouseDown={handleDismiss}
      >
        {/* Popup — fixed height so strip list scrolls */}
        <div
          className="flex flex-col animate-dialog-zoom-in"
          style={{
            width: 494,
            height: "calc(100dvh - 80px)",
            background: COLOR_POPUP_BG,
          }}
          onMouseDown={e => e.stopPropagation()}
        >
          {/* Top title area — 71px, as per design */}
          <div
            className="flex items-center justify-center shrink-0"
            style={{ height: 71, background: COLOR_POPUP_BG }}
          >
            {title && (
              <span style={{ fontFamily: "var(--font-bay)", fontWeight: 700, fontSize: 28, color: "black" }}>
                {title}
              </span>
            )}
          </div>

          {/* Header buttons — 55px, 10px left margin, 6px gaps between buttons */}
          <div className="flex shrink-0 items-stretch" style={{ height: 55, paddingLeft: 10, paddingRight: 10 }}>
            {/* Count badge */}
            <div
              className="flex items-center justify-center border-2 border-t-[#393939] border-l-[#393939] border-b-[#CECECE] border-r-[#CECECE] shadow-[inset_2px_2px_2px_-1px_rgba(57,57,57,0.55),inset_-2px_-2px_2px_-1px_rgba(206,206,206,0.55)]"
              style={{ width: 122, background: COLOR_BADGE_BG }}
            >
              <span style={{ fontFamily: "var(--font-bay)", fontWeight: 700, fontSize: 24, color: "black" }}>
                {strips.length}
              </span>
            </div>

            {/* 6px gap */}
            <div style={{ width: 6 }} />

            {/* SORT button — 136px */}
            <button
              className={`flex items-center justify-center bg-[#B5B5B5] outline-none active:brightness-90 ${CLS_CMD_BEVEL}`}
              style={{ width: 136 }}
              onClick={handleSortOpen}
            >
              <span style={{ fontFamily: "var(--font-bay)", fontSize: 24, color: "black" }}>
                SORT
              </span>
            </button>

            {/* 6px gap */}
            <div style={{ width: 6 }} />

            {/* DISMISS button — fills remaining width */}
            <button
              className={`flex flex-1 items-center justify-center bg-[#B5B5B5] outline-none active:brightness-90 ${CLS_CMD_BEVEL}`}
              onClick={handleDismiss}
            >
              <span style={{ fontFamily: "var(--font-bay)", fontSize: 24, color: "black" }}>
                DISMISS
              </span>
            </button>
          </div>

          {/* 8px gap between header and strip list */}
          <div style={{ height: 8, background: COLOR_POPUP_BG, flexShrink: 0 }} />

          {/* Body — flex-1 so it fills remaining height */}
          <div className="flex flex-1 overflow-hidden">
            {/* 10px left margin */}
            <div style={{ width: 10, background: COLOR_POPUP_BG, flexShrink: 0 }} />

            <ListScrollbar listRef={listRef} itemCount={sortedStrips.length} />

            {/* Strip list — scrollable */}
            <div ref={listRef} className="flex-1 overflow-y-auto flex flex-col [&::-webkit-scrollbar]:hidden [scrollbar-width:none]" style={{ gap: 2, background: COLOR_POPUP_BG }}>
              {sortedStrips.map(strip => (
                <div
                  key={strip.callsign}
                  className="cursor-pointer shrink-0"
                  style={{ background: COLOR_POPUP_BG, height: "2.36dvh", overflow: "hidden", flexShrink: 0 }}
                  onClick={() => handleRowClick(strip)}
                >
                  <div style={{ height: "100%", pointerEvents: "none" }}>
                    <Strip
                      strip={strip}
                      status="HALF"
                      halfStripVariant={rowHalfStripVariant}
                      myPosition={myPosition}
                      selectable={false}
                      delegateCallsignClick={true}
                      fullWidth={true}
                      onStripMoved={handleDismiss}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Scroll buttons — 9% of the dialog height each, triangle is 25% of the button height */}
          <div className="flex shrink-0 items-stretch" style={{ padding: "8px 10px 10px", gap: 6 }}>
            {([["down", 1], ["up", -1]] as const).map(([dir, step]) => (
              <button
                key={dir}
                aria-label={dir === "down" ? "Scroll down" : "Scroll up"}
                className={`flex flex-1 items-center justify-center bg-[#B5B5B5] outline-none active:brightness-90 ${CLS_CMD_BEVEL}`}
                style={{ height: "calc((100dvh - 80px) * 0.09)" }}
                onClick={() => scrollList(step)}
              >
                <span
                  style={{
                    display: "block",
                    height: "25%",
                    aspectRatio: "1.2 / 1",
                    background: "black",
                    clipPath: dir === "down" ? "polygon(0 0, 100% 0, 50% 100%)" : "polygon(50% 0, 100% 100%, 0 100%)",
                  }}
                />
              </button>
            ))}
          </div>
        </div>
      </div>

      {/* Sort Dialog */}
      <Dialog open={sortDialogOpen} onOpenChange={open => { if (!open) setSortDialogOpen(false); }}>
        <DialogContent
          className="p-0 overflow-hidden"
          style={{
            width: 299,
            background: COLOR_BTN_BG,
            border: "1px solid black",
            borderRadius: 0,
          }}
        >
          <DialogTitle className="sr-only">Sort Options</DialogTitle>
          <div style={{ margin: 13, border: "1px solid black", padding: 13, display: "flex", flexDirection: "column", gap: 12 }}>
            {sortModes.map(mode => (
              <button
                key={mode.key}
                className="flex items-center justify-center"
                style={{
                  height: 55,
                  width: 210,
                  background: pendingSortKey === mode.key ? COLOR_SORT_SELECTED : COLOR_SORT_BTN_BG,
                  color: "black",
                  fontFamily: "var(--font-bay)",
                  fontWeight: 700,
                  fontSize: 28,
                  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
                  border: "none",
                  cursor: "pointer",
                }}
                onClick={() => setPendingSortKey(mode.key)}
              >
                {mode.label}
              </button>
            ))}
            <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
              <button
                style={{
                  width: 99,
                  height: 55,
                  background: COLOR_DARK_BTN,
                  color: "white",
                  fontFamily: "var(--font-bay)",
                  fontWeight: 700,
                  fontSize: 28,
                  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
                  border: "none",
                  cursor: "pointer",
                }}
                onClick={handleSortOk}
              >
                OK
              </button>
              <button
                style={{
                  width: 99,
                  height: 55,
                  background: COLOR_DARK_BTN,
                  color: "white",
                  fontFamily: "var(--font-bay)",
                  fontWeight: 700,
                  fontSize: 28,
                  boxShadow: "0 4px 4px rgba(0,0,0,0.25)",
                  border: "none",
                  cursor: "pointer",
                }}
                onClick={() => setSortDialogOpen(false)}
              >
                ESC
              </button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
      {validationDialogCallsign && validationDialogStatus && (
          <ValidationStatusDialog
            callsign={validationDialogCallsign}
            status={validationDialogStatus}
            open={validationDialogOpen}
            onOpenChange={setValidationDialogOpen}
          />
        )}
    </>
  );
}
