import {useLayoutEffect, useRef, useState, type KeyboardEvent, type PointerEvent} from "react";

import type {AMANCommandIntent, AMANFlight} from "@/api/aman";
import {cn} from "@/lib/utils";

import {formatAMANClearedAltitude, formatGainLoss} from "./presentation";

const GAIN_LOSS_CONTEXT = {authoritative: true, connected: true, fresh: true};

const formatEAT = (value: string | null | undefined): string | null => {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : `${String(date.getUTCHours()).padStart(2, "0")}:${String(date.getUTCMinutes()).padStart(2, "0")}`;
};

export function moveIntentFor(flights: AMANFlight[], from: number, to: number): AMANCommandIntent | null {
  const flight = flights[from];
  const target = flights[to];
  if (from === to || !flight?.runway_group_id || target?.runway_group_id !== flight.runway_group_id) return null;
  return to > from
    ? {type: "aman.move_flight", callsign: flight.callsign, runway_group_id: flight.runway_group_id, after_callsign: target.callsign}
    : {type: "aman.move_flight", callsign: flight.callsign, runway_group_id: flight.runway_group_id, before_callsign: target.callsign};
}

/**
 * Complete landing sequence for one holding, first flight at the bottom. Select a callsign
 * (click) then drag it, or use the arrow keys, to move it within the sequence.
 */
export function AMANHoldingSequenceList({flights, eats = {}, disabled = false, transitionAltitude = 0, onCommand}: {
  flights: AMANFlight[];
  eats?: Record<string, string | null | undefined>;
  disabled?: boolean;
  /** Airport transition altitude in feet, for formatting the EuroScope CFL like the strips. */
  transitionAltitude?: number;
  onCommand: (intent: AMANCommandIntent) => void;
}) {
  const [selected, setSelected] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const [hover, setHover] = useState<string | null>(null);
  // The first flight is drawn at the bottom. Keep the list scrolled to the
  // bottom on open and while the user is there, as a bottom-anchored list would.
  const listRef = useRef<HTMLElement>(null);
  const atBottom = useRef(true);
  useLayoutEffect(() => {
    const list = listRef.current;
    if (list !== null && atBottom.current) list.scrollTop = list.scrollHeight;
  }, [flights]);

  const move = (from: number, to: number) => {
    if (disabled || to < 0 || to >= flights.length) return;
    const intent = moveIntentFor(flights, from, to);
    if (intent === null) return;
    onCommand(intent);
  };

  const rowAt = (event: PointerEvent): string | null => document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-seq-callsign]")?.dataset.seqCallsign ?? null;

  const onKeyDown = (event: KeyboardEvent, index: number) => {
    // The list is drawn first-at-bottom, so "up" is later in the sequence.
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    event.preventDefault();
    move(index, index + (event.key === "ArrowUp" ? 1 : -1));
  };

  return (
    // One shared grid: each value column is only as wide as its widest value,
    // aligned down the list, and the callsign column takes what is left and
    // truncates. The scrollbar is hidden (wheel and drag still scroll) so the
    // narrow ACC panel never needs to scroll sideways.
    <section
      aria-label="Holding sequence"
      className="grid min-h-0 flex-1 grid-cols-[auto_minmax(0,1fr)_auto_auto_auto] gap-x-1 overflow-y-auto overflow-x-hidden border-[3px] border-[#dcdcdc] bg-[#3c3c3c] px-0.5 font-mono text-sm text-[#dcdcdc] [align-content:safe_end] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      onScroll={(event) => {
        const list = event.currentTarget;
        atBottom.current = list.scrollHeight - list.scrollTop - list.clientHeight < 2;
      }}
      ref={listRef}
    >
      {flights.map((flight, index) => {
        const isSelected = selected === flight.callsign;
        const gainLoss = flight.gain_loss_seconds;
        const time = formatEAT(eats[flight.callsign]) ?? formatEAT(flight.holding_fix_eta);
        const tmaProtected = flight.freeze_reason === "tma";
        const unavailableTimeLabel = `${tmaProtected ? "TMA entry protection; " : ""}holding-fix ETA / EAT unavailable`;
        return (
          <div
            aria-label={`${flight.callsign} sequence position ${index + 1}`}
            className={cn(
              // The blue box surrounds the whole row, on hover and when selected,
              // matching the timeline targets. A plain :hover is used because
              // Tailwind's hover: is skipped on touch-capable displays.
              // Mouse clicks draw no focus outline; keyboard focus still shows one.
              "col-span-full grid touch-none select-none grid-cols-subgrid items-center rounded-md border border-transparent py-px outline-none focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-white",
              isSelected ? "border-white bg-[#5174b8]" : "[&:hover]:border-white [&:hover]:bg-[#5174b8]",
              hover === flight.callsign && dragging && !isSelected && "bg-[#505052]",
            )}
            data-selected={isSelected || undefined}
            data-seq-callsign={flight.callsign}
            key={flight.callsign}
            onKeyDown={(event) => onKeyDown(event, index)}
            onPointerDown={(event) => {
              setSelected(flight.callsign);
              setHover(flight.callsign);
              setDragging(true);
              event.currentTarget.setPointerCapture?.(event.pointerId);
            }}
            onPointerMove={(event) => {
              if (dragging) setHover(rowAt(event));
            }}
            onPointerUp={(event) => {
              const target = rowAt(event);
              const from = flights.findIndex((item) => item.callsign === selected);
              const to = flights.findIndex((item) => item.callsign === target);
              // The selection only exists for the press or drag; release clears it.
              setDragging(false);
              setHover(null);
              setSelected(null);
              if (from >= 0 && to >= 0) move(from, to);
            }}
            onPointerCancel={() => {
              setDragging(false);
              setHover(null);
              setSelected(null);
            }}
            tabIndex={0}
          >
            <span className="text-right text-[#a9bdc5]">{index + 1}</span>
            <span className="min-w-0 truncate font-bold" title={flight.callsign}>{flight.callsign}</span>
            <span className="text-right text-[#a9bdc5]" data-field="cfl" title="Cleared flight level (EuroScope)">{formatAMANClearedAltitude(flight.cleared_altitude, transitionAltitude)}</span>
            <span className="text-right">{gainLoss === null ? "" : formatGainLoss(gainLoss, GAIN_LOSS_CONTEXT)}</span>
            <span
              className={cn("text-right", time === null && "text-[#a9bdc5]")}
              title={time === null ? unavailableTimeLabel : "Holding-fix ETA / EAT (UTC)"}
            >{time ?? <>
              <span aria-hidden="true">{tmaProtected ? "TMA" : "—"}</span>
              <span className="sr-only">{unavailableTimeLabel}</span>
            </>}</span>
          </div>
        );
      }).reverse()}
    </section>
  );
}
