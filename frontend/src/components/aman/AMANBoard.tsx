import {useEffect, useMemo, useRef, useState} from "react";

import type {
  AMANConnectionState,
  AMANFlight,
  AMANPresentationStatus,
  AMANRunwayGap,
  AMANState,
} from "@/api/aman";
import {getAMANMutationBlockReason} from "@/api/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {AMAN_ALL_VIEW, availableAMANViews, controllerAMANViews, readAMANViewPreference, resolveAMANView, type AMANView} from "@/lib/aman-view-preference";
import {defaultACCFamilies} from "@/lib/aman-default-families";
import {useWebSocketStore} from "@/store/store-hooks";
import {AMANAircraftTarget, type AMANAircraftTargetField} from "./AMANAircraftTarget";
import {AMANAircraftTargetPreferenceControls} from "./AMANAircraftTargetPreferences";
import {AMANSettingsHeader} from "./AMANSettingsHeader";
import {fieldsForAMANAircraftTargetSide, useAMANAircraftTargetPreferences} from "./amanAircraftTargetPreferenceModel";
import {ACCTimeline} from "./ACCTimeline";
import {FMPPairedTimeline, FMPPairedTimelineFooter} from "./FMPPairedTimeline";
import {RWYPairedTimeline} from "./RWYPairedTimeline";
import {AMANAxisTopPercent, AMAN_MAXIMUM_HORIZON_MINUTES, AMAN_MINIMUM_HORIZON_MINUTES, useAMANTimelineAxis} from "./AMANTimelineAxis";
import {buildAMANLanes, formatAMANTime} from "./presentation";

const MINIMUM_TIMELINE_HEIGHT_PIXELS = 960;
const WHEEL_STEP_MINUTES = 2;
/** Quiet time at "now" before a continued wheel-down starts compressing. */
const WHEEL_COMPRESS_PAUSE_MS = 300;

const STAR_FAMILY_COLORS: Record<string, string> = {
  ERNOV: "#95D692", TESPI: "#95D692", TUDLO: "#95D692",
  TIDVU: "#68A3C3", MONAK: "#68A3C3",
};
const starFamilyColor = (family: string) => STAR_FAMILY_COLORS[family.trim().toUpperCase()];
type RailDrag =
  | {mode: "scroll"; grab: number}
  | {mode: "compress"; startY: number; startHorizon: number};

const SCROLL_RANGE_MINUTES = AMAN_MAXIMUM_HORIZON_MINUTES - AMAN_MINIMUM_HORIZON_MINUTES;

/**
 * The rail is one continuous control, later times up. At the default
 * horizon the thumb scrolls through the 90-minute look-ahead. Pushed past the
 * bottom (now), it squashes: a smaller thumb is a more compressed timeline.
 * Left-drag moves through both; right-drag compresses directly (down) or
 * expands (up).
 */
