import {useEffect, useMemo, useState, type ReactNode} from "react";

import {getAMANHeaderReadModel, getAMANMutationBlockReason, type AMANCommandIntent, type AMANCommandRejection, type AMANConnectionState, type AMANMutationBlockReason, type AMANPendingCommand, type AMANPresentationStatus, type AMANState} from "@/api/aman";
import {Dialog, DialogClose, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {cn} from "@/lib/utils";

export type AMANSettingsView = "holds" | "runway" | "acc";

interface AMANSettingsHeaderProps {
  connectionState: AMANConnectionState;
  commandRejections?: Record<string, AMANCommandRejection>;
  hasFMPAuthority?: boolean;
  onCommand?: (intent: AMANCommandIntent) => void;
  onOpenTargetPreferences: () => void;
  onRunwayGroupViewChange: (runwayGroupID: string) => void;
  onViewChange: (view: AMANSettingsView) => void;
  presentationStatus: AMANPresentationStatus;
  pendingCommands?: Record<string, AMANPendingCommand>;
  readOnly?: boolean;
  runwayGroupOptions?: Array<{id: string; label: string}>;
  secondaryAccessory?: ReactNode;
  selectedRunwayGroupID: string | null;
  state: AMANState;
  view: AMANSettingsView;
}

const fieldClass = "min-w-0 border border-black bg-[#e4e4e4] px-2 py-1 text-black";
const mutationBlockLabels: Record<AMANMutationBlockReason, string> = {
  no_state: "waiting for AMAN state", disconnected: "disconnected", observer: "observer session",
  unauthorized: "FMP authority is required", not_authoritative: "AMAN is not authoritative", not_ready: "AMAN is technically degraded",
};

function HeaderField({label, value, className}: {label: string; value: ReactNode; className?: string}) {
  return <div className={cn(fieldClass, className)} title={typeof value === "string" ? `${label}: ${value}` : label}><span className="block truncate text-[0.58rem] font-bold uppercase leading-none text-[#555355]">{label}</span><span className="mt-1 block truncate font-display text-xs font-semibold leading-none">{value}</span></div>;
}

export function AMANSettingsHeader({connectionState, commandRejections = {}, hasFMPAuthority = false, onCommand = () => undefined, onOpenTargetPreferences, onRunwayGroupViewChange, onViewChange, pendingCommands = {}, presentationStatus, readOnly = true, runwayGroupOptions, secondaryAccessory, selectedRunwayGroupID, state, view}: AMANSettingsHeaderProps) {
  const header = useMemo(() => getAMANHeaderReadModel(state), [state]);
  const [clock, setClock] = useState(() => Date.now());
  const [runwayOpen, setRunwayOpen] = useState(false);
  const [rateOpen, setRateOpen] = useState(false);
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
        <HeaderField label="Current view" value={`FMP / ${view === "holds" ? "ALL" : view.toUpperCase()}`} />
        <HeaderField label="UTC" value={new Date(clock).toISOString().slice(11, 19)} />
      </div>
      <div className="flex min-h-0 items-stretch gap-1 overflow-hidden bg-[#888] p-1">
        <button aria-label="Open target information preferences" className="aman-settings-button bg-[#5174b8] text-white" onClick={onOpenTargetPreferences} type="button">MAESTRO</button>
        {(["holds", "runway", "acc"] as const).map((candidate) => {
          const selected = candidate === view;
          const label = candidate === "holds" ? "ALL" : candidate === "runway" ? "RWY" : "ACC";
          return <button aria-controls="aman-timeline-grid" aria-pressed={selected} className={cn("aman-settings-button", selected ? "bg-[#86a4af] text-white" : "bg-[#d6d6d6] text-black")} key={candidate} onClick={() => onViewChange(candidate)} type="button"><span aria-hidden="true">{selected ? "✓ " : ""}</span>{label}</button>;
        })}
        <div className="aman-settings-button bg-[#d6d6d6] text-black">DSEQ · {desequenced}</div>
        {secondaryAccessory}
        <div aria-live="polite" className={cn("ml-auto flex items-center px-2 font-mono text-[0.68rem] font-bold", stateCues.length > 0 ? "border border-amber-200 bg-[#3f3f3f] text-amber-100" : "text-white")}>{stateCues.join(" · ") || "✓ READY"}</div>
      </div>
      <Dialog onOpenChange={setRunwayOpen} open={runwayOpen}>
        <DialogContent className="bg-[#e4e4e4] text-black">
          <DialogTitle>Runways in use</DialogTitle>
          <p className="text-sm">Server-confirmed: <b>{header.active_runway_groups.map((group) => group.id).join(" + ") || "Unavailable"}</b></p>
          <fieldset className="grid gap-2"><legend className="font-semibold">Replacement active runway set</legend>{runwayChoices.map((group) => <label className="flex items-center gap-2" key={group.id}><input checked={runwayIDs.includes(group.id)} disabled={blocked || !!runwayPending} onChange={() => { setRunwayIDs((current) => current.includes(group.id) ? current.filter((id) => id !== group.id) : [...current, group.id]); onRunwayGroupViewChange(group.id); }} type="checkbox" /><span aria-hidden="true">{selectedRunwayGroupID === group.id ? "▣" : "□"}</span>{group.label}</label>)}</fieldset>
          {runwayIDs.length === 0 && <p role="alert" className="font-semibold text-red-800">Select at least one runway.</p>}
          {runwayPending && <p aria-live="polite" role="status">Pending replacement: {runwayPending.runway_group_ids?.join(" + ")} · waiting for server confirmation</p>}
          {runwayRejection && <p role="alert" className="font-semibold text-red-800">Rejected: {runwayRejection.message} ({runwayRejection.code}, server revision {runwayRejection.current_revision})</p>}
          {blockReason && <p role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          <div className="flex justify-end gap-2"><DialogClose className="border border-black px-3 py-1">Cancel</DialogClose><button className="border border-black bg-[#f3d02e] px-3 py-1 font-semibold disabled:opacity-50" disabled={blocked || !!runwayPending || runwayIDs.length === 0} onClick={() => onCommand({type: "aman.set_active_runway_groups", runway_group_ids: runwayIDs})} type="button">Replace active set</button></div>
        </DialogContent>
      </Dialog>
      <Dialog onOpenChange={setRateOpen} open={rateOpen}>
        <DialogContent className="bg-[#e4e4e4] text-black">
          <DialogTitle>Arrival rate</DialogTitle>
          <p className="text-sm">Server-confirmed rates: <b>{rates}</b></p>
          <label className="grid gap-1">Runway group<select aria-label="Rate runway group" className="border border-black bg-white p-2" disabled={blocked || !!ratePending} onChange={(event) => setRateGroupID(event.target.value)} value={rateGroupID}>{(state.runway_groups ?? []).map((group) => <option key={group.id} value={group.id}>{group.id}</option>)}</select></label>
          <label className="grid gap-1">Arrivals per hour<input className="border border-black bg-white p-2" min="1" onChange={(event) => setRate(event.target.value)} type="number" value={rate} /></label>
          <label className="grid gap-1">Effective at (optional; blank applies now)<input className="border border-black bg-white p-2" onChange={(event) => setRateEffectiveAt(event.target.value)} type="datetime-local" value={rateEffectiveAt} /></label>
          {!validRate && <p role="alert" className="font-semibold text-red-800">Enter a whole arrival rate of at least one and a valid effective time.</p>}
          {ratePending && <p aria-live="polite" role="status">Pending rate: {ratePending.runway_group_id} · {ratePending.arrivals_per_hour}/h · waiting for server confirmation</p>}
          {rateRejection && <p role="alert" className="font-semibold text-red-800">Rejected: {rateRejection.message} ({rateRejection.code}, server revision {rateRejection.current_revision})</p>}
          {blockReason && <p role="status">Read-only: {mutationBlockLabels[blockReason]}.</p>}
          <div className="flex justify-end gap-2"><DialogClose className="border border-black px-3 py-1">Cancel</DialogClose><button className="border border-black bg-[#f3d02e] px-3 py-1 font-semibold disabled:opacity-50" disabled={blocked || !!ratePending || !rateGroupID || !validRate} onClick={() => onCommand({type: "aman.set_rate", runway_group_id: rateGroupID, arrivals_per_hour: Number(rate), effective_at: rateEffectiveAt === "" ? state.generated_at : new Date(rateEffectiveAt).toISOString()})} type="button">Set arrival rate</button></div>
        </DialogContent>
      </Dialog>
    </header>
  );
}
