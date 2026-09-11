import type {AMANHoldingEntry} from "@/api/aman";
import {cn} from "@/lib/utils";
import {type KeyboardEvent, useId, useRef} from "react";
import {
  HOLDING_GRAPH_MAX_FLIGHT_LEVEL,
  HOLDING_GRAPH_MIN_FLIGHT_LEVEL,
  HOLDING_GRAPH_WINDOW_MINUTES,
  layoutHoldingGraph,
} from "./holding-graph-positioning";

const timeTicks = Array.from({length: 13}, (_, index) => index * 5);
const altitudeTicks = Array.from({length: 22}, (_, index) => 300 - index * 10);

function clockLabel(value: Date): string {
  return value.toISOString().slice(11, 16);
}

function compactTime(label: string): string {
  return label.replace(":", "");
}

function entryState(position: ReturnType<typeof layoutHoldingGraph>[number]) {
  if (position.urgency === "missing") {
    return {label: "TIMING UNAVAILABLE", symbol: "?", tone: "border-slate-300 text-slate-200"};
  }
  if (position.time.edgeLabel === "OVERDUE") {
    return {label: "PAST EAT", symbol: "!", tone: "border-[#e65b5b] text-[#ff8b8b]"};
  }
  if (position.urgency === "soon") {
    return {label: "DUE ≤4 MIN", symbol: "≤4", tone: "border-[#29d229] text-[#62e562]"};
  }
  return {label: "SCHEDULED", symbol: "S", tone: "border-[#f0e129] text-[#f0e129]"};
}

function sourceState(entry: AMANHoldingEntry) {
  if (entry.source_status === "stale") {
    return {label: "STALE DATA", symbol: "△", tone: "border-amber-300 text-amber-200"};
  }
  if (entry.source_status === "disconnected") {
    return {label: "SOURCE DISCONNECTED", symbol: "×", tone: "border-red-300 text-red-200"};
  }
  return {label: "LIVE DATA", symbol: "", tone: ""};
}

function accessibleEntryLabel(position: ReturnType<typeof layoutHoldingGraph>[number]): string {
  const state = entryState(position);
  const source = sourceState(position.entry);
  return [
    position.entry.callsign,
    position.entry.holding,
    position.altitude.label === "—" ? "CFL unavailable" : position.altitude.label,
    position.time.label === "—" ? "EAT unavailable" : `EAT ${position.time.label}`,
    state.label,
    source.label,
  ].join(", ");
}