function TimelineScrollRail({
  horizonMinutes,
  offsetMinutes,
  viewportHeight,
  onViewChange,
}: {
  horizonMinutes: number;
  offsetMinutes: number;
  viewportHeight: number;
  onViewChange: (horizonMinutes: number, offsetMinutes: number) => void;
}) {
  const railTopPad = 4;
  const inner = Math.max(1, viewportHeight - railTopPad - 4);
  const baseHeight = inner * AMAN_MINIMUM_HORIZON_MINUTES / AMAN_MAXIMUM_HORIZON_MINUTES;
  const travel = inner - baseHeight;
  const compressed = horizonMinutes > AMAN_MINIMUM_HORIZON_MINUTES;
  const thumbHeight = baseHeight * AMAN_MINIMUM_HORIZON_MINUTES / horizonMinutes;
  const thumbTop = compressed ? inner - thumbHeight : travel * (1 - offsetMinutes / SCROLL_RANGE_MINUTES);
  const drag = useRef<RailDrag | null>(null);
  const pointerY = (clientY: number, rail: HTMLDivElement) => clientY - rail.getBoundingClientRect().top - railTopPad;
  // Above the bottom the thumb top sets the scroll position; below it, the
  // thumb is squashed against the bottom and its height sets the compression.
  const moveThumbTo = (top: number) => {
    if (top > travel) onViewChange(AMAN_MINIMUM_HORIZON_MINUTES * baseHeight / Math.max(1e-6, inner - top), 0);
    else onViewChange(AMAN_MINIMUM_HORIZON_MINUTES, SCROLL_RANGE_MINUTES * (1 - top / Math.max(1, travel)));
  };
  // Up scrolls later, or first expands a compressed timeline; down scrolls
  // back toward now, then compresses.
  const step = (minutes: number) => {
    if (minutes > 0) {
      if (compressed) onViewChange(Math.max(AMAN_MINIMUM_HORIZON_MINUTES, horizonMinutes - minutes), 0);
      else onViewChange(AMAN_MINIMUM_HORIZON_MINUTES, offsetMinutes + minutes);
    } else if (offsetMinutes > 0) {
      onViewChange(AMAN_MINIMUM_HORIZON_MINUTES, Math.max(0, offsetMinutes + minutes));
    } else {
      onViewChange(horizonMinutes - minutes, 0);
    }
  };
  const position = compressed ? AMAN_MINIMUM_HORIZON_MINUTES - horizonMinutes : Math.round(offsetMinutes);

  return (
    <div
      aria-controls="aman-timeline-grid"
      aria-label="Timeline scroll position"
      aria-orientation="vertical"
      aria-valuemax={SCROLL_RANGE_MINUTES}
      aria-valuemin={-SCROLL_RANGE_MINUTES}
      aria-valuenow={position}
      aria-valuetext={`${Math.round(offsetMinutes)} to ${Math.round(offsetMinutes + horizonMinutes)} minutes ahead, ${horizonMinutes}-minute horizon`}
      className="absolute bottom-0 left-0 top-1 z-40 w-[13.2px] overflow-hidden rounded-md border border-white touch-none"
      onContextMenu={(event) => event.preventDefault()}
      onPointerDown={(event) => {
        if (event.button !== 0 && event.button !== 2) return;
        event.preventDefault();
        event.currentTarget.setPointerCapture(event.pointerId);
        if (event.button === 2) {
          drag.current = {mode: "compress", startY: event.clientY, startHorizon: horizonMinutes};
          return;
        }
        const y = pointerY(event.clientY, event.currentTarget);
        // Grabbing the thumb keeps the grabbed point under the pointer; a click
        // on the track centres the thumb there, like a page jump.
        const onThumb = y >= thumbTop && y <= thumbTop + thumbHeight;
        drag.current = {mode: "scroll", grab: onThumb ? y - thumbTop : thumbHeight / 2};
        if (!onThumb) moveThumbTo(y - thumbHeight / 2);
      }}
      onPointerMove={(event) => {
        const current = drag.current;
        if (current === null || !event.currentTarget.hasPointerCapture(event.pointerId)) return;
        if (current.mode === "scroll") {
          moveThumbTo(pointerY(event.clientY, event.currentTarget) - current.grab);
        } else {
          // Dragging down compresses, matching a left-drag past the bottom.
          onViewChange(current.startHorizon + (event.clientY - current.startY) / inner * AMAN_MAXIMUM_HORIZON_MINUTES, 0);
        }
      }}
      onPointerUp={(event) => {
        drag.current = null;
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
      }}
      onPointerCancel={() => {
        drag.current = null;
      }}
      onKeyDown={(event) => {
        const minutes = event.key === "ArrowUp" ? 1 : event.key === "ArrowDown" ? -1 : event.key === "PageUp" ? 5 : event.key === "PageDown" ? -5 : null;
        if (minutes !== null) {
          event.preventDefault();
          step(minutes);
        } else if (event.key === "Home" || event.key === "End") {
          // Home jumps to the latest look-ahead; End returns to the default view at now.
          event.preventDefault();
          onViewChange(AMAN_MINIMUM_HORIZON_MINUTES, event.key === "Home" ? SCROLL_RANGE_MINUTES : 0);
        }
      }}
      role="scrollbar"
      tabIndex={0}
    >
      <div
        className="absolute left-0 right-0 z-10 rounded-sm border border-white bg-[#86a4af]"
        data-testid="aman-timeline-scroll-thumb"
        style={{height: `${thumbHeight}px`, top: `${railTopPad + thumbTop}px`}}
      />
    </div>
  );
}

