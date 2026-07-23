import {useMemo, useState} from "react";

import type {
  AMANConnectionState,
  AMANFlight,
  AMANPresentationStatus,
  AMANState,
} from "@/api/aman";
import {cn} from "@/lib/utils";
import {
  buildAMANLanes,
  buildTimelineRange,
  formatAMANTime,
  formatGainLoss,
  layoutTimelineMarkers,
  operationalMarkerTimestamp,
  type AMANTimelineRange,
} from "./presentation";

const badgeBase = "inline-flex items-center rounded border px-1.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide";

function modeTone(mode: AMANState["effective_mode"]): string {
  switch (mode) {
    case "authoritative": return "border-emerald-400 bg-emerald-950 text-emerald-200";
    case "shadow": return "border-sky-400 bg-sky-950 text-sky-200";
    case "read_only": return "border-amber-400 bg-amber-950 text-amber-200";
    case "blocked": return "border-red-400 bg-red-950 text-red-200";
    case "disabled": return "border-slate-500 bg-slate-900 text-slate-300";
  }
}

function lifecycleTone(flight: AMANFlight): string {
  switch (flight.lifecycle_state) {
    case "unstable": return "border-amber-400 bg-amber-950 text-amber-100";
    case "stable": return "border-emerald-500 bg-emerald-950 text-emerald-100";
    case "go_around": return "border-red-400 bg-red-950 text-red-100";
    case "landed": return "border-slate-500 bg-slate-800 text-slate-200";
    case "removed": return "border-slate-700 bg-slate-950 text-slate-500 line-through";
    default: return "border-sky-600 bg-sky-950 text-sky-100";
  }
}

function dataTone(status: AMANFlight["data_status"]): string {
  if (status === "fresh") return "border-emerald-500 bg-emerald-950 text-emerald-200";
  if (status === "stale") return "border-amber-400 bg-amber-950 text-amber-100";
  return "border-red-500 bg-red-950 text-red-100";
}

function freezeTone(reason: AMANFlight["freeze_reason"]): string {
  if (reason === "superstable") return "border-cyan-300 bg-cyan-950 text-cyan-100";
  if (reason === "manual") return "border-fuchsia-300 bg-fuchsia-950 text-fuchsia-100";
  return "border-slate-600 bg-slate-900 text-slate-300";
}

function timelinePercent(timestamp: string | null, range: AMANTimelineRange): number | null {
  if (timestamp === null) return null;
  const value = Date.parse(timestamp);
  if (!Number.isFinite(value)) return null;
  return Math.max(0, Math.min(100, ((value - range.startMs) / (range.endMs - range.startMs)) * 100));
}

