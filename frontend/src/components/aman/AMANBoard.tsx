import {useEffect, useMemo, useRef, useState} from "react";

import type {
  AMANConnectionState,
  AMANFlight,
  AMANPresentationStatus,
  AMANState,
} from "@/api/aman";
import {Dialog, DialogClose, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {AMAN_ALL_VIEW, availableAMANViews, controllerAMANViews, readAMANViewPreference, resolveAMANView, type AMANView} from "@/lib/aman-view-preference";
import {cn} from "@/lib/utils";
import {useWebSocketStore} from "@/store/store-hooks";
import {AMANAircraftTarget, type AMANAircraftTargetField} from "./AMANAircraftTarget";
import {AMANAircraftTargetPreferenceControls} from "./AMANAircraftTargetPreferences";
import {fieldsForAMANAircraftTargetSide, useAMANAircraftTargetPreferences} from "./amanAircraftTargetPreferenceModel";
import {ACCTimeline} from "./ACCTimeline";
import {FMPPairedTimeline} from "./FMPPairedTimeline";
import {RWYPairedTimeline} from "./RWYPairedTimeline";
import {AMANAxisTopPercent, AMANTimelineAxis, formatAMANAxisLabel, useAMANTimelineAxis} from "./AMANTimelineAxis";
import {
  buildAMANHoldingLanes,
  buildAMANLanes,
  formatAMANTime,
  layoutTimelineMarkers,
  type AMANTimelineRange,
} from "./presentation";

const badgeBase = "inline-flex items-center rounded border px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide";
const TIMELINE_PIXELS_PER_MINUTE = 18;
const RULER_WIDTH_PIXELS = 58;
const STRIP_STACK_PIXELS = 30;

function modeTone(mode: AMANState["effective_mode"]): string {
  switch (mode) {
    case "authoritative": return "border-emerald-400 bg-emerald-950 text-emerald-200";
    case "shadow": return "border-sky-400 bg-sky-950 text-sky-200";
    case "read_only": return "border-amber-400 bg-amber-950 text-amber-200";
    case "blocked": return "border-red-400 bg-red-950 text-red-200";
    case "disabled": return "border-slate-500 bg-slate-900 text-slate-300";
  }
}

function timelinePosition(timestamp: string | null, range: AMANTimelineRange): number | null {
  return AMANAxisTopPercent(timestamp, range);
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
  clockMs,
  axisStatus,
  gainLossAuthoritative,
  gainLossConnected,
  selectedFlightID,
  onSelectFlight,
  onOpenFlightDetails,
  leadingFields,
  trailingFields,
}: {
  label: string;
  flights: AMANFlight[];
  range: AMANTimelineRange;
  stripSide: "left" | "right";
  fillAvailableSpace: boolean;
  showStar: boolean;
  currentPosition: number | null;
  clockMs: number;
  axisStatus: "fresh" | "stale" | "disconnected";
  gainLossAuthoritative: boolean;
  gainLossConnected: boolean;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
  onOpenFlightDetails?: (flightID: string) => void;
  leadingFields: (flight: AMANFlight) => readonly AMANAircraftTargetField[];
  trailingFields: (flight: AMANFlight) => readonly AMANAircraftTargetField[];
}) {
  const minimumGapPercent = (60_000 / (range.endMs - range.startMs)) * 100;
  const markers = layoutTimelineMarkers(flights, range, minimumGapPercent);

  return (
    <section className={cn("relative h-full", fillAvailableSpace ? "min-w-[520px] flex-1" : "min-w-[520px]")} data-testid={`holding-timeline-lane-${label}`}>
      <div className="absolute inset-x-0 bottom-12 top-5">
        {currentPosition !== null && <div className="pointer-events-none absolute inset-x-0 bottom-0 z-0 bg-[#464646]" style={{top: `${currentPosition}%`}} />}
        <AMANTimelineAxis clockMs={clockMs} range={range} status={axisStatus} />
      {markers.map((marker) => {
        const selected = marker.flight.flight_id === selectedFlightID;
        const top = timelinePosition(marker.timestamp, range) ?? 0;
        const rulerEdge = stripSide === "left"
          ? `calc(50% - ${RULER_WIDTH_PIXELS / 2}px)`
          : `calc(50% + ${RULER_WIDTH_PIXELS / 2}px)`;
        const stackOffset = -marker.track * STRIP_STACK_PIXELS;
        return (
          <div key={marker.flight.flight_id}>
            <div
              className={cn(
                "absolute z-20 flex min-h-7 -translate-y-1/2 items-center",
                stripSide === "left" ? "-translate-x-full" : "translate-x-0",
              )}
              style={{left: rulerEdge, top: `calc(${top}% + ${stackOffset}px)`}}
            >
              {stripSide === "right" && <span className={cn(
                "relative h-px shrink-0",
                marker.flight.freeze_reason === "superstable" ? "bg-cyan-200" : marker.flight.freeze_reason === "manual" ? "bg-fuchsia-200" : "bg-[#a9bdc5]",
              )} style={{width: "24px"}}><i className="absolute -left-0.5 -top-0.5 block h-1 w-1 rounded-full bg-[#e4e4e4]" /></span>}
              <div className="flex min-h-7 items-stretch" data-marker-time={marker.timestamp} data-testid={`operational-marker-${marker.flight.flight_id}`}>
                <AMANAircraftTarget
                  flight={marker.flight}
                  guidance={{authoritative: gainLossAuthoritative, connected: gainLossConnected}}
                  leadingFields={leadingFields(marker.flight)}
                  onSelect={() => {
                    onSelectFlight(marker.flight.flight_id);
                    onOpenFlightDetails?.(marker.flight.flight_id);
                  }}
                  selected={selected}
                  trailingFields={trailingFields(marker.flight)}
                />
                {showStar && marker.flight.star_family && <span className="flex items-center border border-l-0 border-[#b8b8b8] bg-[#3f3f3f] px-1.5 font-mono text-[11px] text-[#a9bdc5]">{marker.flight.star_family}</span>}
              </div>
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
  onOpenFlightDetails?: (flightID: string) => void;
  accView?: AMANView;
  onACCViewChange?: (view: AMANView) => void;
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
  accView: suppliedACCView,
  onACCViewChange,
}: AMANBoardViewProps) {
  const position = useWebSocketStore((value) => value.position);
  const storedACCView = useWebSocketStore((value) => value.amanSelectedView);
  const setStoredACCView = useWebSocketStore((value) => value.setAMANSelectedView);
  const preferredACCView = readAMANViewPreference();
  const accView = suppliedACCView ?? resolveAMANView(
    state,
    preferredACCView === null ? null : storedACCView,
    controllerAMANViews(state, [position]),
  );
  const setACCView = onACCViewChange ?? setStoredACCView;
  const lanes = useMemo(() => state ? buildAMANLanes(state) : [], [state]);
  const [view, setView] = useState<"holds" | "runway" | "acc">("holds");
  const [selectedRunwayGroupID, setSelectedRunwayGroupID] = useState<string | null>(null);
  const [targetPreferences, setTargetPreferences] = useAMANAircraftTargetPreferences();
  const [targetPreferencesOpen, setTargetPreferencesOpen] = useState(false);
  const timelineScrollRef = useRef<HTMLDivElement>(null);
  const initializedTimelineScroll = useRef(false);
  const [timelineScroll, setTimelineScroll] = useState({top: 0, viewportHeight: 0, contentHeight: 0});
  const activeRunwayLane = lanes.find((lane) => lane.id === selectedRunwayGroupID) ?? lanes[0] ?? null;
  const timelineFlights = useMemo(() => activeRunwayLane?.flights ?? [], [activeRunwayLane]);
  const holdingLanes = useMemo(() => buildAMANHoldingLanes(timelineFlights), [timelineFlights]);
  const axis = useAMANTimelineAxis(state?.generated_at ?? new Date(0).toISOString());
  const range = axis.range;
  const timelineHeight = useMemo(
    () => Math.max(720, Math.ceil((range.endMs - range.startMs) / 60_000) * TIMELINE_PIXELS_PER_MINUTE),
    [range],
  );
  const nowPosition = AMANAxisTopPercent(axis.clockMs, range);
  const axisStatus = connectionState === "disconnected" ? "disconnected" : presentationStatus === "degraded" ? "stale" : "fresh";
  const gainLossAuthoritative = state?.authoritative === true && state.effective_mode === "authoritative";
  const targetFields = (flight: AMANFlight): AMANAircraftTargetField[] => [
    {id: "feeder-fix-eta", label: "Feeder-fix ETA", value: flight.feeder_fix_eta ? formatAMANTime(flight.feeder_fix_eta) : "—"},
    {id: "total-delay", label: "Total delay", value: flight.expected_holding_seconds === null ? "—" : `D${String(Math.ceil(flight.expected_holding_seconds / 60)).padStart(2, "0")}`},
    {id: "runway", label: "Runway", value: flight.slot?.runway_group_id ?? flight.runway_group_id ?? "—"},
    {id: "wtc", label: "WTC", value: "—"},
    {id: "aircraft-type", label: "Aircraft type", value: "—"},
    {id: "feeder-fix", label: "Feeder fix", value: flight.feeder_fix ?? "—"},
  ];
  const renderFMPTarget = (flight: AMANFlight) => (
    <AMANAircraftTarget
      flight={flight}
      guidance={{
        authoritative: gainLossAuthoritative,
        connected: connectionState === "connected",
      }}
      leadingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
      onSelect={() => {
        onSelectFlight(flight.flight_id);
        onOpenFlightDetails?.(flight.flight_id);
      }}
      selected={flight.flight_id === selectedFlightID}
      trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
    />
  );
  const renderRWYTarget = (flight: AMANFlight) => (
    <div className="flex min-h-7 items-stretch">
      {renderFMPTarget(flight)}
      {flight.star_family && <span className="flex items-center border border-l-0 border-[#b8b8b8] bg-[#3f3f3f] px-1.5 font-mono text-[11px] text-[#a9bdc5]">{flight.star_family}</span>}
    </div>
  );
  const renderACCTarget = (flight: AMANFlight) => {
    const emphasized = accView === AMAN_ALL_VIEW || flight.star_family === accView;
    return (
      <div className="flex min-h-7 items-stretch">
        <AMANAircraftTarget
          emphasis={emphasized ? "primary" : "subdued"}
          flight={flight}
          guidance={{authoritative: gainLossAuthoritative, connected: connectionState === "connected"}}
          leadingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
          onSelect={() => {
            onSelectFlight(flight.flight_id);
            onOpenFlightDetails?.(flight.flight_id);
          }}
          selected={flight.flight_id === selectedFlightID}
          trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
        />
        {flight.star_family && <span className={cn("flex items-center border border-l-0 border-[#b8b8b8] px-1.5 font-mono text-[11px]", emphasized ? "bg-[#3f3f3f] text-[#a9bdc5]" : "bg-[#686868] text-white")}>{emphasized && accView !== AMAN_ALL_VIEW ? "★ " : ""}{flight.star_family}</span>}
      </div>
    );
  };
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
          <button aria-label="Open target information preferences" className="grid min-w-[164px] place-items-center rounded-md bg-[#e4e4e4] px-3 text-center font-mono text-sm text-[#555] hover:ring-2 hover:ring-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-white" onClick={() => setTargetPreferencesOpen(true)} type="button">{new Date(state.generated_at).toISOString().slice(11, 19)}</button>
        </div>
        <div className="mt-1 flex h-9 items-center gap-1 rounded-sm bg-[#888] px-1">
          <span className="rounded border border-black bg-[#86a4af] px-3 py-1 text-xs font-bold">MAESTRO</span>
          <button aria-controls="aman-timeline-grid" aria-pressed={view === "holds"} className={cn("rounded border border-black px-3 py-1 text-xs font-bold", view === "holds" ? "bg-white text-black" : "bg-[#d6d6d6] text-black")} onClick={() => setView("holds")} type="button">ALL</button>
          <button aria-controls="aman-timeline-grid" aria-pressed={view === "runway"} className={cn("rounded border border-black px-3 py-1 text-xs font-bold", view === "runway" ? "bg-white text-black" : "bg-[#d6d6d6] text-black")} onClick={() => setView("runway")} type="button">RWY</button>
          <button aria-controls="aman-timeline-grid" aria-pressed={view === "acc"} className={cn("rounded border border-black px-3 py-1 text-xs font-bold", view === "acc" ? "bg-white text-black" : "bg-[#d6d6d6] text-black")} onClick={() => setView("acc")} type="button">ACC</button>
          {view === "acc" && <label className="ml-1 flex items-center gap-1 text-xs font-bold text-black">Emphasis
            <select aria-label="ACC STAR family emphasis" className="rounded border border-black bg-white px-1 py-0.5" onChange={(event) => setACCView(event.target.value)} value={accView}>
              <option value={AMAN_ALL_VIEW}>ALL</option>
              {[...availableAMANViews(state)].map((family) => <option key={family} value={family}>{family}</option>)}
            </select>
          </label>}
          <span className="rounded border border-black bg-[#d6d6d6] px-3 py-1 text-xs font-bold text-black">DSEQ - 0</span>
          <span className="ml-2 border-l border-black/40 pl-2 font-mono text-xs text-black">{formatAMANAxisLabel(range.startMs, range.startMs)}–{formatAMANAxisLabel(range.endMs, range.startMs)} UTC · {axis.horizonMinutes} min</span>
          <span className={cn("ml-auto", badgeBase, modeTone(state.effective_mode))}>{state.effective_mode.replace("_", " ")}</span>
          <span className={cn(badgeBase, connectionState === "connected" ? "border-emerald-400 bg-emerald-950 text-emerald-200" : "border-red-400 bg-red-950 text-red-100")}>{connectionState}</span>
          {presentationStatus !== "ready" && <span className={cn(badgeBase, "border-amber-400 bg-amber-950 text-amber-100")}>{presentationStatus}</span>}
        </div>
      </header>

      <div className="relative min-h-0 flex-1">
        <div className="h-full overflow-auto pl-9 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" onScroll={syncTimelineScroll} ref={timelineScrollRef}>
          <div className={cn("relative flex", view === "holds" ? "min-w-max" : "min-w-full")} data-testid="aman-timeline-grid" id="aman-timeline-grid" style={{height: `${timelineHeight}px`}}>
            {view === "runway" ? (
              <RWYPairedTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                range={range}
                renderTarget={renderRWYTarget}
                state={state}
                status={axisStatus}
              />
            ) : view === "acc" ? (
              <ACCTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                flights={state.flights}
                range={range}
                renderTarget={renderACCTarget}
                status={axisStatus}
              />
            ) : state.timeline_configuration !== undefined ? (
              <FMPPairedTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                flights={state.flights}
                mappings={state.timeline_configuration.mappings}
                range={range}
                renderTarget={renderFMPTarget}
                status={axisStatus}
              />
            ) : holdingLanes.map((lane, index) => (
              <HoldingTimeline
                flights={lane.flights}
                key={lane.id}
                label={lane.label}
                onSelectFlight={onSelectFlight}
                onOpenFlightDetails={onOpenFlightDetails}
                range={range}
                selectedFlightID={selectedFlightID}
                stripSide={index % 2 === 0 ? "left" : "right"}
                fillAvailableSpace={false}
                showStar={false}
                currentPosition={nowPosition}
                clockMs={axis.clockMs}
                axisStatus={axisStatus}
                gainLossAuthoritative={gainLossAuthoritative}
                gainLossConnected={connectionState === "connected"}
                leadingFields={(flight) => fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
                trailingFields={(flight) => fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
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
        <button className="ml-2 border border-slate-300 bg-[#4b5563] px-4 py-2 font-display text-sm font-bold text-white hover:bg-[#5b6676] disabled:cursor-not-allowed disabled:opacity-50" disabled={selectedFlightID === null} onClick={() => selectedFlightID !== null && onOpenFlightDetails?.(selectedFlightID)} type="button">DETAIL</button>
        <span className="ml-4 text-xs text-slate-300">{activeRunwayLane?.label ?? "No runway group"} · operational marker</span>
        {state.technical_health.blocked_reasons.length > 0 && <span className="ml-auto text-xs text-red-200">{state.technical_health.blocked_reasons.join(", ")}</span>}
      </footer>
      <Dialog onOpenChange={setTargetPreferencesOpen} open={targetPreferencesOpen}>
        <DialogContent className="w-[min(720px,calc(100vw-2rem))] bg-[#e4e4e4] text-[#202020]">
          <DialogTitle>Target information</DialogTitle>
          <AMANAircraftTargetPreferenceControls onChange={setTargetPreferences} preferences={targetPreferences} />
          <DialogClose className="justify-self-end rounded border border-black bg-[#555355] px-4 py-2 text-sm font-semibold text-white hover:bg-[#6b696b]">Close target information</DialogClose>
        </DialogContent>
      </Dialog>
    </section>
  );
}
