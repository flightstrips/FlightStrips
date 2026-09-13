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
  onCommand?: (intent: AMANCommandIntent) => void;
  onACCViewChange?: (view: string) => void;
  onRunwayGroupViewChange: (runwayGroupID: string) => void;
  onViewChange: (view: AMANSettingsView) => void;
  presentationStatus: AMANPresentationStatus;
  pendingCommands?: Record<string, AMANPendingCommand>;
  readOnly?: boolean;
  runwayGroupOptions?: Array<{id: string; label: string}>;
  secondaryAccessory?: ReactNode;
  selectedACCView?: string;
  selectedRunwayGroupID: string | null;
  state: AMANState;
  view: AMANSettingsView;
}

const fieldClass = "min-w-0 border border-black bg-[#e4e4e4] px-2 py-1 text-black";
const designDialogClass = "w-[min(26rem,calc(100vw-2rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden";
const designTitleClass = "border-b-2 border-[#dcdcdc] px-4 py-3 text-center text-lg font-bold uppercase";
const mutationBlockLabels: Record<AMANMutationBlockReason, string> = {
  no_state: "waiting for AMAN state", disconnected: "disconnected", observer: "observer session",
  unauthorized: "FMP authority is required", not_authoritative: "AMAN is not authoritative", not_ready: "AMAN is technically degraded",
};

function HeaderField({label, value, className}: {label: string; value: ReactNode; className?: string}) {
  return <div className={cn(fieldClass, className)} title={typeof value === "string" ? `${label}: ${value}` : label}><span className="block truncate text-[0.58rem] font-bold uppercase leading-none text-[#555355]">{label}</span><span className="mt-1 block truncate font-display text-xs font-semibold leading-none">{value}</span></div>;
}

