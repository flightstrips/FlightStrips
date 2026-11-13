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
const HORIZON_WHEEL_STEP_MINUTES = 2;

const STAR_FAMILY_COLORS: Record<string, string> = {
  ERNOV: "#95D692", TESPI: "#95D692", TUDLO: "#95D692",
  TIDVU: "#68A3C3", MONAK: "#68A3C3",
};
const starFamilyColor = (family: string) => STAR_FAMILY_COLORS[family.trim().toUpperCase()];
function TimelineScrollRail({
  horizonMinutes,
  viewportHeight,
  onHorizonChange,
}: {
  horizonMinutes: number;
  viewportHeight: number;
  onHorizonChange: (minutes: number) => void;
}) {
  const thumbHeight = 132;
  const railTopPad = 4;
  const travel = Math.max(0, viewportHeight - railTopPad - 4 - thumbHeight);
  const span = AMAN_MAXIMUM_HORIZON_MINUTES - AMAN_MINIMUM_HORIZON_MINUTES;
  // Up is a longer timeline, so the thumb rests at the top at the maximum horizon.
  const thumbTop = (AMAN_MAXIMUM_HORIZON_MINUTES - horizonMinutes) / span * travel;
  const setHorizonFromPointer = (clientY: number, rail: HTMLDivElement) => {
    const bounds = rail.getBoundingClientRect();
    const ratio = Math.max(0, Math.min(1, (clientY - bounds.top - railTopPad - thumbHeight / 2) / Math.max(1, travel)));
    onHorizonChange(AMAN_MAXIMUM_HORIZON_MINUTES - ratio * span);
  };

  return (
    <div
      aria-controls="aman-timeline-grid"
      aria-label="Timeline scroll position"
      aria-orientation="vertical"
      aria-valuemax={AMAN_MAXIMUM_HORIZON_MINUTES}
      aria-valuemin={AMAN_MINIMUM_HORIZON_MINUTES}
      aria-valuenow={horizonMinutes}
      aria-valuetext={`${horizonMinutes} minutes ahead`}
      className="absolute bottom-0 left-0 top-1 z-40 w-[13.2px] overflow-hidden rounded-md border border-white touch-none"
      onPointerDown={(event) => {
        event.currentTarget.setPointerCapture(event.pointerId);
        setHorizonFromPointer(event.clientY, event.currentTarget);
      }}
      onPointerMove={(event) => {
        if (event.currentTarget.hasPointerCapture(event.pointerId)) setHorizonFromPointer(event.clientY, event.currentTarget);
      }}
      onKeyDown={(event) => {
        let next: number | null = null;
        if (event.key === "ArrowUp") next = horizonMinutes + 1;
        else if (event.key === "ArrowDown") next = horizonMinutes - 1;
        else if (event.key === "PageUp") next = horizonMinutes + 5;
        else if (event.key === "PageDown") next = horizonMinutes - 5;
        else if (event.key === "Home") next = AMAN_MAXIMUM_HORIZON_MINUTES;
        else if (event.key === "End") next = AMAN_MINIMUM_HORIZON_MINUTES;
        if (next === null) return;
        event.preventDefault();
        onHorizonChange(next);
      }}
      role="scrollbar"
      tabIndex={0}
    >
      <div
        className="absolute left-0 right-0 z-10 rounded-sm border border-white bg-[#86a4af]"
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
  onSelectFlight: (callsign: string) => void;
  onOpenControls?: () => void;
  onOpenFlightActions?: (callsign: string) => void;
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
  const position = useWebSocketStore((value) => value.position);
  const controllers = useWebSocketStore((value) => value.controllers) ?? [];
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
      onSelect={() => {
        onSelectFlight(flight.callsign);
        onOpenFlightActions?.(flight.callsign);
      }}
      selected={flight.callsign === selectedCallsign}
      dimmed={isDimmedFamily(flight.star_family ?? flight.feeder ?? flight.star)}
      trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
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
        onSelect={() => {
          onSelectFlight(flight.callsign);
          onOpenFlightActions?.(flight.callsign);
        }}
        selected={flight.callsign === selectedCallsign}
        trailingFields={fieldsForAMANAircraftTargetSide(targetFields(flight), targetPreferences, "runway")}
      />
    );
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
        onCommand={sendCommand}
        onRunwayGroupViewChange={setSelectedRunwayGroupID}
        onStarFamiliesChange={setSelectedStarFamilies}
        onViewChange={(next) => {
          if (next === "acc") {
            setACCView(AMAN_ALL_VIEW);
            setSelectedStarFamilies(defaultACCFamilies(position, controllers, availableAMANViews(state)));
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
          onWheel={(event) => {
            if (event.deltaY !== 0) axis.setHorizonMinutes(axis.horizonMinutes + (event.deltaY < 0 ? HORIZON_WHEEL_STEP_MINUTES : -HORIZON_WHEEL_STEP_MINUTES));
          }}
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
        onHorizonChange={axis.setHorizonMinutes}
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