export function TMTHoldingGraph({entries, now = new Date()}: {entries: AMANHoldingEntry[]; now?: Date}) {
  const positions = layoutHoldingGraph(entries, now);
  const placed = positions.filter(({time, altitude}) => time.percent !== null && altitude.percent !== null);
  const unplaced = positions.filter(({time, altitude}) => time.percent === null || altitude.percent === null);
  const degraded = entries.filter(({source_status}) => source_status !== "fresh");
  const instructionsId = useId();
  const entryRefs = useRef(new Map<string, HTMLLIElement>());

  function setEntryRef(flightID: string, element: HTMLLIElement | null) {
    if (element) entryRefs.current.set(flightID, element);
    else entryRefs.current.delete(flightID);
  }

  function moveFocus(event: KeyboardEvent<HTMLLIElement>, flightID: string) {
    const current = positions.findIndex(({entry}) => entry.flight_id === flightID);
    let target = current;
    if (event.key === "ArrowDown" || event.key === "ArrowRight") target = (current + 1) % positions.length;
    else if (event.key === "ArrowUp" || event.key === "ArrowLeft") target = (current - 1 + positions.length) % positions.length;
    else if (event.key === "Home") target = 0;
    else if (event.key === "End") target = positions.length - 1;
    else return;
    event.preventDefault();
    entryRefs.current.get(positions[target].entry.flight_id)?.focus();
  }

  return (
    <section aria-label="TMT holding information" className="flex flex-col border border-[#777] bg-[#292929] text-[#dcdcdc]">
      <header className="flex items-center border-b border-[#777] px-3 py-2">
        <h2 className="font-display text-sm font-bold tracking-wide">TMT · HOLDING INFORMATION</h2>
        <span className="ml-auto font-mono text-[10px]">{entries.length} HOLDING</span>
      </header>
      <p className="sr-only" id={instructionsId}>Use Tab to enter the aircraft list. Use arrow keys to move between aircraft, or Home and End to jump to the first or last aircraft.</p>
      {degraded.length > 0 && (
        <p aria-live="polite" className="border-b border-amber-300 bg-amber-950 px-3 py-1.5 font-mono text-[10px] text-amber-100" role="status">
          △ DEGRADED DATA · {degraded.length} AIRCRAFT STALE OR DISCONNECTED
        </p>
      )}

      <div aria-describedby={instructionsId} className="relative mx-2 h-[500px] min-h-[420px] bg-[#3c3c3c]" data-testid="holding-graph">
        <div aria-hidden="true" className="absolute inset-y-3 left-[14%] border-l-[3px] border-[#dcdcdc]">
          {timeTicks.map((minutes) => (
            <span className="absolute left-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" key={minutes} style={{top: `${100 - minutes / HOLDING_GRAPH_WINDOW_MINUTES * 100}%`}}>
              {minutes % 10 === 0 && (
                <span className="absolute right-4 top-0 -translate-y-1/2 font-mono text-[9px] font-semibold">
                  {clockLabel(new Date(now.valueOf() + minutes * 60_000))}
                </span>
              )}
            </span>
          ))}
        </div>

        <div aria-hidden="true" className="absolute inset-y-3 right-[8%] border-r-[3px] border-[#dcdcdc]">
          {altitudeTicks.map((level) => (
            <span className="absolute right-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" key={level} style={{top: `${(HOLDING_GRAPH_MAX_FLIGHT_LEVEL - level) / (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) * 100}%`}}>
              <span className="absolute left-4 top-0 -translate-y-1/2 font-mono text-[10px] font-semibold">{level}</span>
            </span>
          ))}
        </div>

        <svg aria-hidden="true" className="absolute inset-3 h-[calc(100%_-_1.5rem)] w-[calc(100%_-_1.5rem)]" preserveAspectRatio="none" viewBox="0 0 100 100">
          {placed.map((position) => {
            const timeX = 14 + (position.timeTrack ?? 0) * 1.5;
            const targetX = 34 + (position.altitudeTrack ?? 0) * 1.5;
            return <line key={position.entry.flight_id} stroke="#dcdcdc" strokeWidth="0.45" x1={timeX} x2={targetX} y1={100 - position.time.percent!} y2={position.altitude.percent!} />;
          })}
        </svg>

        <ul className="absolute inset-3">
          {placed.map((position) => {
            const state = entryState(position);
            const source = sourceState(position.entry);
            return (
              <li
                aria-label={accessibleEntryLabel(position)}
                aria-keyshortcuts="ArrowDown ArrowRight ArrowUp ArrowLeft Home End"
                className="absolute left-[34%] grid w-[51%] -translate-y-1/2 grid-cols-[54px_1fr_52px] border border-[#cdcdcd] bg-[#3c3c3c] font-mono text-[10px] font-semibold hover:z-10 focus:z-20 focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-white"
                key={position.entry.flight_id}
                onKeyDown={(event) => moveFocus(event, position.entry.flight_id)}
                ref={(element) => setEntryRef(position.entry.flight_id, element)}
                style={{marginLeft: `${(position.altitudeTrack ?? 0) * 5}px`, top: `clamp(14px, ${position.altitude.percent}%, calc(100% - 14px))`}}
                tabIndex={0}
              >
                <span className={cn("flex items-center justify-center border-r border-[#cdcdcd] px-1", state.tone)} title={state.label}>
                  <b className="mr-1 text-[8px]" aria-hidden="true">{state.symbol}</b>{compactTime(position.time.label)}
                </span>
                <span className="truncate px-1.5 py-1 text-center">
                  {position.entry.callsign}<small className="ml-1 text-[8px] text-slate-300">{position.entry.holding}</small>
                  {source.symbol && <small className={cn("ml-1 border px-0.5 text-[8px]", source.tone)} title={source.label}>{source.symbol}</small>}
                </span>
                <span className="border-l border-[#cdcdcd] px-1 py-1 text-center">{position.altitude.label}</span>
              </li>
            );
          })}
        </ul>

        {placed.length === 0 && <p className="absolute inset-0 grid place-items-center text-xs text-slate-300">No positioned holding aircraft</p>}
        <div className="absolute inset-x-0 bottom-0 grid translate-y-full grid-cols-[22%_1fr_22%] border-[3px] border-[#dcdcdc] bg-[#3b3b3b] text-center text-xs font-bold">
          <span className="border-r-[3px] border-[#dcdcdc] py-1">EAT</span><span className="py-1">HOLDING</span><span className="border-l-[3px] border-[#dcdcdc] py-1">ALT</span>
        </div>
      </div>

      {unplaced.length > 0 && (
        <ul className="mt-8 border-t border-[#777] px-2 py-2 text-[10px]" aria-label="Holding aircraft with missing values">
          {unplaced.map((position) => {
            const {entry, time, altitude} = position;
            const source = sourceState(entry);
            return <li
              aria-label={accessibleEntryLabel(position)}
              aria-keyshortcuts="ArrowDown ArrowRight ArrowUp ArrowLeft Home End"
              className="grid grid-cols-[1fr_55px_55px_55px_auto] gap-1 font-mono focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-white"
              key={entry.flight_id}
              onKeyDown={(event) => moveFocus(event, entry.flight_id)}
              ref={(element) => setEntryRef(entry.flight_id, element)}
              tabIndex={0}
            ><b>{entry.callsign}</b><span>{entry.holding}</span><span>EAT {time.label}</span><span>CFL {altitude.label}</span><span className={source.tone}>{source.symbol} {source.symbol && source.label}</span></li>;
          })}
        </ul>
      )}
      <footer className="flex gap-3 border-t border-[#777] px-2 py-1.5 text-[9px] font-semibold">
        <span className="text-[#62e562]">≤4 DUE ≤4 MIN</span><span className="text-[#f0e129]">S SCHEDULED</span><span className="text-[#ff8b8b]">! PAST EAT</span><span className="text-amber-200">△ STALE</span><span className="text-red-200">× DISCONNECTED</span>
      </footer>
    </section>
  );
}
