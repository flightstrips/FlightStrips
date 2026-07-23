import {useEffect, useMemo, useRef, useState} from "react";

import type {
  AMANConnectionState,
  AMANFlight,
  AMANPresentationStatus,
  AMANState,
} from "@/api/aman";
import {cn} from "@/lib/utils";
import {
  buildAMANHoldingLanes,
  buildAMANLanes,
  formatAMANTime,
  formatGainLoss,
  layoutTimelineMarkers,
  operationalMarkerTimestamp,
  type AMANTimelineRange,
} from "./presentation";

const badgeBase = "inline-flex items-center rounded border px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide";
const TIMELINE_PIXELS_PER_MINUTE = 18;
const RULER_WIDTH_PIXELS = 58;
const STRIP_STACK_PIXELS = 18;

function modeTone(mode: AMANState["effective_mode"]): string {
  switch (mode) {
    case "authoritative": return "border-emerald-400 bg-emerald-950 text-emerald-200";
    case "shadow": return "border-sky-400 bg-sky-950 text-sky-200";
    case "read_only": return "border-amber-400 bg-amber-950 text-amber-200";
    case "blocked": return "border-red-400 bg-red-950 text-red-200";
    case "disabled": return "border-slate-500 bg-slate-900 text-slate-300";
  }
}

function timelinePercent(timestamp: string | null, range: AMANTimelineRange): number | null {
  if (timestamp === null) return null;
  const value = Date.parse(timestamp);
  if (!Number.isFinite(value)) return null;
  return Math.max(0, Math.min(100, ((value - range.startMs) / (range.endMs - range.startMs)) * 100));
}

/** The operational ruler follows the reference: later times are above earlier times. */
function timelinePosition(timestamp: string | null, range: AMANTimelineRange): number | null {
  const percent = timelinePercent(timestamp, range);
  return percent === null ? null : 100 - percent;
}

function buildScrollableTimelineRange(flights: AMANFlight[], generatedAt: string): AMANTimelineRange {
  const timestamps = [generatedAt, ...flights.flatMap((flight) => [operationalMarkerTimestamp(flight), flight.raw_teta])]
    .map((value) => value === null ? Number.NaN : Date.parse(value))
    .filter(Number.isFinite);
  const fallback = Date.parse(generatedAt);
  const minimum = timestamps.length > 0 ? Math.min(...timestamps) : fallback;
  const maximum = timestamps.length > 0 ? Math.max(...timestamps) : fallback;
  const hour = 3_600_000;
  const startMs = Math.floor(minimum / hour) * hour;
  const endMs = Math.max(startMs + hour, Math.ceil(maximum / hour) * hour);
  return {startMs, endMs};
}

function buildTimelineTicks(range: AMANTimelineRange): Array<{position: number; timestamp: string; major: boolean}> {
  const minute = 60_000;
  const count = Math.round((range.endMs - range.startMs) / minute);
  return Array.from({length: count + 1}, (_, index) => {
    const timeMs = range.startMs + index * minute;
    return {
      position: 100 - index / count * 100,
      timestamp: new Date(timeMs).toISOString(),
      major: new Date(timeMs).getUTCMinutes() % 5 === 0,
    };
  });
}