export function AMANSettingsHeader({accViewOptions = [], connectionState, commandRejections = {}, hasFMPAuthority = false, onACCViewChange, onCommand = () => undefined, onRunwayGroupViewChange, onViewChange, pendingCommands = {}, presentationStatus, readOnly = true, runwayGroupOptions, secondaryAccessory, selectedACCView, selectedRunwayGroupID, state, view}: AMANSettingsHeaderProps) {
  const header = useMemo(() => getAMANHeaderReadModel(state), [state]);
  const [clock, setClock] = useState(() => Date.now());
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [rateOpen, setRateOpen] = useState(false);
  const [maestroOpen, setMaestroOpen] = useState(false);
  const [dseqOpen, setDseqOpen] = useState(false);
  const [runwayIDs, setRunwayIDs] = useState<string[]>([]);
  const [rateGroupID, setRateGroupID] = useState("");
  const [rate, setRate] = useState("30");
  const [rateEffectiveAt, setRateEffectiveAt] = useState("");
  useEffect(() => {
    const timer = window.setInterval(() => setClock(Date.now()), 1_000);
    return () => window.clearInterval(timer);
  }, []);

  const rates = header.active_runway_groups.map((group) => `${group.id}: ${group.active_rate_per_hour ?? "Unavailable"}/h`).join(" · ") || "Unavailable";
  const wind = header.wind === null ? "Unavailable" : `${header.wind.surface_direction_degrees}°/${header.wind.surface_speed_knots}kt · FL100 ${header.wind.direction_10000_degrees}°/${header.wind.speed_10000_knots}kt`;
  const traffic = header.traffic_summary.tma_above_1500_feet_count === null ? "Unavailable" : `TMA ${header.traffic_summary.tma_above_1500_feet_count} · Horizon ${header.traffic_summary.maestro_horizon_count}`;
  const stateCues = [
    connectionState === "disconnected" && "✕ DISCONNECTED",
    presentationStatus === "degraded" && "△ STALE",
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
  const validRate = Number.isInteger(Number(rate)) && Number(rate) > 0 && (rateEffectiveAt === "" || !Number.isNaN(new Date(rateEffectiveAt).valueOf()));
  const openRunways = () => {
    setRunwayIDs(header.active_runway_groups.map((group) => group.id));
    setRunwayOpen(true);
  };
  const openRate = () => {
    const group = header.active_runway_groups[0];
    setRateGroupID(group?.id ?? state.runway_groups?.[0]?.id ?? "");
    setRate(String(group?.active_rate_per_hour ?? 30));
    setRateEffectiveAt("");
    setRateOpen(true);
  };

  return (
    <header aria-label="MAESTRO settings" className="aman-settings-header">
      <span className="sr-only">{state.airport}</span><span className="sr-only">{presentationStatus}</span><span className="sr-only">{connectionState}</span><span className="sr-only">{header.effective_mode}</span>
      <div className="grid min-h-0 grid-cols-[minmax(4.5rem,0.7fr)_minmax(0,2fr)_minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,0.9fr)_minmax(0,0.8fr)] gap-1 overflow-hidden">
        <HeaderField className="bg-[#f3d02e]" label="RIU" value={<button aria-haspopup="dialog" className="w-full truncate text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-black" onClick={openRunways} type="button">{header.active_runway_groups.map((group) => group.id).join(" + ") || "Unavailable"}{runwayPending ? " · PENDING" : ""}</button>} />
        <HeaderField className="bg-[#f3d02e]" label="Runway rate" value={<button aria-haspopup="dialog" className="w-full truncate text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-black" onClick={openRate} type="button">{rates}{ratePending ? " · PENDING" : ""}</button>} />
        <HeaderField label="Wind surface / 10,000 ft" value={wind} />
        <HeaderField label={`Traffic · ${header.traffic_summary.status}`} value={traffic} />
        <HeaderField label="Current view" value={view === "acc" && selectedACCView !== undefined && onACCViewChange !== undefined
          ? <select aria-label="ACC STAR family emphasis" className="w-full appearance-none bg-transparent font-display font-semibold outline-none" onChange={(event) => onACCViewChange(event.target.value)} value={selectedACCView}>{accViewOptions.map((option) => <option className="text-black" key={option} value={option}>{option}</option>)}</select>
          : `FMP / ${view === "holds" ? "ALL" : view.toUpperCase()}`} />
        <HeaderField label="UTC" value={new Date(clock).toISOString().slice(11, 19)} />
      </div>
      <div className="flex min-h-0 items-stretch gap-1 overflow-hidden bg-[#888] p-1">
        <button aria-haspopup="dialog" className="aman-settings-button bg-[#5174b8] text-white" onClick={() => setMaestroOpen(true)} type="button">MAESTRO</button>
        {(["holds", "runway", "acc"] as const).map((candidate) => {
          const selected = candidate === view;
          const label = candidate === "holds" ? "ALL" : candidate === "runway" ? "RWY" : "ACC";
          return <button aria-controls="aman-timeline-grid" aria-pressed={selected} className={cn("aman-settings-button", selected ? "bg-[#86a4af] text-white" : "bg-[#d6d6d6] text-black")} key={candidate} onClick={() => onViewChange(candidate)} type="button"><span aria-hidden="true">{selected ? "✓" : ""}</span><span>{label}</span></button>;
        })}
        <button aria-haspopup="dialog" className="aman-settings-button bg-[#d6d6d6] text-black" onClick={() => setDseqOpen(true)} type="button">DSEQ · {desequenced}</button>
        {secondaryAccessory}
        <div aria-live="polite" className={cn("ml-auto flex items-center px-2 font-mono text-[0.68rem] font-bold", stateCues.length > 0 ? "border border-amber-200 bg-[#3f3f3f] text-amber-100" : "text-white")}>{stateCues.join(" · ") || "✓ READY"}</div>
      </div>
      <Dialog onOpenChange={setMaestroOpen} open={maestroOpen}>
        <DialogContent className="w-[min(207px,calc(100vw-2rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className={designTitleClass}>MAESTRO</DialogTitle>
          <div className="grid px-2 py-2 text-base font-bold">
            <button className="bg-[#86a4af] px-3 py-2 text-left hover:bg-[#a3d5e8] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white" onClick={() => { onViewChange("holds"); setMaestroOpen(false); }} type="button">MAESTRO</button>
            {maestroFixes.map((fix) => <button className="px-3 py-2 text-left hover:bg-[#a3d5e8] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white" key={fix} onClick={() => { onViewChange("holds"); setMaestroOpen(false); }} type="button">{fix}</button>)}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setDseqOpen} open={dseqOpen}>
        <DialogContent className="w-[min(359px,calc(100vw-2rem))] max-w-none gap-0 rounded-none border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className={designTitleClass}>Desequenced</DialogTitle>
          <div className="grid min-h-24 gap-1 px-4 py-3">
            {desequencedFlights.length === 0 && <p className="place-self-center text-sm">No desequenced aircraft</p>}
            {desequencedFlights.map((flight) => <div className="grid grid-cols-[1fr_auto_auto] items-center gap-2" key={flight.flight_id}>
              <b>{flight.callsign}</b>
              <button className="border border-[#dcdcdc] bg-[#6b7f9f] px-3 py-1 text-sm font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked} onClick={() => onCommand({type: "aman.resume_flight", flight_id: flight.flight_id})} type="button">RESUME</button>
              <button className="border border-[#dcdcdc] bg-[#6b7f9f] px-3 py-1 text-sm font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked} onClick={() => { if (window.confirm(`Remove ${flight.callsign} from AMAN? This cannot be resumed.`)) onCommand({type: "aman.remove_flight", flight_id: flight.flight_id}); }} type="button">REMOVE</button>
            </div>)}
          </div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setRunwayOpen} open={runwayOpen}>
        <DialogContent className="w-[min(207px,calc(100vw-2rem))] max-w-none gap-0 rounded-md border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
          <DialogTitle className={designTitleClass}>Runway in use</DialogTitle>
          <p className="sr-only">Server-confirmed: <b>{header.active_runway_groups.map((group) => group.id).join(" + ") || "Unavailable"}</b></p>
          <fieldset className="grid gap-1 px-3 py-3"><legend className="sr-only">Replacement active runway set</legend>{runwayChoices.map((group) => <label className="grid grid-cols-[1fr_1.75rem] items-center gap-3 text-lg font-bold" key={group.id}><span className={selectedRunwayGroupID === group.id ? "text-[#f3d02e]" : undefined}>{group.id.replace(/^ARRIVAL-/, "")}</span><input aria-label={group.label} checked={runwayIDs.includes(group.id)} className="h-6 w-6 appearance-none border-2 border-[#dcdcdc] bg-[#dcdcdc] checked:bg-[#202020]" disabled={blocked || !!runwayPending} onChange={() => { setRunwayIDs((current) => current.includes(group.id) ? current.filter((id) => id !== group.id) : [...current, group.id]); onRunwayGroupViewChange(group.id); }} type="checkbox" /></label>)}</fieldset>
          {runwayIDs.length === 0 && <p role="alert" className="px-3 pb-2 text-sm font-semibold text-amber-200">Select at least one runway.</p>}
          {runwayPending && <p aria-live="polite" className="px-3 pb-2 text-xs" role="status">Pending replacement: {runwayPending.runway_group_ids?.join(" + ")} · waiting for server confirmation</p>}
          {runwayRejection && <p role="alert" className="px-3 pb-2 text-xs font-semibold text-amber-200">Rejected: {runwayRejection.message} ({runwayRejection.code}, server revision {runwayRejection.current_revision})</p>}
          {blockReason && <p className="px-3 pb-2 text-xs" role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          <div className="grid grid-cols-2 border-t-2 border-[#dcdcdc]"><button aria-label="Replace active set" className="border-r border-[#dcdcdc] bg-[#6b7f9f] py-2 font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked || !!runwayPending || runwayIDs.length === 0} onClick={() => onCommand({type: "aman.set_active_runway_groups", runway_group_ids: runwayIDs})} type="button">YES</button><DialogClose aria-label="Cancel" className="bg-[#6b7f9f] py-2 text-center font-bold hover:bg-[#a3d5e8]">NO</DialogClose></div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setRateOpen} open={rateOpen}>
        <DialogContent className={`${designDialogClass} w-[min(555px,calc(100vw-2rem))]`}>
          <DialogTitle className={designTitleClass}>Arrival rate</DialogTitle>
          <div className="grid gap-3 px-5 py-4">
            <label className="grid grid-cols-[10rem_1fr] items-center gap-3 font-bold">New Runway Rate<input aria-label="Arrivals per hour" className="border border-[#dcdcdc] bg-[#d6d6d6] px-3 py-2 text-black" min="1" onChange={(event) => setRate(event.target.value)} type="number" value={rate} /></label>
            <label className="grid grid-cols-[10rem_1fr] items-center gap-3 font-bold">Runway<select aria-label="Rate runway group" className="border border-[#dcdcdc] bg-[#d6d6d6] px-3 py-2 text-black" disabled={blocked || !!ratePending} onChange={(event) => setRateGroupID(event.target.value)} value={rateGroupID}>{(state.runway_groups ?? []).map((group) => <option key={group.id} value={group.id}>{group.id}</option>)}</select></label>
            <label className="grid grid-cols-[10rem_1fr] items-center gap-3 text-sm font-bold">Effective at<input className="border border-[#dcdcdc] bg-[#d6d6d6] px-3 py-2 text-black" onChange={(event) => setRateEffectiveAt(event.target.value)} type="datetime-local" value={rateEffectiveAt} /></label>
            <p className="text-sm">Server-confirmed rates: <b>{rates}</b></p>
            {!validRate && <p role="alert" className="font-semibold text-amber-200">Enter a whole arrival rate of at least one and a valid effective time.</p>}
            {ratePending && <p aria-live="polite" role="status">Pending rate: {ratePending.runway_group_id} · {ratePending.arrivals_per_hour}/h · waiting for server confirmation</p>}
            {rateRejection && <p role="alert" className="font-semibold text-amber-200">Rejected: {rateRejection.message} ({rateRejection.code}, server revision {rateRejection.current_revision})</p>}
            {blockReason && <p role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          </div>
          <div className="grid grid-cols-2 border-t-2 border-[#dcdcdc]"><button aria-label="Set arrival rate" className="border-r border-[#dcdcdc] bg-[#6b7f9f] py-2 font-bold hover:bg-[#a3d5e8] disabled:opacity-50" disabled={blocked || !!ratePending || !rateGroupID || !validRate} onClick={() => onCommand({type: "aman.set_rate", runway_group_id: rateGroupID, arrivals_per_hour: Number(rate), effective_at: rateEffectiveAt === "" ? state.generated_at : new Date(rateEffectiveAt).toISOString()})} type="button">YES</button><DialogClose aria-label="Cancel" className="bg-[#6b7f9f] py-2 text-center font-bold hover:bg-[#a3d5e8]">NO</DialogClose></div>
        </DialogContent>
      </Dialog>
    </header>
  );
}
