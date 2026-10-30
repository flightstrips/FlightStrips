import type {AMANHoldingEntry} from "@/api/aman";
import {cn} from "@/lib/utils";
import {type KeyboardEvent, useEffect, useId, useRef, useState} from "react";
import "./TMTHoldingGraph.css";
import {
  HOLDING_GRAPH_MAX_FLIGHT_LEVEL,
  HOLDING_GRAPH_MIN_FLIGHT_LEVEL,
  HOLDING_GRAPH_WINDOW_MINUTES,
  layoutHoldingGraph,
  layoutHoldingLabels,
} from "./holding-graph-positioning";

const altitudeTicks = Array.from({length: 20}, (_, index) => 280 - index * 10);
const LABEL_TRIANGLE_WIDTH = 8;

function clockLabel(value: Date): string {
  return value.toISOString().slice(11, 16);
}

function compactTime(label: string): string {
  return label.replace(":", "");
}

function holdingTimeTicks(now: Date) {
  const firstMinute = Math.ceil(now.valueOf() / 60_000);
  return Array.from({length: HOLDING_GRAPH_WINDOW_MINUTES + 1}, (_, index) => {
    const date = new Date((firstMinute + index) * 60_000);
    return {date, minutes: (date.valueOf() - now.valueOf()) / 60_000};
  }).filter(({date, minutes}) => minutes <= HOLDING_GRAPH_WINDOW_MINUTES && date.getUTCMinutes() % 2 === 1);
}