export interface AMANBoardViewProps {
  state: AMANState | null;
  presentationStatus: AMANPresentationStatus;
  error: string | null;
  connectionState: AMANConnectionState;
  selectedCallsign: string | null;
  onSelectFlight?: (callsign: string) => void;
  onOpenControls?: () => void;
  /** The anchor is the clicked target, so the flight menu can open over it. */
  onOpenFlightActions?: (callsign: string, anchor?: HTMLElement) => void;
  onOpenFlightDetails?: (callsign: string) => void;
  focusedRunwayGroupID?: string | null;
  accView?: AMANView;
  onACCViewChange?: (view: AMANView) => void;
  onViewStateChange?: (viewState: {view: "holds" | "runway" | "acc"; starFamilies: string[]}) => void;
}

export function AMANBoardView({
  state,
  presentationStatus,
  error,
  connectionState,
  selectedCallsign,
  onSelectFlight,
  onOpenFlightActions,
  focusedRunwayGroupID = null,
  accView: suppliedACCView,
  onACCViewChange,
  onViewStateChange,
}: AMANBoardViewProps) {
  const callsign = useWebSocketStore((value) => value.callsign);
  const controllers = useWebSocketStore((value) => value.controllers) ?? [];
  const storedACCView = useWebSocketStore((value) => value.amanSelectedView);
  const setStoredACCView = useWebSocketStore((value) => value.setAMANSelectedView);
  const readOnly = useWebSocketStore((value) => value.readOnly);
  const hasFMPAuthority = useWebSocketStore((value) => value.amanFMPAuthority);
  const forcedFMP = useWebSocketStore((value) => value.amanForcedFMP);
  const forceFMPPending = useWebSocketStore((value) => value.amanForceFMPCommandID !== null);
  const forceFMPRejection = useWebSocketStore((value) => value.amanForceFMPRejection);
  const setForcedFMP = useWebSocketStore((value) => value.setAMANForcedFMP);
  const pendingCommands = useWebSocketStore((value) => value.amanPendingCommands);
  const commandRejections = useWebSocketStore((value) => value.amanCommandRejections);
  const sendCommand = useWebSocketStore((value) => value.sendAMANCommand);
  const transitionAltitude = useWebSocketStore((value) => value.transitionAltitude);
  const preferredACCView = readAMANViewPreference();
  const accView = suppliedACCView ?? resolveAMANView(
    state,
    preferredACCView === null ? null : storedACCView,
    controllerAMANViews(state, [callsign]),
  );
  const setACCView = onACCViewChange ?? setStoredACCView;
  const lanes = useMemo(() => state ? buildAMANLanes(state) : [], [state]);
  const [view, setView] = useState<"holds" | "runway" | "acc">("holds");
  const [selectedStarFamilies, setSelectedStarFamilies] = useState<string[]>([]);
  useEffect(() => {
    onViewStateChange?.({view, starFamilies: selectedStarFamilies});
  }, [onViewStateChange, view, selectedStarFamilies]);
  const [selectedRunwayGroupID, setSelectedRunwayGroupID] = useState<string | null>(null);
  const [targetPreferences, setTargetPreferences] = useAMANAircraftTargetPreferences();
  const [targetPreferencesOpen, setTargetPreferencesOpen] = useState(false);
  const [gapRemoval, setGapRemoval] = useState<{gap: AMANRunwayGap; runway: string} | null>(null);
  const timelineScrollRef = useRef<HTMLDivElement>(null);
  const [timelineViewportHeight, setTimelineViewportHeight] = useState(0);
  const [railHeight, setRailHeight] = useState(0);
  const sectionRef = useRef<HTMLElement>(null);
  const selectedFlightRunwayGroupID = state?.flights.find((flight) => flight.callsign === selectedCallsign)?.runway_group_id ?? null;
  const defaultRunwayGroupID = state?.active_runway_groups?.[0]
    ?? state?.runway_groups.find((group) => group.selected)?.id
    ?? null;
  const activeRunwayLane = lanes.find((lane) => lane.id === selectedRunwayGroupID)
    ?? lanes.find((lane) => lane.id === focusedRunwayGroupID)
    ?? lanes.find((lane) => lane.id === selectedFlightRunwayGroupID)
    ?? lanes.find((lane) => lane.id === defaultRunwayGroupID)
    ?? lanes[0]
    ?? null;
  const axis = useAMANTimelineAxis(state?.generated_at ?? new Date(0).toISOString());
  const range = axis.range;
  const timelineHeight = timelineViewportHeight > 0 ? timelineViewportHeight : MINIMUM_TIMELINE_HEIGHT_PIXELS;
  const nowPosition = AMANAxisTopPercent(axis.clockMs, range);
  const sourceStatus = (state?.technical_health.observation_source ?? state?.technical_health.vatsim)?.status;
  const axisStatus = connectionState === "disconnected" ? "disconnected" : sourceStatus === "degraded" || sourceStatus === "unavailable" ? "stale" : "fresh";
  const gainLossAuthoritative = state?.authoritative === true && state.effective_mode === "authoritative";
  const gapRemovalDisabled = getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority}) !== null
    || Object.values(pendingCommands).some((command) => command.type === "aman.create_gap" || command.type === "aman.remove_gap");
  const targetFields = (flight: AMANFlight): AMANAircraftTargetField[] => [
    ...(flight.feeder_fix_eta ? [{id: "feeder-fix-eta", label: "Feeder-fix ETA", value: formatAMANTime(flight.feeder_fix_eta)}] as const : []),
    ...(flight.expected_holding_seconds === null ? [] : [{id: "total-delay", label: "Total delay", value: `D${String(Math.ceil(flight.expected_holding_seconds / 60)).padStart(2, "0")}`}] as const),
    ...(flight.slot?.runway_group_id ?? flight.runway_group_id ? [{id: "runway", label: "Runway", value: flight.slot?.runway_group_id ?? flight.runway_group_id}] as const : []),
    ...(flight.wake_category ? [{id: "wtc", label: "WTC", value: flight.wake_category}] as const : []),
    ...(flight.aircraft_type ? [{id: "aircraft-type", label: "Aircraft type", value: flight.aircraft_type}] as const : []),
    ...(flight.feeder_fix ? [{id: "feeder-fix", label: "Feeder fix", value: flight.feeder_fix}] as const : []),
  ];
  const isDimmedFamily = (family: string | null | undefined) => selectedStarFamilies.length > 0 && (family == null || !selectedStarFamilies.includes(family));
  const renderTarget = (flight: AMANFlight, compact = false) => (
    <AMANAircraftTarget
      compact={compact}
      flight={flight}
      guidance={{
        authoritative: gainLossAuthoritative,
        connected: connectionState === "connected",
      }}
      leadingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
      onSelect={(target) => {
        onSelectFlight?.(flight.callsign);
        onOpenFlightActions?.(flight.callsign, target);
      }}
      selected={flight.callsign === selectedCallsign}
      dimmed={isDimmedFamily(flight.star_family ?? flight.feeder ?? flight.star)}
      trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
      transitionAltitude={transitionAltitude}
    />
  );
  const renderFMPTarget = (flight: AMANFlight) => renderTarget(flight, true);
  const renderRWYTarget = (flight: AMANFlight) => (
    <div className="flex min-h-7 items-stretch">
      {renderTarget(flight)}
      {flight.star_family && <span className="flex items-center border border-l-0 border-[#b8b8b8] bg-[#3f3f3f] px-1.5 font-mono text-[11px] text-[#a9bdc5]" style={{color: isDimmedFamily(flight.star_family ?? flight.feeder ?? flight.star) ? "#747474" : starFamilyColor(flight.star_family)}}>{flight.star_family}</span>}
    </div>
  );
  const renderACCTarget = (flight: AMANFlight) => {
    const emphasized = accView === AMAN_ALL_VIEW || flight.star_family === accView;
    return (
      <AMANAircraftTarget
        compact
        delayFirst
        emphasis={emphasized ? "primary" : "subdued"}
        flight={flight}
        guidance={{authoritative: gainLossAuthoritative, connected: connectionState === "connected"}}
        leadingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
        onSelect={(target) => {
          onSelectFlight?.(flight.callsign);
          onOpenFlightActions?.(flight.callsign, target);
        }}
        selected={flight.callsign === selectedCallsign}
        trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
        transitionAltitude={transitionAltitude}
      />
    );
  };
  // Mirrors the scroll rail. Wheel down scrolls back toward now and, once
  // there, compresses the timeline (more minutes in view). Wheel up first
  // expands a compressed timeline, then scrolls later in time. A scroll-back
  // that reaches now holds there until the wheel pauses, so a flick or
  // trackpad momentum cannot run on into compression unintentionally.
  const wheelRef = useRef({lastAt: Number.NEGATIVE_INFINITY, holdingAtNow: false});
  const handleTimelineWheel = (deltaY: number) => {
    if (deltaY === 0) return;
    const wheel = wheelRef.current;
    const at = Date.now();
    const paused = at - wheel.lastAt >= WHEEL_COMPRESS_PAUSE_MS;
    wheel.lastAt = at;
    if (deltaY < 0) {
      wheel.holdingAtNow = false;
      if (axis.horizonMinutes > AMAN_MINIMUM_HORIZON_MINUTES) axis.setView(Math.max(AMAN_MINIMUM_HORIZON_MINUTES, axis.horizonMinutes - WHEEL_STEP_MINUTES), 0);
      else axis.setView(AMAN_MINIMUM_HORIZON_MINUTES, axis.offsetMinutes + WHEEL_STEP_MINUTES);
    } else if (axis.offsetMinutes > 0) {
      wheel.holdingAtNow = true;
      axis.setView(AMAN_MINIMUM_HORIZON_MINUTES, Math.max(0, axis.offsetMinutes - WHEEL_STEP_MINUTES));
    } else if (!wheel.holdingAtNow || paused) {
      wheel.holdingAtNow = false;
      axis.setView(axis.horizonMinutes + WHEEL_STEP_MINUTES, 0);
    }
  };
  const syncTimelineViewport = () => {
    const timeline = timelineScrollRef.current;
    if (timeline !== null) setTimelineViewportHeight(timeline.clientHeight);
    if (sectionRef.current !== null) setRailHeight(sectionRef.current.clientHeight);
  };
  const hasTimelineState = state !== null;
  useEffect(() => {
    syncTimelineViewport();
    const timeline = timelineScrollRef.current;
    const section = sectionRef.current;
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(syncTimelineViewport);
    if (timeline !== null) observer?.observe(timeline);
    if (section !== null) observer?.observe(section);
    window.addEventListener("resize", syncTimelineViewport);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", syncTimelineViewport);
    };
  }, [hasTimelineState]);

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
    <section aria-label="AMAN presentation" className={`relative grid h-full min-h-[640px] w-full ${view === "acc" && selectedStarFamilies.length > 0 && selectedStarFamilies.length <= 2 ? "" : "max-w-[1440px]"} grid-rows-[clamp(7.425rem,12%,8.55rem)_minmax(0,1fr)] overflow-hidden bg-[#505052] text-white shadow-2xl`} ref={sectionRef}>
      <AMANSettingsHeader
        connectionState={connectionState}
        commandRejections={commandRejections}
        hasFMPAuthority={hasFMPAuthority}
        forcedFMP={forcedFMP}
        forceFMPPending={forceFMPPending}
        forceFMPRejection={forceFMPRejection}
        onForceFMP={setForcedFMP}
        onCommand={sendCommand}
        onRunwayGroupViewChange={setSelectedRunwayGroupID}
        onStarFamiliesChange={setSelectedStarFamilies}
        onViewChange={(next) => {
          if (next === "acc") {
            setACCView(AMAN_ALL_VIEW);
            setSelectedStarFamilies(defaultACCFamilies(callsign, controllers, availableAMANViews(state)));
          }
          setView(next);
        }}
        onACCViewChange={(next) => setACCView(next as AMANView)}
        presentationStatus={presentationStatus}
        pendingCommands={pendingCommands}
        readOnly={readOnly}
        runwayGroupOptions={state.runway_groups.map((group) => ({id: group.id, label: `${group.id} : ${state.flights.filter((flight) => flight.runway_group_id === group.id).length}`}))}
        secondaryAccessory={<>
          {state.technical_health.blocked_reasons.length > 0 && <span className="self-center text-xs text-red-900">{state.technical_health.blocked_reasons.join(", ")}</span>}
        </>}
        accViewOptions={[AMAN_ALL_VIEW, ...availableAMANViews(state)]}
        selectedACCView={accView}
        selectedStarFamilies={selectedStarFamilies}
        selectedRunwayGroupID={activeRunwayLane?.id ?? null}
        state={state}
        view={view}
      />
      <div className="relative min-h-0 flex-1">
        <div
          className="h-full overflow-hidden pl-[13.2px]"
          onWheel={(event) => handleTimelineWheel(event.deltaY)}
          ref={timelineScrollRef}
        >
          <div className="relative flex min-w-full" data-testid="aman-timeline-grid" id="aman-timeline-grid" style={{height: `${timelineHeight}px`}}>
            {view === "runway" ? (
              <RWYPairedTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                gapRemovalDisabled={gapRemovalDisabled}
                onOpenTargetInformation={() => setTargetPreferencesOpen(true)}
                onRemoveGap={(gap, runway) => setGapRemoval({gap, runway})}
                range={range}
                renderTarget={renderRWYTarget}
                state={state}
                status={axisStatus}
              />
            ) : view === "acc" && accView !== AMAN_ALL_VIEW ? (
              <ACCTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                flights={state.flights}
                label={accView}
                gapRemovalDisabled={gapRemovalDisabled}
                onOpenTargetInformation={() => setTargetPreferencesOpen(true)}
                onRemoveGap={(gap, runway) => setGapRemoval({gap, runway})}
                runwayGroups={state.runway_groups}
                range={range}
                renderTarget={renderACCTarget}
                status={axisStatus}
              />
            ) : state.timeline_configuration !== undefined ? (
              <FMPPairedTimeline
                clockMs={axis.clockMs}
                currentPosition={nowPosition}
                flights={state.flights}
                gaps={activeRunwayLane?.gaps ?? []}
                closures={activeRunwayLane?.closures ?? []}
                capacityReservations={activeRunwayLane?.capacityReservations ?? []}
                mappings={state.timeline_configuration.mappings}
                gapRemovalDisabled={gapRemovalDisabled}
                onRemoveGap={(gap, runway) => setGapRemoval({gap, runway})}
                range={range}
                renderTarget={renderFMPTarget}
                runway={activeRunwayLane?.id ?? "runway"}
                status={axisStatus}
              />
            ) : (
              <section aria-label="AMAN timeline configuration unavailable" className="grid h-full min-w-[37.5rem] flex-1 place-content-center gap-2 border border-amber-400/60 bg-[#3f3f3f] p-8 text-center">
                <h2 className="font-display text-lg font-bold text-amber-100">Timeline configuration unavailable</h2>
                <p className="max-w-md text-sm text-slate-200">Waiting for a versioned terminal-layout projection from AMAN.</p>
              </section>
            )}
          </div>
        </div>
        {(view === "holds" || (view === "acc" && accView === AMAN_ALL_VIEW)) && state.timeline_configuration !== undefined && <FMPPairedTimelineFooter
          clockMs={axis.clockMs}
          mappings={state.timeline_configuration.mappings}
          selectedFamilies={selectedStarFamilies}
          onOpenTargetInformation={() => setTargetPreferencesOpen(true)}
          range={range}
        />}
      </div>
      <TimelineScrollRail
        horizonMinutes={axis.horizonMinutes}
        offsetMinutes={axis.offsetMinutes}
        onViewChange={axis.setView}
        viewportHeight={Math.max(0, railHeight - 4)}
      />
      <Dialog onOpenChange={setTargetPreferencesOpen} open={targetPreferencesOpen}>
        <DialogContent className="w-[min(493px,calc(100vw-2rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-0 text-white [&>button]:hidden">
          <DialogTitle className="border-b-2 border-[#dcdcdc] px-5 py-3 text-center font-display text-lg font-bold uppercase">Target Information</DialogTitle>
          <AMANAircraftTargetPreferenceControls onChange={setTargetPreferences} preferences={targetPreferences} />
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={(open) => !open && setGapRemoval(null)} open={gapRemoval !== null}>
        <DialogContent className="w-[min(22rem,calc(100vw-1rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className="border-b-2 border-[#dcdcdc] px-4 py-3 text-center text-base font-bold uppercase">Remove GAP</DialogTitle>
          <p className="px-4 py-4 text-center text-sm">Remove {gapRemoval?.gap.label} from {gapRemoval?.runway}?</p>
          <div className="grid grid-cols-2 border-t-2 border-[#dcdcdc]">
            <button className="border-r border-[#dcdcdc] bg-[#6b7f9f] py-2 font-bold hover:bg-[#a3d5e8] disabled:opacity-40" disabled={gapRemovalDisabled} onClick={() => {
              if (gapRemoval === null) return;
              sendCommand({type: "aman.remove_gap", runway_group_id: gapRemoval.runway, gap_id: gapRemoval.gap.id});
              setGapRemoval(null);
            }} type="button">YES</button>
            <button className="bg-[#6b7f9f] py-2 font-bold hover:bg-[#a3d5e8]" onClick={() => setGapRemoval(null)} type="button">NO</button>
          </div>
        </DialogContent>
      </Dialog>
    </section>
  );
}