function TimelineLane({
  label,
  flights,
  range,
  selectedFlightID,
  onSelectFlight,
}: {
  label: string;
  flights: AMANFlight[];
  range: AMANTimelineRange;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
}) {
  const markers = layoutTimelineMarkers(flights, range);
  const topPercent = (timestamp: string) => 100 - (timelinePercent(timestamp, range) ?? 0);

  return (
    <section className="min-w-[245px] border-l border-black/70 bg-[#292929]" data-testid={`timeline-lane-${label}`}>
      <header className="flex min-h-16 items-center justify-between border-b border-black bg-[#3c3c3c] px-3 text-white">
        <div>
          <div className="font-display text-xl font-bold tracking-wide">{label}</div>
          <div className="text-[11px] uppercase tracking-[0.18em] text-slate-300">{flights.length} flights</div>
        </div>
        <span className="rounded border border-white/70 px-2 py-1 text-xs">{flights.length}</span>
      </header>
      <div
        className="relative h-[620px] overflow-hidden bg-[linear-gradient(to_bottom,rgba(255,255,255,0.18)_1px,transparent_1px)] bg-[size:100%_10%]"
      >
        <div className="absolute bottom-0 left-0 top-0 w-6 border-r border-slate-500 bg-[#1e1e1e]" aria-hidden="true" />
        <div className="absolute bottom-0 left-6 top-0 border-l border-dashed border-slate-500" aria-hidden="true" />
        {markers.map((marker) => {
          const selected = marker.flight.flight_id === selectedFlightID;
          const rawLeft = timelinePercent(marker.flight.raw_teta, range);
          return (
            <div key={marker.flight.flight_id}>
              {rawLeft !== null && marker.flight.raw_teta !== marker.timestamp && (
                <div
                  aria-label={`${marker.flight.callsign} raw TETA ${formatAMANTime(marker.flight.raw_teta)}`}
                  className="absolute left-5 h-3 w-3 -translate-x-1/2 rotate-45 border border-amber-200 bg-amber-400"
                  data-testid={`raw-marker-${marker.flight.flight_id}`}
                  style={{top: `${100 - rawLeft}%`}}
                  title={`Raw TETA ${formatAMANTime(marker.flight.raw_teta)} (informational only)`}
                />
              )}
              <button
                aria-label={`Select ${marker.flight.callsign} timeline marker`}
                className={cn(
                  "absolute left-9 right-2 rounded border-2 bg-[#86a4af] px-2 py-1.5 text-left text-xs text-white shadow-lg focus:outline-none focus:ring-2 focus:ring-white",
                  marker.flight.freeze_reason === "superstable" && "border-cyan-200",
                  marker.flight.freeze_reason === "manual" && "border-fuchsia-200",
                  marker.flight.freeze_reason === "none" && "border-black",
                  selected && "ring-2 ring-[#f3d02e]",
                )}
                data-marker-time={marker.timestamp}
                data-testid={`operational-marker-${marker.flight.flight_id}`}
                onClick={() => onSelectFlight(marker.flight.flight_id)}
                style={{top: `calc(${topPercent(marker.timestamp)}% + ${marker.track * 38}px)`}}
                type="button"
              >
                <span className="block truncate font-bold tracking-wide">{marker.flight.callsign}</span>
                <span className="block text-[11px] text-white/90">{formatAMANTime(marker.timestamp)} · {formatGainLoss(marker.flight.gain_loss_seconds)}</span>
              </button>
            </div>
          );
        })}
      </div>
      <footer className="border-t border-black bg-[#3c3c3c] px-3 py-2 text-center font-display text-sm font-semibold tracking-wide text-white">
        {flights.map((flight) => flight.holding_fix ?? flight.route_fact?.fix ?? "—").filter((fix, index, values) => values.indexOf(fix) === index).join(" · ") || "No route fix"}
      </footer>
    </section>
  );
}