function TimelineRuler({range, currentPosition}: {range: AMANTimelineRange; currentPosition: number | null}) {
  const ticks = buildTimelineTicks(range);

  return (
    <div className="absolute inset-y-0 left-1/2 w-[58px] -translate-x-1/2 overflow-visible border border-[#d8d8d8]" aria-hidden="true">
      {currentPosition !== null && <>
        <div className="absolute inset-x-0 bottom-0 bg-[#3a3a3a]" style={{top: `${currentPosition}%`}} />
        <div className="absolute inset-x-0 z-10 -translate-y-1/2" style={{top: `${currentPosition}%`}}>
          {/* Each ruler border is bracketed as >|<, not a chevron pair on one side. */}
          <i className="absolute top-1/2 h-0 w-0 -translate-y-1/2 border-y-[4px] border-y-transparent border-l-[5px] border-l-white" style={{right: "calc(100% + 1px)"}} />
          <i className="absolute left-px top-1/2 h-0 w-0 -translate-y-1/2 border-y-[4px] border-y-transparent border-r-[5px] border-r-white" />
          <span className="absolute left-[7px] right-[7px] top-1/2 border-t border-dashed border-white" />
          <i className="absolute right-px top-1/2 h-0 w-0 -translate-y-1/2 border-y-[4px] border-y-transparent border-l-[5px] border-l-white" />
          <i className="absolute top-1/2 h-0 w-0 -translate-y-1/2 border-y-[4px] border-y-transparent border-r-[5px] border-r-white" style={{left: "calc(100% + 1px)"}} />
        </div>
      </>}
      {ticks.map(({position, major, timestamp}) => {
        const isCurrentMarker = currentPosition !== null && Math.abs(position - currentPosition) < 0.5;
        return (
          <div className="absolute inset-x-0 z-[1] -translate-y-1/2" key={timestamp} style={{top: `${position}%`}}>
            <span className={cn("absolute left-0 block h-px bg-[#d8d8d8]", major ? "w-3" : "w-1.5")} />
            <span className={cn("absolute right-0 block h-px bg-[#d8d8d8]", major ? "w-3" : "w-1.5")} />
            {major && !isCurrentMarker && <span className="absolute left-1/2 -translate-x-1/2 -translate-y-1/2 bg-[#505052] px-0.5 font-mono text-[11px] font-semibold text-white">{formatAMANTime(timestamp)}</span>}
          </div>
        );
      })}
    </div>
  );
}

function TimelineScrollRail({
  scrollTop,
  viewportHeight,
  contentHeight,
  currentPosition,
  onScrollTo,
  onJumpToCurrent,
}: {
  scrollTop: number;
  viewportHeight: number;
  contentHeight: number;
  currentPosition: number | null;
  onScrollTo: (scrollTop: number) => void;
  onJumpToCurrent: () => void;
}) {
  const maximumScroll = Math.max(0, contentHeight - viewportHeight);
  const thumbHeight = viewportHeight > 0 && contentHeight > 0
    ? Math.max(44, Math.min(viewportHeight - 28, viewportHeight * viewportHeight / contentHeight))
    : 44;
  const travel = Math.max(0, viewportHeight - 28 - thumbHeight);
  const thumbTop = maximumScroll === 0 ? 0 : scrollTop / maximumScroll * travel;
  const tickPositions = Array.from({length: 72}, (_, index) => index / 71 * 100);
  const setScrollFromPointer = (clientY: number, rail: HTMLDivElement) => {
    const bounds = rail.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (clientY - bounds.top - 14 - thumbHeight / 2) / Math.max(1, travel)));
    onScrollTo(ratio * maximumScroll);
  };

  return (
    <div
      aria-controls="aman-timeline-grid"
      aria-label="Timeline scroll position"
      aria-orientation="vertical"
      aria-valuemax={maximumScroll}
      aria-valuemin={0}
      aria-valuenow={Math.round(scrollTop)}
      className="absolute bottom-0 left-0 top-0 z-40 w-9 border border-[#ddd] bg-[#242424] touch-none"
      onPointerDown={(event) => {
        event.currentTarget.setPointerCapture(event.pointerId);
        setScrollFromPointer(event.clientY, event.currentTarget);
      }}
      onPointerMove={(event) => {
        if (event.currentTarget.hasPointerCapture(event.pointerId)) setScrollFromPointer(event.clientY, event.currentTarget);
      }}
      role="scrollbar"
    >
      <div className="absolute inset-x-2 bottom-8 top-1 border-x border-[#dadada]">
        {tickPositions.map((position) => (
          <span
            className={cn("absolute left-1 right-1 h-px", currentPosition !== null && position > currentPosition ? "bg-lime-300" : "bg-[#e2e2e2]")}
            key={position}
            style={{top: `${position}%`}}
          />
        ))}
      </div>
      <div
        className="absolute left-1 right-1 z-10 rounded-md border-2 border-[#e4e4e4] bg-[#565656] shadow-[inset_0_0_0_2px_#2b2b2b]"
        style={{height: `${thumbHeight}px`, top: `${14 + thumbTop}px`}}
      >
        <span className="absolute inset-x-1 top-1/2 border-t border-[#a8a8a8]" />
      </div>
      <button aria-label="Jump to current timeline time" className="absolute bottom-0 left-0 grid h-8 w-full place-items-center border-t border-[#ddd] text-lg font-bold text-white hover:bg-[#505050]" onClick={onJumpToCurrent} type="button">↻</button>
    </div>
  );
}

