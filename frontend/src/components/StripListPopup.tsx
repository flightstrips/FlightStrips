import { useState } from "react";
import { Strip } from "@/components/strip/Strip.tsx";
import type { FrontendStrip } from "@/api/models.ts";
import {
  Dialog,
  DialogContent,
  DialogTitle,
} from "@/components/ui/dialog";
import { COLOR_DEP_STRIP_BG } from "@/components/strip/shared";

// StripListPopup color constants
const COLOR_POPUP_BG      = "#D5D5D5"; // main popup background
const COLOR_BADGE_BG      = "#C3C3C3"; // count badge background
const COLOR_BADGE_BORDER  = "#5A5A5A"; // badge border
const COLOR_BTN_BG        = "#B3B3B3"; // sort / dismiss button background
const COLOR_GUTTER_BG     = "#989898"; // scrollbar gutter
const COLOR_SORT_SELECTED = "#1BFF16"; // active sort mode highlight
const COLOR_SORT_BTN_BG   = "#D6D6D6"; // inactive sort mode button
const COLOR_DARK_BTN      = "#3F3F3F"; // OK / ESC dark buttons

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
}

export function StripListPopup<T extends FrontendStrip>({
  title,
  strips,
  sortModes,
  onRowClick,
  onDismiss,
  myPosition,
}: StripListPopupProps<T>) {
  const [currentSortKey, setCurrentSortKey] = useState(sortModes[0]?.key ?? "");
  const [sortDialogOpen, setSortDialogOpen] = useState(false);
  const [pendingSortKey, setPendingSortKey] = useState(currentSortKey);

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

  return (
    <>
      {/* Backdrop */}
      <div
        className="fixed inset-0 z-50 flex items-center justify-center"
        style={{ background: "rgba(0,0,0,0.45)" }}
        onMouseDown={onDismiss}
      >
        {/* Popup — fixed height so strip list scrolls */}
        <div
          className="flex flex-col"
          style={{
            width: 494,
            height: "calc(100vh - 80px)",
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
              <span style={{ fontFamily: "Rubik, sans-serif", fontWeight: 700, fontSize: 28, color: "black" }}>
                {title}
              </span>
            )}
          </div>

          {/* Header buttons — 55px, 10px left margin, 6px gaps between buttons */}
          <div className="flex shrink-0 items-stretch" style={{ height: 55, paddingLeft: 10 }}>
            {/* Count badge */}
            <div
              className="flex items-center justify-center"
              style={{
                width: 122,
                background: COLOR_BADGE_BG,
                borderLeft: `1px solid ${COLOR_BADGE_BORDER}`,
                borderRight: `1px solid ${COLOR_BADGE_BORDER}`,
              }}
            >
              <span style={{ fontFamily: "Rubik, sans-serif", fontWeight: 700, fontSize: 24, color: "black" }}>
                {strips.length}
              </span>
            </div>

            {/* 6px gap */}
            <div style={{ width: 6 }} />

            {/* SORT button — 136px */}
            <button
              className="flex items-center justify-center"
              style={{
                width: 136,
                background: COLOR_BTN_BG,
                border: "1px solid black",
              }}
              onClick={handleSortOpen}
            >
              <span style={{ fontFamily: "Rubik, sans-serif", fontWeight: 700, fontSize: 24, color: "black" }}>
                SORT
              </span>
            </button>

            {/* 6px gap */}
            <div style={{ width: 6 }} />

            {/* DISMISS button — fills remaining width */}
            <button
              className="flex flex-1 items-center justify-center"
              style={{
                background: COLOR_BTN_BG,
                border: "1px solid black",
              }}
              onClick={onDismiss}
            >
              <span style={{ fontFamily: "Rubik, sans-serif", fontWeight: 700, fontSize: 24, color: "black" }}>
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

            {/* Grey scrollbar gutter — 29px */}
            <div style={{ width: 29, background: COLOR_GUTTER_BG, flexShrink: 0 }} />

            {/* Strip list — scrollable */}
            <div className="flex-1 overflow-y-auto flex flex-col" style={{ gap: 2, background: COLOR_POPUP_BG }}>
              {sortedStrips.map(strip => (
                <div
                  key={strip.callsign}
                  className="cursor-pointer shrink-0"
                  style={{ background: COLOR_DEP_STRIP_BG, height: 45, overflow: "hidden" }}
                  onClick={() => onRowClick(strip)}
                >
                  <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={false} fullWidth={true} />
                </div>
              ))}
            </div>
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
                  fontFamily: "Rubik, sans-serif",
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
                  fontFamily: "Rubik, sans-serif",
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
                  fontFamily: "Rubik, sans-serif",
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
    </>
  );
}