function FlightStrip({flight, selected, onSelect}: {flight: AMANFlight; selected: boolean; onSelect: () => void}) {
  const direct = flight.route_fact === null
    ? "No accepted direct"
    : `${flight.route_fact.state === "active" ? "Accepted direct" : "Expired direct"} ${flight.route_fact.fix}`;
  const markerTime = operationalMarkerTimestamp(flight);

  return (
    <button
      aria-label={`${flight.callsign} AMAN strip`}
      className={cn(
        "grid w-full gap-3 rounded-lg border-2 bg-slate-900 p-3 text-left text-slate-100 shadow-md focus:outline-none focus:ring-2 focus:ring-white",
        selected ? "border-white" : "border-slate-600",
        flight.lifecycle_state === "go_around" && "border-red-400",
        flight.lifecycle_state === "landed" && "opacity-70",
      )}
      data-flight-id={flight.flight_id}
      onClick={onSelect}
      type="button"
    >
      <header className="flex flex-wrap items-start justify-between gap-2 border-b border-slate-700 pb-2">
        <div>
          <div className="flex items-baseline gap-2">
            <span className="text-xl font-black tracking-wide">{flight.callsign}</span>
            <span className="text-sm text-slate-400">#{flight.order ?? flight.slot?.sequence ?? "Unavailable"}</span>
          </div>
          <div className="text-xs text-slate-400">Flight ID {flight.flight_id}</div>
        </div>
        <div className="flex flex-wrap justify-end gap-1">
          <span className={cn(badgeBase, lifecycleTone(flight))}>{flight.lifecycle_state.replace("_", " ")}</span>
          <span className={cn(badgeBase, dataTone(flight.data_status))}>{flight.data_status}</span>
          <span className={cn(badgeBase, freezeTone(flight.freeze_reason))}>Freeze {flight.freeze_reason}</span>
        </div>
      </header>

      <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm md:grid-cols-4">
        <div><span className="text-slate-400">Aircraft / WTC</span><strong className="block">Unavailable / Unavailable</strong></div>
        <div><span className="text-slate-400">Runway / group</span><strong className="block">Unavailable / {flight.runway_group_id ?? "Unavailable"}</strong></div>
        <div><span className="text-slate-400">Sequence / order</span><strong className="block">{flight.slot?.sequence ?? "Unavailable"} / {flight.order ?? "Unavailable"}</strong></div>
        <div><span className="text-slate-400">Slot</span><strong className="block">{formatAMANTime(flight.slot?.time ?? null)}</strong></div>
        <div><span className="text-slate-400">Operational marker</span><strong className="block">{formatAMANTime(markerTime)}</strong></div>
        <div><span className="text-slate-400">Operational TETA</span><strong className="block">{formatAMANTime(flight.operational_teta)}</strong></div>
        <div className={flight.raw_teta !== flight.operational_teta ? "text-amber-200" : ""}><span className="text-slate-400">Raw TETA (drift only)</span><strong className="block">{formatAMANTime(flight.raw_teta)}</strong></div>
        <div><span className="text-slate-400">Gain / loss</span><strong className="block">{formatGainLoss(flight.gain_loss_seconds)}</strong></div>
        <div><span className="text-slate-400">Distance to go</span><strong className="block">{flight.distance_to_go_nm === null ? "Unavailable" : `${flight.distance_to_go_nm.toFixed(1)} NM`}</strong></div>
        <div><span className="text-slate-400">Feeder</span><strong className="block">{flight.feeder ?? "Unavailable"}</strong></div>
        <div><span className="text-slate-400">Published holding</span><strong className="block">Unavailable</strong></div>
        <div><span className="text-slate-400">Selected holding / ETA</span><strong className="block">{flight.holding_fix ?? "Unavailable"} / {formatAMANTime(flight.holding_fix_eta)}</strong></div>
        <div><span className="text-slate-400">Direct</span><strong className="block">{direct}</strong></div>
        <div><span className="text-slate-400">Confidence</span><strong className="block">{flight.confidence ?? "Unavailable"}</strong></div>
        <div><span className="text-slate-400">Input age</span><strong className="block">{flight.input_age_seconds === null ? "Unavailable" : `${flight.input_age_seconds}s`}</strong></div>
        <div><span className="text-slate-400">Geometry version / digest</span><strong className="block break-all">{flight.geometry_version ?? "Unavailable"} / {flight.geometry_digest ?? "Unavailable"}</strong></div>
      </div>

      <div className="grid gap-2 text-xs text-slate-300 md:grid-cols-2">
        <div className="rounded border border-slate-700 bg-slate-950 p-2">
          <b>Provenance:</b>{" "}
          {flight.provenance === null
            ? "Unavailable"
            : `model ${flight.provenance.model_version}; config ${flight.provenance.config_version}; performance ${flight.provenance.performance_profile_id ?? "Unavailable"}; weather ${flight.provenance.weather_source ?? "Unavailable"}; sources ${flight.provenance.sources.join(", ") || "none"}`}
        </div>
        <div className="rounded border border-slate-700 bg-slate-950 p-2">
          <b>Freeze:</b> {flight.freeze_reason}{flight.frozen_at ? ` since ${formatAMANTime(flight.frozen_at)}` : ""}. Slot group {flight.slot?.runway_group_id ?? "Unavailable"}, revision {flight.slot?.revision ?? "Unavailable"}, reason {flight.slot?.reason ?? "Unavailable"}.
        </div>
      </div>

      <div className="rounded border border-slate-700 bg-slate-950 p-2 text-xs text-slate-300">
        <b>Direct fact:</b>{" "}
        {flight.route_fact === null
          ? "Unavailable"
          : `${flight.route_fact.id}; ${flight.route_fact.fix}; ${flight.route_fact.state}; observed ${formatAMANTime(flight.route_fact.observed_at)}`}
      </div>

      {flight.eta_review && (
        <div className="rounded border border-amber-400 bg-amber-950 p-2 text-sm text-amber-100">
          <b>Discrepancy {flight.eta_review.status}:</b> initial/FPL {formatAMANTime(flight.eta_review.initial_baseline_teta)}, calculated {formatAMANTime(flight.eta_review.calculated_operational_teta)}, selected {formatAMANTime(flight.eta_review.selected_teta)}, manual {formatAMANTime(flight.eta_review.manual_teta)}. Created {formatAMANTime(flight.eta_review.created_at)}, deadline {formatAMANTime(flight.eta_review.deadline_at)}, resolved {formatAMANTime(flight.eta_review.resolved_at)}, actor {flight.eta_review.actor ?? "Unavailable"}, note {flight.eta_review.note ?? "Unavailable"}.
        </div>
      )}

      <div className="rounded border border-slate-700 bg-slate-950 p-2 text-sm">
        <b>Queue offers:</b>{" "}
        {flight.queue_offers.length === 0 ? "None" : flight.queue_offers.map((offer) => (
          <span className="mr-3 inline-block" key={`${offer.runway_group_id}-${offer.queue_position}-${offer.candidate_slot.revision}`}>
            #{offer.queue_position} {offer.runway_group_id} at {formatAMANTime(offer.candidate_slot.time)} ({offer.reason}; slot revision {offer.candidate_slot.revision}; airport revision {offer.airport_revision}; expires {formatAMANTime(offer.expires_at)})
          </span>
        ))}
      </div>
    </button>
  );
}

