import {createRoot} from "react-dom/client";

import type {AMANState} from "@/api/aman";
import {AMANSettingsHeader} from "@/components/aman/AMANSettingsHeader";
import {AMANWorkspaceShell} from "@/components/aman/AMANWorkspaceShell";
import "@/index.css";

const state = {
  airport: "EKCH", generated_at: "2026-09-12T10:00:00.000Z", authoritative: false, effective_mode: "read_only",
  flights: [{sequence_disposition: "desequenced"}],
  header: {
    active_runway_groups: [{id: "ARRIVAL-22", active_rate_per_hour: 30, rate_effective_at: null}],
    readiness: {status: "ready", ready: true, blocked_reasons: []},
    traffic_summary: {status: "ready", tma_above_1500_feet_count: 7, maestro_horizon_count: 11}, wind: null,
  },
} as unknown as AMANState;

createRoot(document.getElementById("root")!).render(
  <AMANWorkspaceShell
    maestro={<div className="grid h-full min-h-0 grid-rows-[clamp(7.5rem,13.333%,9rem)_minmax(0,1fr)]">
      <AMANSettingsHeader connectionState="connected" onOpenTargetPreferences={() => undefined} onRunwayGroupViewChange={() => undefined} onViewChange={() => undefined} presentationStatus="ready" selectedRunwayGroupID="ARRIVAL-22" state={state} view="holds" />
      <div data-testid="maestro-work-area" className="relative min-h-0 overflow-hidden border border-[#777] bg-[#555355]">
        <div className="absolute inset-x-0 bottom-0 h-1/6 border-t-2 border-[#9c0000] bg-[#3f3f3f]" />
        <div data-testid="timeline-reference" className="absolute bottom-8 left-1/2 top-8 w-12 -translate-x-1/2 border-2 border-[#dcdcdc]" />
        <div data-testid="target-reference" className="absolute left-[12%] top-1/2 h-7 w-24 bg-[#96d796]" />
      </div>
    </div>}
    tmt={<div className="grid h-full place-items-center border border-[#777] bg-[#555355] font-display text-2xl font-bold">TMT</div>}
  />,
);