function entryState(position: ReturnType<typeof layoutHoldingGraph>[number]) {
  if (position.urgency === "missing") {
    return {label: "TIMING UNAVAILABLE", symbol: "?", tone: "text-slate-200"};
  }
  if (position.time.edgeLabel === "OVERDUE") {
    return {label: "PAST EAT", symbol: "!", tone: "text-[#ff8b8b]"};
  }
  if (position.urgency === "soon") {
    return {label: "DUE ≤4 MIN", symbol: "≤4", tone: "text-[#29d229]"};
  }
  return {label: "SCHEDULED", symbol: "S", tone: "text-[#f0e129]"};
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
  const positions = layoutHoldingGraph(entries.filter(({cleared_altitude}) => cleared_altitude === null || cleared_altitude <= HOLDING_GRAPH_MAX_FLIGHT_LEVEL * 100), now);
  const placed = positions.filter(({altitude}) => altitude.percent !== null);
  const unplaced = positions.filter(({altitude}) => altitude.percent === null);
  const instructionsId = useId();
  const arrowId = useId();
  const entryRefs = useRef(new Map<string, HTMLLIElement>());
  const graphRef = useRef<HTMLDivElement>(null);
  const footerRef = useRef<HTMLDivElement>(null);
  const [graphHeight, setGraphHeight] = useState(240);
  const [graphWidth, setGraphWidth] = useState(210);
  const contentWidth = Math.max(graphWidth, compact ? 100 : 300);
  const fontSize = Math.min(16, Math.max(compact ? 8 : 10, contentWidth * 16 / 293));
  const preferredRowHeight = compact ? Math.max(20, Math.round(fontSize * 1.65)) : 28;
  // Squeeze rows so the whole FL090-FL280 scale fits without scrolling.
  const levelCount = (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) / 10;
  const fittedRowHeight = Math.floor((graphHeight - 2 * (preferredRowHeight / 2 + 2)) / levelCount) - 1;
  const rowHeight = compact ? Math.min(preferredRowHeight, Math.max(12, fittedRowHeight)) : preferredRowHeight;
  // Leave subpixel slack rather than creating a scrollbar from rounding alone.
  const availableHeight = Math.max(0, Math.floor(graphHeight - Math.max(12, rowHeight / 2 + 2) * 2));
  // Scale text with squeezed rows so labels and altitude ticks don't overlap.
  const textSize = compact ? Math.min(fontSize, rowHeight / 1.65) : fontSize;
  const labels = layoutHoldingLabels(placed, availableHeight, rowHeight);
  const axisPadding = Math.max(12, labels.edgePadding);
  const plotHeight = labels.height;
  const timeAxisX = compact ? 18 + textSize * 1.2 : 50;
  const altitudeAxisX = contentWidth - (3 + 16 + textSize * 1.8);
  const labelRight = altitudeAxisX - 2;
  const labelWidth = Math.min(labelRight - (timeAxisX + 3 + LABEL_TRIANGLE_WIDTH + 6), textSize * 8.5);
  const labelLeft = labelRight - labelWidth;

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
    <section aria-label={holding ? `TMT holding information for ${holding}` : "TMT holding information"} className={cn("aman-holding-graph relative flex h-full min-w-0 flex-col border-[3px] border-[#dcdcdc] bg-[#3c3c3c] text-[#dcdcdc]", compact && "overflow-hidden")}>
      {!compact && <header className="flex items-center border-b border-[#777] px-3 py-2">
        <h2 className="font-display text-sm font-bold tracking-wide">TMT · HOLDING INFORMATION</h2>
        <span className="ml-auto font-mono text-[10px]">{entries.length} HOLDING</span>
      </header>}
      <p className="sr-only" id={instructionsId}>Use Tab to enter the aircraft list. Use arrow keys to move between aircraft, or Home and End to jump to the first or last aircraft.{onOpenFlightActions && " Press Enter or Space to open the flight menu."}</p>

      <div aria-describedby={instructionsId} className={cn("aman-holding-plot relative min-h-0 flex-1 overflow-auto bg-[#3c3c3c]", !compact && "mx-1")} data-testid="holding-graph" ref={graphRef} onScroll={(event) => {
        if (footerRef.current) footerRef.current.style.transform = `translateX(-${event.currentTarget.scrollLeft}px)`;
      }}>
        <div className="relative" style={{height: plotHeight + axisPadding * 2, width: contentWidth, fontSize: textSize}}>
        <div aria-hidden="true" className="absolute border-l-[3px] border-[#dcdcdc]" data-testid="holding-time-axis" style={{left: timeAxisX, top: axisPadding, bottom: axisPadding}}>
          {holdingTimeTicks(now).map(({date, minutes}) => (
            <span className="absolute right-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" key={minutes} style={{top: `${100 - minutes / HOLDING_GRAPH_WINDOW_MINUTES * 100}%`}}>
              {date.getUTCMinutes() % 10 === 5 && (
                <span className={cn("absolute right-4 top-0 -translate-y-1/2 font-semibold", compact ? "text-[0.9em]" : "text-[10px]")}>
                  {compact ? clockLabel(date).slice(3) : clockLabel(date)}
                </span>
              )}
            </span>
          ))}
        </div>

        <div aria-hidden="true" className="absolute border-l-[3px] border-[#dcdcdc]" data-testid="holding-altitude-axis" style={{left: altitudeAxisX, top: axisPadding, height: labels.height}}>
          {altitudeTicks.map((level) => (
            <span className="absolute left-0 w-3 -translate-y-1/2 border-t-2 border-[#dcdcdc]" data-flight-level={level} key={level} style={{top: labels.altitudeY((HOLDING_GRAPH_MAX_FLIGHT_LEVEL - level) / (HOLDING_GRAPH_MAX_FLIGHT_LEVEL - HOLDING_GRAPH_MIN_FLIGHT_LEVEL) * 100)}}>
              <span className="absolute left-4 top-0 -translate-y-1/2 font-semibold">{level}</span>
            </span>
          ))}
        </div>

        <svg aria-hidden="true" className="absolute left-0 overflow-visible" style={{top: axisPadding, height: plotHeight, width: contentWidth}} preserveAspectRatio="none" viewBox={`0 0 ${contentWidth} ${plotHeight}`}>
          <defs>
            <marker id={arrowId} markerWidth={LABEL_TRIANGLE_WIDTH} markerHeight="10" refX="0" refY="5" orient="0" markerUnits="userSpaceOnUse">
              <path d={`M0,5 L${LABEL_TRIANGLE_WIDTH},0 L${LABEL_TRIANGLE_WIDTH},10 Z`} fill="#dcdcdc" />
            </marker>
          </defs>
          {placed.map((position, index) => {
            if (position.time.percent === null) return null;
            return <line data-callsign={position.entry.callsign} key={position.entry.callsign} markerEnd={`url(#${arrowId})`} stroke="#dcdcdc" strokeWidth="1" vectorEffect="non-scaling-stroke" x1={timeAxisX + 1.5} x2={labelLeft - LABEL_TRIANGLE_WIDTH} y1={(100 - position.time.percent!) / 100 * plotHeight} y2={labels.labelY[index]} />;
          })}
        </svg>

        <ul className="absolute inset-x-0" style={{top: axisPadding, height: labels.height}}>
          {placed.map((position, index) => {
            const state = entryState(position);
            const source = sourceState(position.entry);
            return (
              <li
                aria-label={accessibleEntryLabel(position)}
                aria-keyshortcuts={`ArrowDown ArrowRight ArrowUp ArrowLeft Home End${onOpenFlightActions ? " Enter Space" : ""}`}
                title={onOpenFlightActions ? `Open flight menu for ${position.entry.callsign}` : undefined}
                data-altitude-y={labels.altitudeY(position.altitude.percent!)}
                className="absolute grid -translate-y-1/2 grid-cols-[39%_minmax(0,1fr)] bg-[#3c3c3c] font-semibold leading-none hover:z-10 focus:z-20 focus:outline focus:outline-2 focus:outline-offset-2 focus:outline-white"
                key={position.entry.callsign}
                onClick={() => onOpenFlightActions?.(position.entry.callsign)}
                onKeyDown={(event) => moveFocus(event, position.entry.callsign)}
                ref={(element) => setEntryRef(position.entry.callsign, element)}
                role={onOpenFlightActions ? "button" : undefined}
                style={{height: rowHeight, top: labels.labelY[index], left: labelLeft, width: labelWidth - 2}}
                tabIndex={0}
              >
                <span className={cn("flex items-center justify-center", source.symbol && "text-[0.85em]", state.tone)} title={position.urgency === "missing" ? "No calculated EAT. See this aircraft in Current warnings for the reason." : state.label}>
                  {!compact && <b className="mr-1 text-[8px]" aria-hidden="true">{state.symbol}</b>}{compactTime(position.time.label)}
                </span>
                <span className={cn("flex items-center justify-center whitespace-nowrap px-0.5 text-center", source.symbol && "text-[0.85em]")}>
                  {position.entry.callsign}
                  {source.symbol && <small className={cn("ml-1 border px-0.5 text-[8px]", source.tone)} title={source.label}>{source.symbol}</small>}
                </span>
              </li>
            );
          })}
        </ul>

        {placed.length === 0 && <p className={cn("absolute inset-0 grid place-items-center text-slate-300", compact ? "text-[9px]" : "text-xs")}>{compact ? "NONE" : "No positioned holding aircraft"}</p>}
        </div>
      </div>
        <div className={cn("shrink-0 overflow-hidden border-t-[3px] border-[#dcdcdc] bg-[#3b3b3b] text-center font-semibold", !compact && "mx-1")} style={{height: fontSize * 2.6, fontSize: fontSize * 1.125}}>
          <div className="grid h-full" data-testid="holding-graph-footer" ref={footerRef} style={{width: contentWidth, gridTemplateColumns: `${timeAxisX + 3}px ${altitudeAxisX - timeAxisX - 3}px 1fr`}}>
            <span className="grid place-items-center border-r-[3px] border-[#dcdcdc]">EAT</span><span className="grid place-items-center truncate">{holding ?? "HOLDING"}</span><span className="grid place-items-center border-l-[3px] border-[#dcdcdc]">ALT</span>
          </div>
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
