import type {AMANHoldingEntry} from "@/api/aman";
import {cn} from "@/lib/utils";
import {type KeyboardEvent, useEffect, useId, useRef, useState} from "react";
import {
  HOLDING_GRAPH_MAX_FLIGHT_LEVEL,
  HOLDING_GRAPH_MIN_FLIGHT_LEVEL,
  HOLDING_GRAPH_WINDOW_MINUTES,
  layoutHoldingGraph,
  layoutHoldingLabels,
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

export function TMTHoldingGraph({compact = false, entries, holding, now = new Date(), onOpenFlightActions}: {compact?: boolean; entries: AMANHoldingEntry[]; holding?: string; now?: Date; onOpenFlightActions?: (callsign: string) => void}) {
  const positions = layoutHoldingGraph(entries, now);
  const placed = positions.filter(({altitude}) => altitude.percent !== null);
  const unplaced = positions.filter(({altitude}) => altitude.percent === null);
  const instructionsId = useId();
  const entryRefs = useRef(new Map<string, HTMLLIElement>());
  const graphRef = useRef<HTMLDivElement>(null);
  const [graphHeight, setGraphHeight] = useState(240);
  const [graphWidth, setGraphWidth] = useState(210);
  const rowHeight = compact ? 20 : 28;
  const availableHeight = Math.max(0, graphHeight - Math.max(12, rowHeight / 2 + 2) * 2);
  const altitudeHeight = Math.min(availableHeight,
    (rowHeight + 1) * (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) / 10);
  const labels = layoutHoldingLabels(placed, altitudeHeight, rowHeight);
  const axisPadding = Math.max(12, labels.edgePadding);
  const plotHeight = Math.max(availableHeight, labels.height);
  const altitudeOffset = (plotHeight - labels.height) / 2;
  const contentWidth = Math.max(graphWidth, (compact ? 104 : 230) / 0.51);
  const labelWidth = contentWidth * 0.51;
  const timeAxisX = contentWidth * 0.14;
  const labelLeft = contentWidth * 0.34;

  useEffect(() => {
    const graph = graphRef.current;
    if (!graph || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => {
      setGraphHeight(entry.contentRect.height);
      setGraphWidth(entry.contentRect.width);
    });
    observer.observe(graph);
    return () => observer.disconnect();
  }, []);

  function setEntryRef(callsign: string, element: HTMLLIElement | null) {
    if (element) entryRefs.current.set(callsign, element);
    else entryRefs.current.delete(callsign);
  }

  function moveFocus(event: KeyboardEvent<HTMLLIElement>, callsign: string) {
    if (onOpenFlightActions && (event.key === "Enter" || event.key === " ")) {
      event.preventDefault();
      onOpenFlightActions(callsign);
      return;
    }
    const current = positions.findIndex(({entry}) => entry.callsign === callsign);
    let target: number;
    if (event.key === "ArrowDown" || event.key === "ArrowRight") target = (current + 1) % positions.length;
    else if (event.key === "ArrowUp" || event.key === "ArrowLeft") target = (current - 1 + positions.length) % positions.length;
    else if (event.key === "Home") target = 0;
    else if (event.key === "End") target = positions.length - 1;
    else return;
    event.preventDefault();
    entryRefs.current.get(positions[target].entry.callsign)?.focus();
  }

  return (
    <section aria-label={holding ? `TMT holding information for ${holding}` : "TMT holding information"} className={cn("relative flex h-full min-w-0 flex-col border border-[#777] bg-[#292929] text-[#dcdcdc]", compact && "overflow-hidden")}>
      {!compact && <header className="flex items-center border-b border-[#777] px-3 py-2">
        <h2 className="font-display text-sm font-bold tracking-wide">TMT · HOLDING INFORMATION</h2>
        <span className="ml-auto font-mono text-[10px]">{entries.length} HOLDING</span>
      </header>}
      <p className="sr-only" id={instructionsId}>Use Tab to enter the aircraft list. Use arrow keys to move between aircraft, or Home and End to jump to the first or last aircraft.{onOpenFlightActions && " Press Enter or Space to open the flight menu."}</p>

      <div aria-describedby={instructionsId} className={cn("relative min-h-0 flex-1 overflow-auto bg-[#3c3c3c]", !compact && "mx-1")} data-testid="holding-graph" ref={graphRef}>
        <div className="relative" style={{height: plotHeight + axisPadding * 2, width: contentWidth}}>
        <div aria-hidden="true" className="absolute border-l-[3px] border-[#dcdcdc]" data-testid="holding-time-axis" style={{left: timeAxisX, top: axisPadding, bottom: axisPadding}}>
          {timeTicks.map((minutes) => (
            <span className="absolute left-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" key={minutes} style={{top: `${100 - minutes / HOLDING_GRAPH_WINDOW_MINUTES * 100}%`}}>
              {minutes % 10 === 0 && (
                <span className="absolute right-4 top-0 -translate-y-1/2 font-mono text-[9px] font-semibold">
                  {compact ? clockLabel(new Date(now.valueOf() + minutes * 60_000)).slice(3) : clockLabel(new Date(now.valueOf() + minutes * 60_000))}
                </span>
              )}
            </span>
          ))}
        </div>

        <div aria-hidden="true" className="absolute right-[8%] border-r-[3px] border-[#dcdcdc]" data-testid="holding-altitude-axis" style={{top: axisPadding + altitudeOffset, height: labels.height}}>
          {altitudeTicks.map((level) => (
            <span className="absolute right-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" data-flight-level={level} key={level} style={{top: labels.altitudeY((HOLDING_GRAPH_MAX_FLIGHT_LEVEL - level) / (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) * 100)}}>
              <span className="absolute right-4 top-0 -translate-y-1/2 font-mono text-[10px] font-semibold">{level}</span>
            </span>
          ))}
        </div>

        <svg aria-hidden="true" className="absolute left-0" style={{top: axisPadding, height: plotHeight, width: contentWidth}} preserveAspectRatio="none" viewBox={`0 0 ${contentWidth} ${plotHeight}`}>
          {placed.map((position, index) => {
            if (position.time.percent === null) return null;
            return <line data-callsign={position.entry.callsign} key={position.entry.callsign} stroke="#dcdcdc" strokeWidth="2" vectorEffect="non-scaling-stroke" x1={timeAxisX + 1.5} x2={labelLeft} y1={(100 - position.time.percent!) / 100 * plotHeight} y2={altitudeOffset + labels.labelY[index]} />;
          })}
        </svg>

        <ul className="absolute inset-x-0" style={{top: axisPadding + altitudeOffset, height: labels.height}}>
          {placed.map((position, index) => {
            const state = entryState(position);
            const source = sourceState(position.entry);
            return (
              <li
                aria-label={accessibleEntryLabel(position)}
                aria-keyshortcuts={`ArrowDown ArrowRight ArrowUp ArrowLeft Home End${onOpenFlightActions ? " Enter Space" : ""}`}
                title={onOpenFlightActions ? `Open flight menu for ${position.entry.callsign}` : undefined}
                data-altitude-y={labels.altitudeY(position.altitude.percent!)}
                className={cn("absolute left-[34%] grid w-[51%] -translate-y-1/2 border border-[#cdcdcd] bg-[#3c3c3c] font-mono font-semibold hover:z-10 focus:z-20 focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-white", compact ? "grid-cols-[2.25rem_minmax(0,1fr)] text-[10px]" : "grid-cols-[54px_1fr_52px] text-[10px]")}
                key={position.entry.callsign}
                onClick={() => onOpenFlightActions?.(position.entry.callsign)}
                onKeyDown={(event) => moveFocus(event, position.entry.callsign)}
                ref={(element) => setEntryRef(position.entry.callsign, element)}
                role={onOpenFlightActions ? "button" : undefined}
                style={{height: rowHeight, top: labels.labelY[index], left: labelLeft, width: labelWidth - 2}}
                tabIndex={0}
              >
                <span className={cn("flex items-center justify-center border-r border-[#cdcdcd] px-1", state.tone)} title={position.urgency === "missing" ? "No calculated EAT. See this aircraft in Current warnings for the reason." : state.label}>
                  {!compact && <b className="mr-1 text-[8px]" aria-hidden="true">{state.symbol}</b>}{compactTime(position.time.label)}
                </span>
                <span className={cn("flex items-center justify-center whitespace-nowrap text-center", compact ? "px-0.5" : "px-1.5")}>
                  {position.entry.callsign}{!compact && <small className="ml-1 text-[8px] text-slate-300">{position.entry.holding}</small>}
                  {source.symbol && <small className={cn("ml-1 border px-0.5 text-[8px]", source.tone)} title={source.label}>{source.symbol}</small>}
                </span>
                {!compact && <span className="border-l border-[#cdcdcd] px-1 py-1 text-center">{position.altitude.label}</span>}
              </li>
            );
          })}
        </ul>

        {placed.length === 0 && <p className={cn("absolute inset-0 grid place-items-center text-slate-300", compact ? "text-[9px]" : "text-xs")}>{compact ? "NONE" : "No positioned holding aircraft"}</p>}
        </div>
      </div>
        <div className={cn("grid shrink-0 grid-cols-[22%_1fr_22%] border-[3px] border-[#dcdcdc] bg-[#3b3b3b] text-center font-bold", compact ? "text-[9px]" : "mx-1 text-xs")}>
          <span className="border-r-[3px] border-[#dcdcdc] py-1">EAT</span><span className="truncate py-1">{holding ?? "HOLDING"}</span><span className="border-l-[3px] border-[#dcdcdc] py-1">ALT</span>
        </div>

      {unplaced.length > 0 && (
        <ul className="max-h-[35%] shrink-0 overflow-auto border-t border-[#777] bg-[#292929] px-2 py-2 text-[10px]" aria-label="Holding aircraft with missing values">
          {unplaced.map((position) => {
            const {entry, time, altitude} = position;
            const source = sourceState(entry);
            return <li
              aria-label={accessibleEntryLabel(position)}
              aria-keyshortcuts={`ArrowDown ArrowRight ArrowUp ArrowLeft Home End${onOpenFlightActions ? " Enter Space" : ""}`}
              title={onOpenFlightActions ? `Open flight menu for ${entry.callsign}` : undefined}
              className="grid grid-cols-[1fr_55px_55px_55px_auto] gap-1 font-mono focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-white"
              key={entry.callsign}
              onClick={() => onOpenFlightActions?.(entry.callsign)}
              onKeyDown={(event) => moveFocus(event, entry.callsign)}
              ref={(element) => setEntryRef(entry.callsign, element)}
              role={onOpenFlightActions ? "button" : undefined}
              tabIndex={0}
            ><b>{entry.callsign}</b><span>{entry.holding}</span><span>EAT {time.label}</span><span>CFL {altitude.label}</span><span className={source.tone}>{source.symbol} {source.symbol && source.label}</span></li>;
          })}
        </ul>
      )}
      {!compact && <footer className="flex gap-3 border-t border-[#777] px-2 py-1.5 text-[9px] font-semibold">
        <span className="text-[#62e562]">≤4 DUE ≤4 MIN</span><span className="text-[#f0e129]">S SCHEDULED</span><span className="text-[#ff8b8b]">! PAST EAT</span><span className="text-amber-200">△ STALE</span><span className="text-red-200">× DISCONNECTED</span>
      </footer>}
    </section>
  );
}