export interface AMANBoardViewProps {
  state: AMANState | null;
  presentationStatus: AMANPresentationStatus;
  error: string | null;
  connectionState: AMANConnectionState;
  selectedFlightID: string | null;
  onSelectFlight: (flightID: string) => void;
}

export function AMANBoardView({
  state,
  presentationStatus,
  error,
  connectionState,
  selectedFlightID,
  onSelectFlight,
}: AMANBoardViewProps) {
  const lanes = useMemo(() => state ? buildAMANLanes(state) : [], [state]);
  const range = useMemo(() => state ? buildTimelineRange(state.flights) : null, [state]);
  const [filter, setFilter] = useState<"all" | "runway" | "frozen">("all");
  const visibleLanes = useMemo(() => lanes.map((lane) => ({
    ...lane,
    flights: filter === "frozen"
      ? lane.flights.filter((flight) => flight.freeze_reason !== "none")
      : lane.flights,
  })).filter((lane) => filter !== "frozen" || lane.flights.length > 0), [filter, lanes]);

  if (state === null) {
    return (
      <section aria-label="AMAN presentation" className="grid min-h-72 place-items-center rounded-xl border border-slate-700 bg-slate-900 p-8 text-center text-slate-100">
        <div>
          <h1 className="text-2xl font-bold">AMAN timeline unavailable</h1>
          <p className="mt-2 text-slate-300">{error ? `State rejected: ${error}` : "Waiting for a complete AMAN state replacement."}</p>
          <span className={cn(badgeBase, connectionState === "connected" ? "border-emerald-500 text-emerald-200" : "border-red-500 text-red-200")}>{connectionState}</span>
        </div>
      </section>
    );
  }

  return (
    <section aria-label="AMAN presentation" className="grid gap-3 text-slate-100">
      <header className="border border-black bg-[#3c3c3c] shadow-lg">
        <div className="grid gap-px bg-black lg:grid-cols-[auto_minmax(0,1fr)_auto_auto_auto]">
          <div className="grid min-w-20 place-items-center bg-[#f3d02e] px-4 py-3 font-display text-2xl font-bold text-black">{state.airport}</div>
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1 bg-[#f3d02e] px-4 py-3 text-black">
            <h1 className="font-display text-2xl font-bold tracking-wide">ARRIVAL MANAGER</h1>
            <span className="text-sm font-semibold">{state.runway_groups.map((group) => `${group.id}: ${state.flights.filter((flight) => flight.runway_group_id === group.id).length}`).join("   ") || "No runway group"}</span>
          </div>
          <div className="bg-[#e4e4e4] px-3 py-2 text-right text-xs text-black">Health: <b>{state.technical_health.status}</b><br />TMA: {state.flights.length}</div>
          <div className="bg-[#e4e4e4] px-3 py-2 text-center text-xs text-black">{state.policy_version}<br />Revision {state.revision}</div>
          <div className="bg-[#e4e4e4] px-3 py-2 text-center font-mono text-sm text-[#555]">{new Date(state.generated_at).toISOString().slice(11, 19)}</div>
        </div>
        <div className="flex flex-wrap items-center gap-2 border-t border-black bg-[#3c3c3c] px-3 py-2">
          {(["all", "runway", "frozen"] as const).map((value) => (
            <button
              className={cn("rounded border border-black px-3 py-1 text-xs font-semibold uppercase tracking-wide", filter === value ? "bg-[#86a4af] text-white" : "bg-[#e4e4e4] text-black")}
              key={value}
              onClick={() => setFilter(value)}
              type="button"
            >
              {value === "all" ? "MAESTRO" : value === "runway" ? "RWY" : "FS"}
            </button>
          ))}
          <span className={cn(badgeBase, modeTone(state.effective_mode))}>{state.effective_mode.replace("_", " ")}</span>
          <span className={cn(badgeBase, connectionState === "connected" ? "border-emerald-500 bg-emerald-950 text-emerald-200" : "border-red-500 bg-red-950 text-red-100")}>{connectionState}</span>
          <span className={cn(badgeBase, presentationStatus === "ready" ? "border-emerald-500 bg-emerald-950 text-emerald-200" : "border-amber-400 bg-amber-950 text-amber-100")}>{presentationStatus}</span>
          <span className="ml-auto text-xs text-slate-300">Operational time · amber diamond = raw TETA only</span>
        </div>
        {state.technical_health.blocked_reasons.length > 0 && (
          <div role="alert" className="m-3 rounded border border-red-500 bg-red-950 p-2 text-sm text-red-100">
            Blocked: {state.technical_health.blocked_reasons.join(", ")}
          </div>
        )}
        <div className="grid gap-px border-t border-black bg-black sm:grid-cols-2 xl:grid-cols-3">
          {([
            ["VATSIM", state.technical_health.vatsim],
            ["Navigation", state.technical_health.navigation],
            ["Weather", state.technical_health.weather],
            ["Repository", state.technical_health.repository],
            ["Predictor", state.technical_health.predictor],
            ["Replay validation", state.technical_health.replay_validation],
          ] as const).map(([label, health]) => (
            <div className="bg-[#292929] p-2 text-xs" key={label}>
              <b>{label}: {health.status}</b>
              <span className="block text-slate-400">{health.reason ?? "No degradation reason"} · updated {formatAMANTime(health.updated_at)} · age {health.age_seconds === null ? "Unavailable" : `${health.age_seconds}s`}</span>
            </div>
          ))}
        </div>
      </header>

      <div className="overflow-x-auto border border-black bg-[#292929] shadow-lg">
        <div className="min-w-[880px]" data-testid="aman-timeline-grid">
          {range === null ? (
            <div className="p-6 text-center text-slate-300">No operational slot or TETA markers are available.</div>
          ) : (
            <div className="flex min-w-max">
              <div className="relative h-[684px] border-r border-black bg-[#1e1e1e] text-right text-xs text-white" aria-label="Operational time scale">
                {[0, 20, 40, 60, 80, 100].map((position) => (
                  <span className="absolute right-2 -translate-y-1/2" key={position} style={{top: `${position}%`}}>
                    {formatAMANTime(new Date(range.endMs - ((range.endMs - range.startMs) * position / 100)).toISOString())}
                  </span>
                ))}
              </div>
              {visibleLanes.map((lane) => (
                <TimelineLane flights={lane.flights} key={lane.id} label={lane.label} onSelectFlight={onSelectFlight} range={range} selectedFlightID={selectedFlightID} />
              ))}
            </div>
          )}
        </div>
      </div>

      {lanes.map((lane) => (
        <section aria-label={`${lane.label} AMAN strips`} className="grid gap-3" key={lane.id}>
          <h2 className="text-lg font-bold text-white">{lane.label} sequence</h2>
          <div className="grid gap-3 xl:grid-cols-2">
            {lane.flights.length === 0
              ? <div className="rounded border border-slate-700 bg-slate-900 p-4 text-slate-400">No flights in this runway group.</div>
              : lane.flights.map((flight) => (
                <FlightStrip
                  flight={flight}
                  key={flight.flight_id}
                  onSelect={() => onSelectFlight(flight.flight_id)}
                  selected={flight.flight_id === selectedFlightID}
                />
              ))}
          </div>
        </section>
      ))}
    </section>
  );
}
