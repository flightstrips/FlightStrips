import {useEffect, useMemo, useState, type ReactNode} from "react";

import {getAMANHeaderReadModel, type AMANConnectionState, type AMANPresentationStatus, type AMANState} from "@/api/aman";
import {cn} from "@/lib/utils";

export type AMANSettingsView = "holds" | "runway" | "acc";

interface AMANSettingsHeaderProps {
  connectionState: AMANConnectionState;
  onOpenTargetPreferences: () => void;
  onRunwayGroupViewChange: (runwayGroupID: string) => void;
  onViewChange: (view: AMANSettingsView) => void;
  presentationStatus: AMANPresentationStatus;
  runwayGroupOptions?: Array<{id: string; label: string}>;
  secondaryAccessory?: ReactNode;
  selectedRunwayGroupID: string | null;
  state: AMANState;
  view: AMANSettingsView;
}

const fieldClass = "min-w-0 border border-black bg-[#e4e4e4] px-2 py-1 text-black";

function HeaderField({label, value, className}: {label: string; value: ReactNode; className?: string}) {
  return <div className={cn(fieldClass, className)} title={typeof value === "string" ? `${label}: ${value}` : label}><span className="block truncate text-[0.58rem] font-bold uppercase leading-none text-[#555355]">{label}</span><span className="mt-1 block truncate font-display text-xs font-semibold leading-none">{value}</span></div>;
}

export function AMANSettingsHeader({connectionState, onOpenTargetPreferences, onRunwayGroupViewChange, onViewChange, presentationStatus, runwayGroupOptions, secondaryAccessory, selectedRunwayGroupID, state, view}: AMANSettingsHeaderProps) {
  const header = useMemo(() => getAMANHeaderReadModel(state), [state]);
  const [clock, setClock] = useState(() => Date.now());
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
  const runwayChoices = runwayGroupOptions ?? header.active_runway_groups.map((group) => ({id: group.id, label: group.id}));

  return (
    <header aria-label="MAESTRO settings" className="aman-settings-header">
      <span className="sr-only">{state.airport}</span><span className="sr-only">{presentationStatus}</span><span className="sr-only">{connectionState}</span><span className="sr-only">{header.effective_mode}</span>
      <div className="grid min-h-0 grid-cols-[minmax(4.5rem,0.7fr)_minmax(0,2fr)_minmax(0,1.3fr)_minmax(0,1fr)_minmax(0,0.9fr)_minmax(0,0.8fr)] gap-1 overflow-hidden">
        <HeaderField className="bg-[#f3d02e]" label="RIU" value={runwayChoices.length === 0 ? "Unavailable" : runwayChoices.map((group) => <button aria-pressed={selectedRunwayGroupID === group.id} className="mr-1 underline-offset-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-black" key={group.id} onClick={() => onRunwayGroupViewChange(group.id)} type="button">{group.label}</button>)} />
        <HeaderField className="bg-[#f3d02e]" label="Runway rate" value={rates} />
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
    </header>
  );
}
