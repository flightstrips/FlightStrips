import {useEffect, useMemo, useState, type ReactNode} from "react";

import {getAMANHeaderReadModel, getAMANMutationBlockReason, type AMANCommandIntent, type AMANCommandRejection, type AMANConnectionState, type AMANMutationBlockReason, type AMANPendingCommand, type AMANPresentationStatus, type AMANState} from "@/api/aman";
import {Dialog, DialogClose, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {cn} from "@/lib/utils";

export type AMANSettingsView = "holds" | "runway" | "acc";

interface AMANSettingsHeaderProps {
  accViewOptions?: string[];
  connectionState: AMANConnectionState;
  commandRejections?: Record<string, AMANCommandRejection>;
  hasFMPAuthority?: boolean;
  onCommand?: (intent: AMANCommandIntent) => string | null | void;
  onACCViewChange?: (view: string) => void;
  onStarFamiliesChange?: (families: string[]) => void;
  onRunwayGroupViewChange: (runwayGroupID: string) => void;
  onViewChange: (view: AMANSettingsView) => void;
  presentationStatus: AMANPresentationStatus;
  pendingCommands?: Record<string, AMANPendingCommand>;
  readOnly?: boolean;
  runwayGroupOptions?: Array<{id: string; label: string}>;
  secondaryAccessory?: ReactNode;
  selectedACCView?: string;
  selectedStarFamilies?: string[];
  selectedRunwayGroupID: string | null;
  state: AMANState;
  view: AMANSettingsView;
}

interface QueuedRateCommands {
  commands: AMANCommandIntent[];
  expectedRevision: number;
  commandID: string | null;
}

const DEFAULT_ARRIVAL_RATE = 40;
const MIN_ARRIVAL_RATE = 15;
const MAX_ARRIVAL_RATE = 60;
const MAX_APPLY_OFFSET_MINUTES = 30;
const rateSliderClass = "aman-rate-slider";
const rateGroupClass = "grid gap-2 rounded-sm border border-[#dcdcdc] px-3 pb-3 pt-1";
const fieldClass = "min-w-0 rounded-sm border border-black bg-[#e4e4e4] px-2 py-1 text-black";
const designTitleClass = "border-b-2 border-[#dcdcdc] px-4 py-3 text-center text-lg font-bold uppercase";
const mutationBlockLabels: Record<AMANMutationBlockReason, string> = {
  no_state: "waiting for AMAN state", disconnected: "disconnected", observer: "observer session",
  unauthorized: "FMP authority is required", not_authoritative: "AMAN is not authoritative", not_ready: "AMAN is technically degraded",
};

function HeaderField({label, value, className}: {label: string; value: ReactNode; className?: string}) {
  return <div className={cn(fieldClass, "flex flex-col", className)} title={typeof value === "string" ? `${label}: ${value}` : label}><span className="block truncate text-[0.58rem] font-bold uppercase leading-none text-[#555355]">{label}</span><span className="flex min-h-0 flex-1 items-center justify-center truncate text-center font-display text-xs leading-none">{value}</span></div>;
}

export function AMANSettingsHeader({accViewOptions = [], connectionState, commandRejections = {}, hasFMPAuthority = false, onACCViewChange, onCommand = () => undefined, onRunwayGroupViewChange, onStarFamiliesChange, onViewChange, pendingCommands = {}, presentationStatus, readOnly = true, runwayGroupOptions, secondaryAccessory, selectedACCView, selectedStarFamilies = [], selectedRunwayGroupID, state, view}: AMANSettingsHeaderProps) {
  const header = useMemo(() => getAMANHeaderReadModel(state), [state]);
  const [clock, setClock] = useState(() => Date.now());
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [rateOpen, setRateOpen] = useState(false);
  const [maestroOpen, setMaestroOpen] = useState(false);
  const [maestroAnchor, setMaestroAnchor] = useState<DOMRect | null>(null);
  const [dseqOpen, setDseqOpen] = useState(false);
  const [runwayIDs, setRunwayIDs] = useState<string[]>([]);
  const [rateValues, setRateValues] = useState<Record<string, number>>({});
  const [rateBaseMs, setRateBaseMs] = useState(() => Date.now());
  const [applyOffsetMinutes, setApplyOffsetMinutes] = useState(0);
  const [rateCommandQueue, setRateCommandQueue] = useState<QueuedRateCommands | null>(null);
  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, []);

  const activeIDs = header.active_runway_groups.map((group) => group.id.replace(/^ARRIVAL-/, ""));
  const displayRunways = activeIDs.some((id) => id === "04L" || id === "04R") ? ["04L", "04R", "12", "30"] : ["22L", "22R", "12", "30"];
  const rates = header.active_runway_groups.length > 0
    ? displayRunways.map((id, index) => {
      const rate = header.active_runway_groups.find((group) => group.id.replace(/^ARRIVAL-/, "") === id)?.active_rate_per_hour ?? DEFAULT_ARRIVAL_RATE;
      return (
        <span className={cn("flex items-center justify-center", index > 0 && "border-l border-[#dcdcdc]")} key={id}>{id}: {rate}</span>
      );
    })
    : "Unavailable";
  const wind = header.wind === null ? "Unavailable" : `${header.wind.surface_direction_degrees}°/${header.wind.surface_speed_knots}kt · FL100 ${header.wind.direction_10000_degrees}°/${header.wind.speed_10000_knots}kt`;
  const traffic = header.traffic_summary.tma_above_1500_feet_count === null ? "Unavailable" : (
    <span className="mr-auto grid grid-cols-[auto_auto_auto] items-center gap-y-0.5 pl-[2ch] text-sm font-bold leading-none">
      <span>TMA</span><span aria-hidden="true">:</span><span>{header.traffic_summary.tma_above_1500_feet_count}</span>
      <span>TOT</span><span aria-hidden="true">:</span><span>{header.traffic_summary.maestro_horizon_count}</span>
    </span>
  );
  const stateCues = [
    connectionState === "disconnected" && "✕ DISCONNECTED",
    (state.technical_health?.observation_source ?? state.technical_health?.vatsim)?.status === "degraded" && "△ STALE",
    header.availability === "unavailable" && "— UNAVAILABLE",
    header.availability === "degraded" && "△ DEGRADED",
    (!header.authoritative || header.effective_mode !== "authoritative") && "▣ READ ONLY",
  ].filter(Boolean) as string[];
  const desequenced = state.flights.filter((flight) => flight.sequence_disposition === "desequenced").length;
  const desequencedFlights = state.flights.filter((flight) => flight.sequence_disposition === "desequenced");
  const maestroFixes = [...new Set(state.timeline_configuration?.mappings.flatMap((mapping) => [mapping.left, mapping.right]).filter((value): value is string => value !== null) ?? [])];
  const runwayChoices = runwayGroupOptions ?? (state.runway_groups ?? []).map((group) => ({id: group.id, label: group.id}));
  const blockReason = getAMANMutationBlockReason({state, connection_state: connectionState, read_only: readOnly, has_fmp_authority: hasFMPAuthority});
  const blocked = blockReason !== null;
  const pending = Object.values(pendingCommands);
  const runwayPending = pending.find((command) => command.type === "aman.set_active_runway_groups");
  const ratePending = pending.find((command) => command.type === "aman.set_rate");
  const runwayRejection = Object.values(commandRejections).find((rejection) => rejection.command_type === "aman.set_active_runway_groups");
  const rateRejection = Object.values(commandRejections).find((rejection) => rejection.command_type === "aman.set_rate");
  const currentRateFor = (id: string) => {
    const value = header.active_runway_groups.find((group) => group.id.replace(/^ARRIVAL-/, "") === id)?.active_rate_per_hour ?? DEFAULT_ARRIVAL_RATE;
    return Math.min(MAX_ARRIVAL_RATE, Math.max(MIN_ARRIVAL_RATE, value));
  };
  const changedRates = displayRunways.filter((id) => (rateValues[id] ?? currentRateFor(id)) !== currentRateFor(id));
  const applyTimeLabel = new Date(rateBaseMs + applyOffsetMinutes * 60_000).toISOString().slice(11, 16);
  useEffect(() => {
    if (rateCommandQueue === null) return;
    if (blocked) {
      setRateCommandQueue(null);
      return;
    }

    const {commandID, expectedRevision, commands} = rateCommandQueue;
    if (commandID !== null) {
      if (commandRejections[commandID] !== undefined) {
        setRateCommandQueue(null);
        return;
      }
      if (pendingCommands[commandID] !== undefined) return;
    }
    if (state.revision <= expectedRevision) return;

    const [next, ...remaining] = commands;
    if (next === undefined) {
      setRateCommandQueue(null);
      return;
    }
    const nextCommandID = onCommand(next);
    if (nextCommandID === null) {
      setRateCommandQueue(null);
      return;
    }
    setRateCommandQueue(remaining.length === 0 ? null : {
      commands: remaining,
      expectedRevision: state.revision,
      commandID: nextCommandID ?? null,
    });
  }, [blocked, commandRejections, onCommand, pendingCommands, rateCommandQueue, state.revision]);
  const openRunways = () => {
    setRunwayIDs(header.active_runway_groups.map((group) => group.id));
    setRunwayOpen(true);
  };
  const openRate = () => {
    setRateValues(Object.fromEntries(displayRunways.map((id) => [id, currentRateFor(id)])));
    setRateBaseMs(Date.now());
    setApplyOffsetMinutes(0);
    setRateOpen(true);
  };
  const applyRates = () => {
    const effectiveAt = applyOffsetMinutes === 0 ? state.generated_at : new Date(rateBaseMs + applyOffsetMinutes * 60_000).toISOString();
    const commands = changedRates.map((id) => ({
      type: "aman.set_rate" as const,
      runway_group_id: `ARRIVAL-${id}`,
      arrivals_per_hour: rateValues[id],
      effective_at: effectiveAt,
    }));
    const [first, ...remaining] = commands;
    if (first === undefined) return;
    const commandID = onCommand(first);
    if (remaining.length > 0 && commandID !== null) {
      setRateCommandQueue({
        commands: remaining,
        expectedRevision: state.revision,
        commandID: commandID ?? null,
      });
    }
  };

  return (
    <header aria-label="MAESTRO settings" className="aman-settings-header">
      <span className="sr-only">{state.airport}</span><span className="sr-only">{presentationStatus}</span><span className="sr-only">{connectionState}</span><span className="sr-only">{header.effective_mode}</span>
      <div className="grid min-h-0 grid-cols-[minmax(4.5rem,0.7fr)_minmax(0,3.625fr)_minmax(0,0.975fr)_minmax(0,1fr)_minmax(0,0.9fr)_minmax(0,0.8fr)] gap-1 overflow-hidden">
        <HeaderField className="bg-[#f3d02e]" label="RIU" value={<button aria-haspopup="dialog" className="w-full truncate text-center text-xl focus-visible:outline focus-visible:outline-2 focus-visible:outline-black" onClick={openRunways} type="button">{header.active_runway_groups.map((group) => group.id.replace(/^ARRIVAL-/, "")).join(", ") || "Unavailable"}</button>} />
        <HeaderField className="relative bg-[#f3d02e] @container" label="Runway rate" value={<button aria-haspopup="dialog" className="absolute inset-0 grid grid-cols-4 pt-3 text-[length:min(5cqw,1.25rem)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-3px] focus-visible:outline-black" onClick={openRate} type="button">{rates}{(ratePending || rateCommandQueue !== null) && <span className="sr-only"> · PENDING</span>}</button>} />
        <HeaderField label="Wind surface / 10,000 ft" value={wind} />
        <HeaderField label={`Traffic · ${header.traffic_summary.status}`} value={traffic} />
        <HeaderField label="Current view" value={view === "acc" && selectedACCView !== undefined && onACCViewChange !== undefined
          ? <select aria-label="ACC STAR family emphasis" className="w-full appearance-none bg-transparent text-center font-display font-semibold outline-none" onChange={(event) => onACCViewChange(event.target.value)} value={selectedACCView}>{accViewOptions.map((option) => <option className="text-black" key={option} value={option}>{option}</option>)}</select>
          : `FMP / ${view === "holds" ? "ALL" : view.toUpperCase()}`} />
        <HeaderField label="UTC" value={new Date(clock).toISOString().slice(11, 19)} />
      </div>
      <div className="flex min-h-0 items-center gap-1 overflow-hidden bg-[#505052] p-1 shadow-[-4px_0_0_#505052,4px_0_0_#505052,0_4px_0_#505052,-4px_4px_0_#505052,4px_4px_0_#505052]">
        <button aria-expanded={maestroOpen} aria-haspopup="menu" className="aman-settings-button min-w-[5rem] px-3 text-[0.85em] bg-[#5174b8] text-white border-white" onClick={(event) => {
          setMaestroAnchor(event.currentTarget.getBoundingClientRect());
          setMaestroOpen((open) => !open);
        }} type="button">MAESTRO<span aria-hidden="true" className="border-x-[4px] border-x-transparent border-t-[5px] border-t-white" /></button>
        {(["holds", "runway", "acc"] as const).map((candidate) => {
          const selected = candidate === view;
          const label = candidate === "holds" ? "ALL" : candidate === "runway" ? "RWY" : "ACC";
          const shape = candidate === "holds"
            ? ""
            : candidate === "runway"
              ? "-ml-0.75 rounded-[1px]"
              : "-ml-0.75 rounded-l-none rounded-r-sm";
          return <button aria-controls="aman-timeline-grid" aria-pressed={selected} className={cn("aman-settings-button min-w-[3.25rem] px-1", shape, candidate === "holds" && "ml-1", selected ? "bg-[#86a4af] text-white border-white" : "bg-[#d6d6d6] text-black", candidate === "holds" && "rounded-r-none")} key={candidate} onClick={() => onViewChange(candidate)} type="button"><span aria-hidden="true">{selected ? "✓" : ""}</span><span>{label}</span></button>;
        })}
        <button aria-haspopup="dialog" className="aman-settings-button ml-1 bg-[#d6d6d6] text-black" onClick={() => setDseqOpen(true)} type="button">DSEQ · {desequenced}</button>
        {secondaryAccessory}
        <div aria-live="polite" className={cn("ml-auto flex items-center px-2 font-mono text-[0.68rem] font-bold", stateCues.length > 0 ? "border border-amber-200 bg-[#3f3f3f] text-amber-100" : "text-white")}>{stateCues.join(" · ") || "✓ READY"}</div>
      </div>
      {maestroOpen && <>
        <div aria-hidden="true" className="fixed inset-0 z-40" onClick={() => setMaestroOpen(false)} />
        <div aria-label="MAESTRO" className="fixed z-50 rounded-sm border border-white bg-[#5174b8] py-0.5 font-display text-[11px] font-bold text-white shadow-lg" onKeyDown={(event) => { if (event.key === "Escape") setMaestroOpen(false); }} role="menu" style={{left: maestroAnchor?.left ?? 0, top: maestroAnchor?.bottom ?? 0, minWidth: Math.max(maestroAnchor?.width ?? 0, 88)}}>
          <button aria-checked={selectedStarFamilies.length === 0} className={cn("flex w-full items-center gap-1.5 px-2 py-0.5 text-left hover:bg-[#a3d5e8] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white", selectedStarFamilies.length === 0 && "bg-[#86a4af]")} onClick={() => onStarFamiliesChange?.([])} role="menuitemcheckbox" type="button"><span aria-hidden="true" className="w-4">{selectedStarFamilies.length === 0 ? "✓" : ""}</span>MAESTRO</button>
          {maestroFixes.map((fix) => {
            const selected = selectedStarFamilies.includes(fix);
            return <button aria-checked={selected} className={cn("flex w-full items-center gap-1.5 px-2 py-0.5 text-left hover:bg-[#a3d5e8] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white", selected && "bg-[#86a4af]")} key={fix} onClick={() => onStarFamiliesChange?.(selected ? selectedStarFamilies.filter((family) => family !== fix) : [...selectedStarFamilies, fix])} role="menuitemcheckbox" type="button"><span aria-hidden="true" className="w-4">{selected ? "✓" : ""}</span>{fix}</button>;
          })}
        </div>
      </>}
      <Dialog onOpenChange={setDseqOpen} open={dseqOpen}>
        <DialogContent className="w-[min(359px,calc(100vw-2rem))] max-w-none gap-0 rounded-none border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className={designTitleClass}>Desequenced</DialogTitle>
          <div className="grid min-h-24 gap-1 px-4 py-3">
            {desequencedFlights.length === 0 && <p className="place-self-center text-sm">No desequenced aircraft</p>}
            {desequencedFlights.map((flight) => <div className="grid grid-cols-[1fr_auto_auto] items-center gap-2" key={flight.callsign}>
              <b>{flight.callsign}{flight.expedited ? " · Expedited" : ""}</b>
              <button className="border border-[#dcdcdc] bg-[#6b7f9f] px-3 py-1 text-sm font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked} onClick={() => onCommand({type: "aman.resume_flight", callsign: flight.callsign})} type="button">RESUME</button>
              <button className="border border-[#dcdcdc] bg-[#6b7f9f] px-3 py-1 text-sm font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked} onClick={() => { if (window.confirm(`Remove ${flight.callsign} from AMAN for this session? Use the Excluded tab to add it back.`)) onCommand({type: "aman.remove_flight", callsign: flight.callsign}); }} type="button">REMOVE</button>
            </div>)}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setRunwayOpen} open={runwayOpen}>
        <DialogContent className="w-[min(207px,calc(100vw-2rem))] max-w-none gap-0         rounded-md border-2 border-[#5174b8] bg-[#505052] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className={designTitleClass}>Runway in use</DialogTitle>
          <p className="sr-only">Server-confirmed: <b>{header.active_runway_groups.map((group) => group.id).join(" + ") || "Unavailable"}</b></p>
          <fieldset className="grid gap-1 px-3 py-3"><legend className="sr-only">Replacement active runway set</legend>{runwayChoices.map((group) => <label className="grid grid-cols-[1fr_1.75rem] items-center gap-3 text-lg font-bold" key={group.id}><span className={selectedRunwayGroupID === group.id ? "text-[#f3d02e]" : undefined}>{group.id.replace(/^ARRIVAL-/, "")}</span><input aria-label={group.label} checked={runwayIDs.includes(group.id)} className="h-6 w-6 appearance-none border-2 border-[#dcdcdc] bg-[#dcdcdc] checked:bg-[#202020]" disabled={blocked || !!runwayPending} onChange={() => { setRunwayIDs((current) => current.includes(group.id) ? current.filter((id) => id !== group.id) : [...current, group.id]); onRunwayGroupViewChange(group.id); }} type="checkbox" /></label>)}</fieldset>
          {runwayIDs.length === 0 && <p role="alert" className="px-3 pb-2 text-sm font-semibold text-amber-200">Select at least one runway.</p>}
          {runwayPending && <p aria-live="polite" className="px-3 pb-2 text-xs" role="status">Pending replacement: {runwayPending.runway_group_ids?.join(" + ")} · waiting for server confirmation</p>}
          {runwayRejection && <p role="alert" className="px-3 pb-2 text-xs font-semibold text-amber-200">Rejected: {runwayRejection.message} ({runwayRejection.code}, server revision {runwayRejection.current_revision})</p>}
          {blockReason && <p className="px-3 pb-2 text-xs" role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          <div className="grid grid-cols-2 gap-2 px-3 pb-3 pt-1"><button aria-label="Replace active set" className="h-7 rounded-sm bg-[#d6d6d6] font-bold text-black hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked || !!runwayPending || runwayIDs.length === 0} onClick={() => onCommand({type: "aman.set_active_runway_groups", runway_group_ids: runwayIDs})} type="button">YES</button><DialogClose aria-label="Cancel" className="h-7 rounded-sm bg-[#d6d6d6] text-center font-bold text-black hover:bg-[#a3d5e8]">NO</DialogClose></div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setRateOpen} open={rateOpen}>
        <DialogContent className="w-[min(22rem,calc(100vw-2rem))] max-w-none gap-0 rounded-md border-2 border-[#5174b8] bg-[#505052] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className="sr-only">Change rates</DialogTitle>
          <div className="grid gap-3 px-4 py-3">
            <fieldset className={rateGroupClass}>
              <legend className="px-1 text-sm font-bold">Rates</legend>
              {displayRunways.map((id) => {
                const value = rateValues[id] ?? currentRateFor(id);
                return (
                  <div className="grid grid-cols-[3rem_1fr] items-center gap-x-2" key={id}>
                    <span className="row-span-2 font-bold">{id}</span>
                    <span className="text-center text-sm">{value}/hr</span>
                    <input aria-label={`Rate ${id}`} aria-valuetext={`${value} per hour`} className={rateSliderClass} disabled={blocked || !!ratePending || rateCommandQueue !== null} max={MAX_ARRIVAL_RATE} min={MIN_ARRIVAL_RATE} onChange={(event) => setRateValues((current) => ({...current, [id]: Number(event.target.value)}))} step={1} type="range" value={value} />
                  </div>
                );
              })}
            </fieldset>
            <fieldset className={rateGroupClass}>
              <legend className="px-1 text-sm font-bold">Apply</legend>
              <div className="grid grid-cols-[3rem_1fr] items-center gap-x-2">
                <span className="row-span-2 text-sm font-bold leading-tight">Apply rates at</span>
                <span className="text-center text-sm">{applyTimeLabel}Z</span>
                <input aria-label="Apply rates at" aria-valuetext={`${applyTimeLabel} UTC`} className={rateSliderClass} disabled={blocked || !!ratePending || rateCommandQueue !== null} max={MAX_APPLY_OFFSET_MINUTES} min={0} onChange={(event) => setApplyOffsetMinutes(Number(event.target.value))} step={1} type="range" value={applyOffsetMinutes} />
              </div>
            </fieldset>
            {ratePending && <p aria-live="polite" className="text-sm" role="status">Pending rate: {ratePending.runway_group_id} · {ratePending.arrivals_per_hour}/h · waiting for server confirmation{rateCommandQueue !== null && ` · ${rateCommandQueue.commands.length} queued`}</p>}
            {rateRejection && <p role="alert" className="text-sm font-semibold text-amber-200">Rejected: {rateRejection.message} ({rateRejection.code}, server revision {rateRejection.current_revision})</p>}
            {blockReason && <p className="text-sm" role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          </div>
          <div className="flex justify-center gap-4 px-4 pb-3">
            <button aria-label="Set arrival rate" className="h-7 w-24 rounded-sm bg-[#d6d6d6] font-bold text-black hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked || !!ratePending || rateCommandQueue !== null || changedRates.length === 0} onClick={() => { applyRates(); setRateOpen(false); }} type="button">Ok</button>
            <DialogClose aria-label="Cancel" className="h-7 w-24 rounded-sm bg-[#d6d6d6] text-center font-bold text-black hover:bg-[#a3d5e8]">Cancel</DialogClose>
          </div>
        </DialogContent>
      </Dialog>
    </header>
  );
}
