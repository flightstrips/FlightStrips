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
import {cn} from "@/lib/utils";
import {useWebSocketStore} from "@/store/store-hooks";
import {AMANAircraftTarget, type AMANAircraftTargetField} from "./AMANAircraftTarget";
import {AMANAircraftTargetPreferenceControls} from "./AMANAircraftTargetPreferences";
import {AMANSettingsHeader} from "./AMANSettingsHeader";
import {fieldsForAMANAircraftTargetSide, useAMANAircraftTargetPreferences} from "./amanAircraftTargetPreferenceModel";
import {ACCTimeline} from "./ACCTimeline";
import {FMPPairedTimeline, FMPPairedTimelineFooter} from "./FMPPairedTimeline";
import {RWYPairedTimeline} from "./RWYPairedTimeline";
import {AMANAxisTopPercent, formatAMANAxisLabel, useAMANTimelineAxis} from "./AMANTimelineAxis";
import {buildAMANLanes, formatAMANTime} from "./presentation";

const TIMELINE_PIXELS_PER_MINUTE = 32;
const MINIMUM_TIMELINE_HEIGHT_PIXELS = 960;
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
      className="absolute bottom-0 left-0 top-0 z-40 w-6 border border-[#dcdcdc] bg-[#242424] touch-none"
      onPointerDown={(event) => {
        event.currentTarget.setPointerCapture(event.pointerId);
        setScrollFromPointer(event.clientY, event.currentTarget);
      }}
      onPointerMove={(event) => {
        if (event.currentTarget.hasPointerCapture(event.pointerId)) setScrollFromPointer(event.clientY, event.currentTarget);
      }}
      onKeyDown={(event) => {
        const lineStep = 64;
        const pageStep = Math.max(lineStep, viewportHeight * 0.8);
        let next: number | null = null;
        if (event.key === "ArrowUp") next = scrollTop - lineStep;
        else if (event.key === "ArrowDown") next = scrollTop + lineStep;
        else if (event.key === "PageUp") next = scrollTop - pageStep;
        else if (event.key === "PageDown") next = scrollTop + pageStep;
        else if (event.key === "Home") next = 0;
        else if (event.key === "End") next = maximumScroll;
        if (next === null) return;
        event.preventDefault();
        onScrollTo(Math.max(0, Math.min(maximumScroll, next)));
      }}
      role="scrollbar"
      tabIndex={0}
    >
      <div className="absolute inset-x-2 bottom-8 top-1 border-x border-[#dadada]">
        {tickPositions.map((position) => (
          <span
            className={cn("absolute left-1 right-1 h-px", currentPosition !== null && position < currentPosition ? "bg-lime-300" : "bg-[#e2e2e2]")}
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

export interface AMANBoardViewProps {
  state: AMANState | null;
  presentationStatus: AMANPresentationStatus;
  error: string | null;
  connectionState: AMANConnectionState;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
  onOpenControls?: () => void;
  onOpenFlightActions?: (flightID: string) => void;
  onOpenFlightDetails?: (flightID: string) => void;
  focusedRunwayGroupID?: string | null;
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
  onOpenFlightActions,
  onOpenFlightDetails,
  focusedRunwayGroupID = null,
  accView: suppliedACCView,
  onACCViewChange,
}: AMANBoardViewProps) {
  const position = useWebSocketStore((value) => value.position);
  const storedACCView = useWebSocketStore((value) => value.amanSelectedView);
  const setStoredACCView = useWebSocketStore((value) => value.setAMANSelectedView);
  const readOnly = useWebSocketStore((value) => value.readOnly);
  const hasFMPAuthority = useWebSocketStore((value) => value.amanFMPAuthority);
  const pendingCommands = useWebSocketStore((value) => value.amanPendingCommands);
  const commandRejections = useWebSocketStore((value) => value.amanCommandRejections);
  const sendCommand = useWebSocketStore((value) => value.sendAMANCommand);
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
  const [gapRemoval, setGapRemoval] = useState<{gap: AMANRunwayGap; runway: string} | null>(null);
  const timelineScrollRef = useRef<HTMLDivElement>(null);
  const initializedTimelineScroll = useRef(false);
  const [timelineScroll, setTimelineScroll] = useState({top: 0, viewportHeight: 0, contentHeight: 0});
  const selectedFlightRunwayGroupID = state?.flights.find((flight) => flight.flight_id === selectedFlightID)?.runway_group_id ?? null;
  const activeRunwayLane = lanes.find((lane) => lane.id === selectedRunwayGroupID)
    ?? lanes.find((lane) => lane.id === focusedRunwayGroupID)
    ?? lanes.find((lane) => lane.id === selectedFlightRunwayGroupID)
    ?? lanes[0]
    ?? null;
  const axis = useAMANTimelineAxis(state?.generated_at ?? new Date(0).toISOString());
  const range = axis.range;
  const timelineHeight = useMemo(
    () => Math.max(MINIMUM_TIMELINE_HEIGHT_PIXELS, Math.ceil((range.endMs - range.startMs) / 60_000) * TIMELINE_PIXELS_PER_MINUTE),
    [range],
  );
  const nowPosition = AMANAxisTopPercent(axis.clockMs, range);
  const axisStatus = connectionState === "disconnected" ? "disconnected" : presentationStatus === "degraded" ? "stale" : "fresh";
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
  const renderTarget = (flight: AMANFlight, compact = false) => (
    <AMANAircraftTarget
      compact={compact}
      flight={flight}
      guidance={{
        authoritative: gainLossAuthoritative,
        connected: connectionState === "connected",
      }}
      leadingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "feeder")}
      onSelect={() => {
        onSelectFlight(flight.flight_id);
        onOpenFlightActions?.(flight.flight_id);
      }}
      selected={flight.flight_id === selectedFlightID}
      trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
    />
  );
  const renderFMPTarget = (flight: AMANFlight) => renderTarget(flight, true);
  const renderRWYTarget = (flight: AMANFlight) => (
    <div className="flex min-h-7 items-stretch">
      {renderTarget(flight)}
      {flight.star_family && <span className="flex items-center border border-l-0 border-[#b8b8b8] bg-[#3f3f3f] px-1.5 font-mono text-[11px] text-[#a9bdc5]">{flight.star_family}</span>}
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
        onSelect={() => {
          onSelectFlight(flight.flight_id);
          onOpenFlightActions?.(flight.flight_id);
        }}
        selected={flight.flight_id === selectedFlightID}
        trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
      />
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
    <section aria-label="AMAN presentation" className="grid h-full min-h-[640px] w-full max-w-[1440px] grid-rows-[clamp(7.5rem,13.333%,9rem)_minmax(0,1fr)] overflow-hidden bg-[#505052] text-white shadow-2xl">
      <AMANSettingsHeader
        connectionState={connectionState}
        commandRejections={commandRejections}
        hasFMPAuthority={hasFMPAuthority}
        onCommand={sendCommand}
        onRunwayGroupViewChange={setSelectedRunwayGroupID}
        onViewChange={setView}
        onACCViewChange={(next) => setACCView(next as AMANView)}
        presentationStatus={presentationStatus}
        pendingCommands={pendingCommands}
        readOnly={readOnly}
        runwayGroupOptions={state.runway_groups.map((group) => ({id: group.id, label: `${group.id} : ${state.flights.filter((flight) => flight.runway_group_id === group.id).length}`}))}
        secondaryAccessory={<>
          <span className="border-l border-black/40 pl-2 font-mono text-xs text-black">{formatAMANAxisLabel(range.startMs, range.startMs)}–{formatAMANAxisLabel(range.endMs, range.startMs)} UTC · {axis.horizonMinutes} min</span>
          <button className="aman-settings-button bg-lime-400 text-black" onClick={onOpenControls} type="button">FMP</button>
          <button className="aman-settings-button bg-[#4b5563] text-white disabled:opacity-50" disabled={selectedFlightID === null} onClick={() => selectedFlightID !== null && onOpenFlightDetails?.(selectedFlightID)} type="button">DETAIL</button>
          {state.technical_health.blocked_reasons.length > 0 && <span className="self-center text-xs text-red-900">{state.technical_health.blocked_reasons.join(", ")}</span>}
        </>}
        accViewOptions={[AMAN_ALL_VIEW, ...availableAMANViews(state)]}
        selectedACCView={accView}
        selectedRunwayGroupID={activeRunwayLane?.id ?? null}
        state={state}
        view={view}
      />
      <div className="relative min-h-0 flex-1">
        <div className="h-full overflow-auto pl-6 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" onScroll={syncTimelineScroll} ref={timelineScrollRef}>
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
            ) : view === "acc" ? (
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
        {view === "holds" && state.timeline_configuration !== undefined && <FMPPairedTimelineFooter
          clockMs={axis.clockMs}
          mappings={state.timeline_configuration.mappings}
          onOpenTargetInformation={() => setTargetPreferencesOpen(true)}
          range={range}
        />}
      </div>
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