function HoldingTimeline({
  label,
  flights,
  range,
  stripSide,
  fillAvailableSpace,
  showStar,
  currentPosition,
  selectedFlightID,
  onSelectFlight,
}: {
  label: string;
  flights: AMANFlight[];
  range: AMANTimelineRange;
  stripSide: "left" | "right";
  fillAvailableSpace: boolean;
  showStar: boolean;
  currentPosition: number | null;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
}) {
  const minimumGapPercent = (60_000 / (range.endMs - range.startMs)) * 100;
  const markers = layoutTimelineMarkers(flights, range, minimumGapPercent);

  return (
    <section className={cn("relative h-full", fillAvailableSpace ? "min-w-[520px] flex-1" : "min-w-[520px]")} data-testid={`holding-timeline-lane-${label}`}>
      <div className="absolute inset-x-0 bottom-12 top-5">
        {currentPosition !== null && <div className="pointer-events-none absolute inset-x-0 bottom-0 z-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />}
        <TimelineRuler currentPosition={currentPosition} range={range} />
      {markers.map((marker) => {
        const selected = marker.flight.flight_id === selectedFlightID;
        const top = timelinePosition(marker.timestamp, range) ?? 0;
        const gainLoss = formatGainLoss(marker.flight.gain_loss_seconds);
        const gainLossTone = marker.flight.gain_loss_seconds === null
          ? "text-slate-300"
          : marker.flight.gain_loss_seconds < 0
            ? "text-amber-300"
            : marker.flight.gain_loss_seconds > 0
              ? "text-lime-300"
              : "text-white";
        const sequence = String(marker.flight.order ?? marker.flight.slot?.sequence ?? "").padStart(2, "0");
        const star = showStar ? marker.flight.star : null;
        const rulerEdge = stripSide === "left"
          ? `calc(50% - ${RULER_WIDTH_PIXELS / 2}px)`
          : `calc(50% + ${RULER_WIDTH_PIXELS / 2}px)`;
        const stackOffset = -marker.track * STRIP_STACK_PIXELS;
        return (
          <div key={marker.flight.flight_id}>
            <div
              className={cn(
                "absolute z-20 flex h-4 -translate-y-1/2 items-center",
                stripSide === "left" ? "-translate-x-full" : "translate-x-0",
              )}
              style={{left: rulerEdge, top: `calc(${top}% + ${stackOffset}px)`}}
            >
              {stripSide === "right" && <span className={cn(
                "relative h-px shrink-0",
                marker.flight.freeze_reason === "superstable" ? "bg-cyan-200" : marker.flight.freeze_reason === "manual" ? "bg-fuchsia-200" : "bg-[#a9bdc5]",
              )} style={{width: "24px"}}><i className="absolute -left-0.5 -top-0.5 block h-1 w-1 rounded-full bg-[#e4e4e4]" /></span>}
              <button
                aria-label={`Select ${marker.flight.callsign} timeline marker`}
                className={cn(
                  "flex h-4 items-center gap-1 border border-[#666] bg-[#303030] px-1.5 text-left font-mono text-[10px] font-semibold leading-none text-[#e8e8e8] shadow-[0_1px_1px_rgb(0_0_0_/_70%)] focus:outline-none focus:ring-2 focus:ring-white",
                  star === null ? "w-[204px]" : "w-[260px]",
                  marker.flight.freeze_reason === "superstable" && "border-cyan-200",
                  marker.flight.freeze_reason === "manual" && "border-fuchsia-200",
                  selected && "border-[#f3d02e] ring-1 ring-[#f3d02e]",
                )}
                data-marker-time={marker.timestamp}
                data-testid={`operational-marker-${marker.flight.flight_id}`}
                onClick={() => onSelectFlight(marker.flight.flight_id)}
                title={`${marker.flight.callsign} · ${formatAMANTime(marker.timestamp)}${star === null ? "" : ` · STAR ${star}`} · ${marker.flight.lifecycle_state} · ${marker.flight.data_status}`}
                type="button"
              >
                <span className="w-4 text-right text-slate-300">{sequence}</span>
                <span className="w-[66px] truncate">{marker.flight.callsign}</span>
                <span>{formatAMANTime(marker.timestamp)}</span>
                {star !== null && <span className="w-[50px] truncate text-[#a9bdc5]">{star}</span>}
                <span className={cn("ml-auto", gainLossTone)}>{gainLoss}</span>
              </button>
              {stripSide === "left" && <span className={cn(
                "relative h-px shrink-0",
                marker.flight.freeze_reason === "superstable" ? "bg-cyan-200" : marker.flight.freeze_reason === "manual" ? "bg-fuchsia-200" : "bg-[#a9bdc5]",
              )} style={{width: "24px"}}><i className="absolute -right-0.5 -top-0.5 block h-1 w-1 rounded-full bg-[#e4e4e4]" /></span>}
            </div>
            {marker.track > 0 && <span
              aria-hidden="true"
              className="absolute z-10 w-px bg-[#a9bdc5]"
              style={{height: `${Math.abs(stackOffset)}px`, left: rulerEdge, top: `calc(${top}% + ${stackOffset}px)`}}
            />}
          </div>
        );
      })}
      </div>
      <footer className="absolute bottom-0 left-0 right-0 pb-3 text-center font-display text-sm font-bold text-white">{label}</footer>
    </section>
  );
}

export interface AMANBoardViewProps {
  state: AMANState | null;
  presentationStatus: AMANPresentationStatus;
  error: string | null;
  connectionState: AMANConnectionState;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
  onOpenControls?: () => void;
  onOpenFlightDetails?: () => void;
}

export function AMANBoardView({
  state,
  presentationStatus,
  error,
  connectionState,
  selectedFlightID,
  onSelectFlight,
  onOpenControls,
  onOpenFlightDetails,
}: AMANBoardViewProps) {
  const lanes = useMemo(() => state ? buildAMANLanes(state) : [], [state]);
  const [view, setView] = useState<"holds" | "runway">("holds");
  const [selectedRunwayGroupID, setSelectedRunwayGroupID] = useState<string | null>(null);
  const timelineScrollRef = useRef<HTMLDivElement>(null);
  const initializedTimelineScroll = useRef(false);
  const [timelineScroll, setTimelineScroll] = useState({top: 0, viewportHeight: 0, contentHeight: 0});
  const activeRunwayLane = lanes.find((lane) => lane.id === selectedRunwayGroupID) ?? lanes[0] ?? null;
  const timelineFlights = useMemo(() => activeRunwayLane?.flights ?? [], [activeRunwayLane]);
  const holdingLanes = useMemo(() => buildAMANHoldingLanes(timelineFlights), [timelineFlights]);
  const range = useMemo(() => buildScrollableTimelineRange(timelineFlights, state?.generated_at ?? ""), [state?.generated_at, timelineFlights]);
  const timelineHeight = useMemo(
    () => Math.max(720, Math.ceil((range.endMs - range.startMs) / 60_000) * TIMELINE_PIXELS_PER_MINUTE),
    [range],
  );
  const visibleTimelineLanes = useMemo(() => view === "runway"
    ? [{id: "runway", label: activeRunwayLane?.label ?? "Runway", flights: timelineFlights}]
    : holdingLanes, [activeRunwayLane?.label, holdingLanes, timelineFlights, view]);
  const nowPosition = timelinePosition(state?.generated_at ?? null, range);
  const syncTimelineScroll = () => {
    const timeline = timelineScrollRef.current;
    if (timeline === null) return;
    setTimelineScroll({
      top: timeline.scrollTop,
      viewportHeight: timeline.clientHeight,
      contentHeight: timeline.scrollHeight,
    });
  };
  const jumpToCurrentTime = () => {
    const timeline = timelineScrollRef.current;
    if (timeline === null || nowPosition === null) return;
    timeline.scrollTop = Math.max(0, (nowPosition / 100) * timelineHeight - timeline.clientHeight * 0.35);
    syncTimelineScroll();
  };
  useEffect(() => {
    if (initializedTimelineScroll.current || nowPosition === null) return;
    const timeline = timelineScrollRef.current;
    if (timeline === null) return;
    timeline.scrollTop = Math.max(0, (nowPosition / 100) * timelineHeight - timeline.clientHeight * 0.35);
    initializedTimelineScroll.current = true;
    syncTimelineScroll();
  }, [nowPosition, timelineHeight]);
  useEffect(() => {
    syncTimelineScroll();
    window.addEventListener("resize", syncTimelineScroll);
    return () => window.removeEventListener("resize", syncTimelineScroll);
  }, [timelineHeight]);

  if (state === null) {
    return (
      <section aria-label="AMAN presentation" className="grid min-h-72 place-items-center bg-[#505052] p-8 text-center text-white">
        <div>
          <h1 className="font-display text-2xl font-bold">AMAN timeline unavailable</h1>
          <p className="mt-2 text-slate-200">{error ? `State rejected: ${error}` : "Waiting for a complete AMAN state replacement."}</p>
        </div>
      </section>
    );
  }

  return (
    <section aria-label="AMAN presentation" className="flex h-[calc(95.28dvh-24px)] min-h-[640px] w-full max-w-[1440px] flex-col overflow-hidden bg-[#505052] text-white shadow-2xl">
      <header className="shrink-0 bg-[#292929] p-1.5">
        <div className="flex h-16 gap-1 overflow-x-auto">
          <div className="grid place-items-center rounded-md bg-[#f3d02e] px-3 font-display text-xl font-bold text-black">{state.airport}</div>
          {lanes.map((lane) => (
            <button
              aria-pressed={activeRunwayLane?.id === lane.id}
              className={cn(
                "min-w-[120px] rounded-md border border-black px-4 text-left font-display text-lg font-bold text-black",
                activeRunwayLane?.id === lane.id ? "bg-[#f3d02e] ring-2 ring-white" : "bg-[#e6c933] hover:bg-[#f3d02e]",
              )}
              key={lane.id}
              onClick={() => setSelectedRunwayGroupID(lane.id)}
              type="button"
            >
              {lane.label} : {lane.flights.length}
            </button>
          ))}
          <div className="ml-auto grid min-w-[126px] place-items-center rounded-md bg-[#e4e4e4] px-3 text-center text-xs text-black">TMA: {state.flights.length}<br />Health: {state.technical_health.status}</div>
          <div className="grid min-w-[164px] place-items-center rounded-md bg-[#e4e4e4] px-3 text-center font-mono text-sm text-[#555]">{new Date(state.generated_at).toISOString().slice(11, 19)}</div>
        </div>
        <div className="mt-1 flex h-9 items-center gap-1 rounded-sm bg-[#888] px-1">
          <span className="rounded border border-black bg-[#86a4af] px-3 py-1 text-xs font-bold">MAESTRO</span>
          <button className={cn("rounded border border-black px-3 py-1 text-xs font-bold", view === "holds" ? "bg-white text-black" : "bg-[#d6d6d6] text-black")} onClick={() => setView("holds")} type="button">ALL</button>
          <button className={cn("rounded border border-black px-3 py-1 text-xs font-bold", view === "runway" ? "bg-white text-black" : "bg-[#d6d6d6] text-black")} onClick={() => setView("runway")} type="button">RWY</button>
          <span className="rounded border border-black bg-[#d6d6d6] px-3 py-1 text-xs font-bold text-black">DSEQ - 0</span>
          <span className="ml-2 border-l border-black/40 pl-2 font-mono text-xs text-black">{formatAMANTime(new Date(range.startMs).toISOString())}–{formatAMANTime(new Date(range.endMs).toISOString())} · scroll timeline</span>
          <span className={cn("ml-auto", badgeBase, modeTone(state.effective_mode))}>{state.effective_mode.replace("_", " ")}</span>
          <span className={cn(badgeBase, connectionState === "connected" ? "border-emerald-400 bg-emerald-950 text-emerald-200" : "border-red-400 bg-red-950 text-red-100")}>{connectionState}</span>
          {presentationStatus !== "ready" && <span className={cn(badgeBase, "border-amber-400 bg-amber-950 text-amber-100")}>{presentationStatus}</span>}
        </div>
      </header>

      <div className="relative min-h-0 flex-1">
        <div className="h-full overflow-auto pl-9 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" onScroll={syncTimelineScroll} ref={timelineScrollRef}>
          <div className={cn("relative flex", view === "runway" ? "min-w-full" : "min-w-max")} data-testid="aman-timeline-grid" id="aman-timeline-grid" style={{height: `${timelineHeight}px`}}>
            {visibleTimelineLanes.map((lane, index) => (
              <HoldingTimeline
                flights={lane.flights}
                key={lane.id}
                label={lane.label}
                onSelectFlight={onSelectFlight}
                range={range}
                selectedFlightID={selectedFlightID}
                stripSide={index % 2 === 0 ? "left" : "right"}
                fillAvailableSpace={view === "runway"}
                showStar={view === "runway"}
                currentPosition={nowPosition}
              />
            ))}
          </div>
        </div>
        <TimelineScrollRail
          contentHeight={timelineScroll.contentHeight || timelineHeight}
          currentPosition={nowPosition}
          onJumpToCurrent={jumpToCurrentTime}
          onScrollTo={(scrollTop) => {
            if (timelineScrollRef.current !== null) {
              timelineScrollRef.current.scrollTop = scrollTop;
              syncTimelineScroll();
            }
          }}
          scrollTop={timelineScroll.top}
          viewportHeight={timelineScroll.viewportHeight}
        />
      </div>

      <footer className="flex h-14 shrink-0 items-center border-t-4 border-[#292929] bg-[#353535] px-4">
        <button className="bg-lime-400 px-5 py-2 font-display text-lg font-bold text-black shadow-[0_2px_0_#1c1c1c]" onClick={onOpenControls} type="button">FMP</button>
        <button className="ml-2 border border-slate-300 bg-[#4b5563] px-4 py-2 font-display text-sm font-bold text-white hover:bg-[#5b6676] disabled:cursor-not-allowed disabled:opacity-50" disabled={selectedFlightID === null} onClick={onOpenFlightDetails} type="button">DETAIL</button>
        <span className="ml-4 text-xs text-slate-300">{activeRunwayLane?.label ?? "No runway group"} · operational marker</span>
        {state.technical_health.blocked_reasons.length > 0 && <span className="ml-auto text-xs text-red-200">{state.technical_health.blocked_reasons.join(", ")}</span>}
      </footer>
    </section>
  );
}
