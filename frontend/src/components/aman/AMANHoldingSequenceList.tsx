import {useState, type KeyboardEvent, type PointerEvent} from "react";

import type {AMANCommandIntent, AMANFlight} from "@/api/aman";
import {cn} from "@/lib/utils";

import {formatGainLoss} from "./presentation";

const GAIN_LOSS_CONTEXT = {authoritative: true, connected: true, fresh: true};

const formatEAT = (value: string | null | undefined): string => {
  if (!value) return "--:--";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "--:--" : `${String(date.getUTCHours()).padStart(2, "0")}:${String(date.getUTCMinutes()).padStart(2, "0")}`;
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
export function AMANHoldingSequenceList({flights, eats = {}, disabled = false, onCommand}: {flights: AMANFlight[]; eats?: Record<string, string | null | undefined>; disabled?: boolean; onCommand: (intent: AMANCommandIntent) => void}) {
  const [selected, setSelected] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const [hover, setHover] = useState<string | null>(null);

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
    <section aria-label="Holding sequence" className="flex min-h-0 flex-1 flex-col-reverse justify-start overflow-y-auto border-[3px] border-[#dcdcdc] bg-[#3c3c3c] p-1 font-mono text-sm text-[#dcdcdc]">
      {flights.map((flight, index) => {
        const isSelected = selected === flight.callsign;
        const gainLoss = flight.gain_loss_seconds;
        return (
          <div
            aria-label={`${flight.callsign} sequence position ${index + 1}`}
            className={cn("flex touch-none select-none items-center justify-between gap-2 px-1 py-0.5", hover === flight.callsign && dragging && selected !== flight.callsign && "bg-[#505052]")}
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
              setDragging(false);
              setHover(null);
              if (from >= 0 && to >= 0) move(from, to);
            }}
            tabIndex={0}
          >
            <span className="flex items-center gap-2">
              <span className="w-5 text-right text-[#a9bdc5]">{index + 1}</span>
              <span className={cn("rounded-md border px-1.5 py-px font-bold", isSelected ? "border-white bg-[#5174b8]" : "border-transparent")}>{flight.callsign}</span>
            </span>
            <span className="flex items-center gap-3">
              <span className="w-9 text-right">{gainLoss === null ? "" : formatGainLoss(gainLoss, GAIN_LOSS_CONTEXT)}</span>
              <span className="w-12 text-right">{formatEAT(eats[flight.callsign] ?? flight.holding_fix_eta)}</span>
            </span>
          </div>
        );
      })}
    </section>
  );
}
