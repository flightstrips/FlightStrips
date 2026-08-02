import {useEffect, useMemo, useState} from "react";
import type {LatLngTuple} from "leaflet";
import {CircleMarker, MapContainer, Polyline, TileLayer, Tooltip, useMap} from "react-leaflet";
import "leaflet/dist/leaflet.css";
import {operationalErrorReference, type OperationalErrorReference} from "./aman-replay-errors";

type ReplayPoint = {at: string; latitude: number; longitude: number; altitude_feet: number; groundspeed_knots: number};
type ReplayLeg = {id: string; from: [number, number]; to: [number, number]; distance_nm: number; duration_seconds: number; expected_groundspeed_knots: number};
type ReplayModelSegment = {phase_id: string; phase_name: string; pre_tod: boolean; start_altitude_feet: number; end_altitude_feet: number; altitude_feet: number; indicated_airspeed_knots?: number; no_wind_groundspeed_knots: number; groundspeed_knots: number; current_no_wind_groundspeed_knots?: number; current_groundspeed_knots?: number; tailwind_knots?: number; distance_nm: number; duration_seconds: number};
type ReplaySnapshot = {at: string; raw_teta: string; operational_teta: string; freeze_reason: string; star: string; route: ReplayLeg[]; model_segment?: ReplayModelSegment};
type ReplayEvent = {type: "threshold_crossing" | "go_around_detected" | "landing_proxy"; at: string; latitude: number; longitude: number; sequence?: number; reason?: string; supporting_observation_times?: string[]};
type ReplayFlight = {cid: string; callsign: string; filed_route: string; star: string; landed_at: string; events: ReplayEvent[]; snapshots: ReplaySnapshot[]; track: ReplayPoint[]};
type ReplayData = {runway_group: string; go_around_delay_seconds: number; flights: ReplayFlight[]};

function classifiedSTAR(flight: ReplayFlight): string {
  for (let index = flight.snapshots.length - 1; index >= 0; index--) {
    if (flight.snapshots[index].star) return flight.snapshots[index].star;
  }
  return flight.star;
}

function formatTime(value: string): string {
  return new Date(value).toISOString().slice(11, 19);
}

function differenceSeconds(predicted: string, actual: string): number {
  return Math.round((new Date(predicted).getTime() - new Date(actual).getTime()) / 1000);
}

function formatDifference(value: number): string {
  const prefix = value >= 0 ? "+" : "−";
  const absolute = Math.abs(value);
  return `${prefix}${Math.floor(absolute / 60)}m ${String(absolute % 60).padStart(2, "0")}s`;
}

function differenceClass(value: number): string {
  if (Math.abs(value) <= 60) return "text-emerald-300";
  if (Math.abs(value) <= 180) return "text-amber-300";
  return "text-rose-300";
}

function formatSpeed(value: number | null): string {
  return value === null || !Number.isFinite(value) || value <= 0 ? "Unavailable" : `${Math.round(value)} kt`;
}

function speedDifferenceClass(value: number | null): string {
  if (value === null) return "";
  if (Math.abs(value) <= 20) return "text-emerald-300";
  if (Math.abs(value) <= 40) return "text-amber-300";
  return "text-rose-300";
}

function expectedSpeed(snapshot: ReplaySnapshot | null): number | null {
  const value = snapshot?.model_segment?.current_groundspeed_knots ?? snapshot?.model_segment?.groundspeed_knots;
  return value && Number.isFinite(value) ? value : null;
}

function formatWind(value: number | undefined): string {
  if (value === undefined || !Number.isFinite(value)) return "Not applied";
  return `${value >= 0 ? "+" : "−"}${Math.abs(Math.round(value))} kt`;
}

function reportIndexForPrediction(track: ReplayPoint[], predictionAt: string): number {
  const predictionTime = new Date(predictionAt).getTime();
  const index = track.findIndex((point) => new Date(point.at).getTime() >= predictionTime);
  return index >= 0 ? index : Math.max(0, track.length - 1);
}

function eventsAtReport(events: ReplayEvent[], track: ReplayPoint[], reportIndex: number): ReplayEvent[] {
  const current = new Date(track[reportIndex]?.at ?? 0).getTime();
  const previous = reportIndex > 0 ? new Date(track[reportIndex - 1].at).getTime() : Number.NEGATIVE_INFINITY;
  return events.filter((event) => {
    const at = new Date(event.at).getTime();
    return at > previous && at <= current;
  });
}

function eventLabel(event: ReplayEvent): string {
  if (event.type === "threshold_crossing") return event.sequence === 1 ? "First threshold crossing" : `Threshold crossing ${event.sequence ?? ""}`.trim();
  if (event.type === "go_around_detected") return `Go-around detected${event.reason ? ` · ${event.reason.replace(/_/g, " ")}` : ""}`;
  return "Landing proxy";
}

function eventMarkerColor(event: ReplayEvent): string {
  if (event.type === "go_around_detected") return "#fb7185";
  if (event.type === "threshold_crossing" && event.sequence === 1) return "#facc15";
  if (event.type === "threshold_crossing") return "#a78bfa";
  return "#fb923c";
}

function formatElapsed(from: string, to: string): string {
  const seconds = Math.max(0, Math.round((new Date(to).getTime() - new Date(from).getTime()) / 1000));
  return `${Math.floor(seconds / 60)}m ${String(seconds % 60).padStart(2, "0")}s`;
}

function timelineEventLabel(event: ReplayEvent, thresholdCount: number): string {
  if (event.type !== "threshold_crossing") return eventLabel(event);
  if (event.sequence === 1 && thresholdCount > 1) return "Original threshold crossing";
  if (event.sequence === thresholdCount) return thresholdCount > 1 ? "Return / final threshold crossing" : "Final threshold crossing";
  return `Threshold crossing ${event.sequence ?? ""}`.trim();
}

function operationalReferenceLabel(reference: OperationalErrorReference, goAroundDelaySeconds: number): string {
  if (reference.kind === "first_touchdown") return `First touchdown · ${formatTime(reference.at)}Z`;
  if (reference.kind === "go_around_target") {
    return `Go-around target · ${formatTime(reference.at)}Z (+${Math.round(goAroundDelaySeconds / 60)} min)`;
  }
  return `Landing proxy · ${formatTime(reference.at)}Z`;
}

function priorThreshold(events: ReplayEvent[], target: ReplayEvent): ReplayEvent | null {
  const targetAt = new Date(target.at).getTime();
  return events.filter((event) => event.type === "threshold_crossing" && new Date(event.at).getTime() <= targetAt).at(-1) ?? null;
}

type RouteAdaptation = {kind: "direct" | "candidate" | "recovery"; target: string};

function routeAdaptation(snapshot: ReplaySnapshot | null): RouteAdaptation | null {
  const legID = snapshot?.route[0]?.id ?? "";
  const direct = /^(?:DIRECT_TO|OFF_ROUTE_TO):(.+)$/.exec(legID);
  if (direct) return {kind: "direct", target: direct[1]};
  const candidate = /^OFF_ROUTE_CANDIDATE_TO:(.+)$/.exec(legID);
  if (candidate) return {kind: "candidate", target: candidate[1]};
  const recovery = /^OFF_ROUTE_RECOVERY_TO:(.+)$/.exec(legID);
  return recovery ? {kind: "recovery", target: recovery[1]} : null;
}

function FitReplay({coordinates}: {coordinates: LatLngTuple[]}) {
  const map = useMap();
  useEffect(() => {
    if (coordinates.length > 1) map.fitBounds(coordinates, {maxZoom: 9, padding: [36, 36]});
  }, [coordinates, map]);
  useEffect(() => {
    const refresh = () => map.invalidateSize({pan: false});
    const observer = new ResizeObserver(refresh);
    observer.observe(map.getContainer());
    refresh();
    return () => observer.disconnect();
  }, [map]);
  return null;
}

export default function AMANReplayPage() {
  const [data, setData] = useState<ReplayData | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [selectedCID, setSelectedCID] = useState("");
  const [reportIndex, setReportIndex] = useState(0);

  useEffect(() => {
    void fetch("/aman-replay-22l.json", {cache: "no-store"})
      .then(async (response) => {
        if (!response.ok) throw new Error("Replay export has not been generated yet.");
        return await response.json() as ReplayData;
      })
      .then((result) => {
        setData(result);
        setSelectedCID(result.flights[0]?.cid ?? "");
        setReportIndex(0);
      })
      .catch((reason: unknown) => setError(reason instanceof Error ? reason.message : "Unable to load replay export."));
  }, []);

  const flight = useMemo(() => data?.flights.find((value) => value.cid === selectedCID) ?? data?.flights[0] ?? null, [data, selectedCID]);
  const selectedReportIndex = Math.min(reportIndex, Math.max(0, (flight?.track.length ?? 1) - 1));
  const selectedReport = flight?.track[selectedReportIndex] ?? null;
  const snapshotIndex = useMemo(() => {
    if (!flight || !selectedReport) return -1;
    const reportTime = new Date(selectedReport.at).getTime();
    let latest = -1;
    for (let index = 0; index < flight.snapshots.length; index++) {
      if (new Date(flight.snapshots[index].at).getTime() > reportTime) break;
      latest = index;
    }
    return latest;
  }, [flight, selectedReport]);
  const snapshot = snapshotIndex >= 0 ? flight?.snapshots[snapshotIndex] ?? null : null;
  const adaptation = routeAdaptation(snapshot);
  const directTarget = adaptation?.kind === "direct" ? adaptation.target : null;
  const candidateTarget = adaptation?.kind === "candidate" ? adaptation.target : null;
  const fullTrack = useMemo(() => flight?.track.map((point) => [point.latitude, point.longitude] as LatLngTuple) ?? [], [flight]);
  const observedReports = useMemo(() => flight?.track.slice(0, selectedReportIndex + 1) ?? [], [flight, selectedReportIndex]);
  const actualTrack = useMemo(() => observedReports.map((point) => [point.latitude, point.longitude] as LatLngTuple), [observedReports]);
  const actualEnd = actualTrack.at(-1);
  const observedReport = observedReports.at(-1) ?? null;
  const landingAt = flight?.landed_at ?? "";
  const rawDifference = snapshot && landingAt ? differenceSeconds(snapshot.raw_teta, landingAt) : null;
  const operationalReference = snapshot && landingAt
    ? operationalErrorReference(flight?.events ?? [], selectedReport?.at ?? snapshot.at, landingAt, data?.go_around_delay_seconds ?? 600)
    : null;
  const operationalDifference = snapshot && operationalReference ? differenceSeconds(snapshot.operational_teta, operationalReference.at) : null;
  const actualGroundspeed = observedReport?.groundspeed_knots ?? null;
  const modelGroundspeed = expectedSpeed(snapshot);
  const speedDifference = modelGroundspeed !== null && actualGroundspeed !== null ? modelGroundspeed - actualGroundspeed : null;
  const replayEvents = flight?.events ?? [];
  const visibleEvents = selectedReport ? replayEvents.filter((event) => new Date(event.at).getTime() <= new Date(selectedReport.at).getTime()) : [];
  const thresholdEvents = replayEvents.filter((event) => event.type === "threshold_crossing");
  const goAroundDetections = replayEvents.filter((event) => event.type === "go_around_detected");
  const latestVisibleEvent = visibleEvents.at(-1) ?? null;

  if (error) return <main className="grid min-h-screen place-items-center bg-slate-950 p-8 text-slate-200"><p>{error}</p></main>;
  if (!data || !flight) return <main className="grid min-h-screen place-items-center bg-slate-950 p-8 text-slate-200"><p>Loading AMAN replay…</p></main>;

  return <main className="min-h-screen bg-slate-950 p-4 text-slate-100">
    <div className="mx-auto grid max-w-[1600px] gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
      <section className="min-w-0 overflow-hidden border border-slate-700 bg-slate-900 shadow-2xl">
        <header className="flex flex-wrap items-baseline justify-between gap-3 border-b border-slate-700 px-4 py-3">
          <div><h1 className="text-lg font-semibold">AMAN 22L replay</h1><p className="text-sm text-slate-400">Modelled route versus saved VATSIM position reports (15-second feed).</p></div>
          <div className="flex items-center gap-2"><button className="rounded border border-slate-600 px-2 py-1 text-xs text-slate-300 hover:border-cyan-400 hover:text-cyan-200" onClick={() => document.getElementById("prediction-history")?.scrollIntoView({behavior: "smooth", block: "start"})} type="button">Prediction history ↓</button><span className="rounded bg-cyan-950 px-2 py-1 font-mono text-xs text-cyan-200">{data.runway_group}</span></div>
        </header>
        <MapContainer aria-label="AMAN replay route map" center={fullTrack[0] ?? [55.6, 12.6]} className="h-[76vh] min-h-[560px] w-full" scrollWheelZoom={false} worldCopyJump zoom={8}>
          <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>' keepBuffer={4} url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png" />
          <FitReplay coordinates={fullTrack} />
          {snapshot?.route.map((leg, index) => <Polyline key={leg.id} pathOptions={index === 0 && directTarget ? {color: "#facc15", dashArray: "10 7", opacity: 1, weight: 5} : index === 0 && candidateTarget ? {color: "#fb923c", dashArray: "4 8", opacity: 1, weight: 4} : {color: "#22d3ee", opacity: 0.9, weight: 4}} positions={[leg.from, leg.to]}><Tooltip sticky>{leg.id} · leg average {formatSpeed(leg.expected_groundspeed_knots)} · {leg.distance_nm.toFixed(1)} NM / {Math.round(leg.duration_seconds)}s</Tooltip></Polyline>)}
          <Polyline pathOptions={{color: "#f97316", dashArray: "4 8", opacity: 0.32, weight: 2}} positions={fullTrack} />
          <Polyline pathOptions={{color: "#fb923c", opacity: 0.95, weight: 3}} positions={actualTrack} />
          {snapshot?.route.map((leg, index) => <CircleMarker center={leg.to} key={`fix-${leg.id}`} pathOptions={{color: "#0f172a", fillColor: index === 0 && directTarget ? "#facc15" : index === 0 && candidateTarget ? "#fb923c" : "#e2e8f0", fillOpacity: 1, weight: 1}} radius={index === 0 && (directTarget || candidateTarget) ? 5 : 3.5}><Tooltip direction="top" opacity={0.95}>{index === 0 && directTarget ? `Confirmed inferred direct to ${directTarget}` : index === 0 && candidateTarget ? `Candidate direct to ${candidateTarget} — awaiting stable track` : leg.id}<br />Leg-average speed {formatSpeed(leg.expected_groundspeed_knots)}</Tooltip></CircleMarker>)}
          {actualTrack[0] && <CircleMarker center={actualTrack[0]} pathOptions={{color: "#0f172a", fillColor: "#60a5fa", fillOpacity: 1, weight: 2}} radius={6}><Tooltip>First captured VATSIM point</Tooltip></CircleMarker>}
          {visibleEvents.map((event, index) => <CircleMarker center={[event.latitude, event.longitude]} key={`${event.type}-${event.at}-${index}`} pathOptions={{color: "#0f172a", fillColor: eventMarkerColor(event), fillOpacity: 1, weight: 2}} radius={event.type === "go_around_detected" ? 8 : 6}><Tooltip direction="top" opacity={0.95}>{eventLabel(event)}<br />{formatTime(event.at)}Z{event.type === "go_around_detected" && event.supporting_observation_times?.length ? <><br />Evidence: {event.supporting_observation_times.map((value) => `${formatTime(value)}Z`).join(", ")}</> : null}</Tooltip></CircleMarker>)}
          {actualEnd && <CircleMarker center={actualEnd} pathOptions={{color: "#0f172a", fillColor: "#4ade80", fillOpacity: 1, weight: 2}} radius={6}><Tooltip>{observedReport ? <>Selected feed report · {formatTime(observedReport.at)}Z<br />Actual {formatSpeed(actualGroundspeed)} · expected {formatSpeed(modelGroundspeed)}<br />Model − actual {speedDifference === null ? "Unavailable" : `${speedDifference >= 0 ? "+" : "−"}${Math.abs(Math.round(speedDifference))} kt`}</> : "Selected feed report"}</Tooltip></CircleMarker>}
        </MapContainer>
        <div className="border-t border-slate-700 px-4 py-3">
          <h2 className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-400">Raw VATSIM flight-plan route</h2>
          <p className="break-words font-mono text-sm text-slate-200">{flight.filed_route || "No route was filed in the captured VATSIM data."}</p>
        </div>
        <div className="flex flex-wrap gap-x-5 gap-y-2 border-t border-slate-700 px-4 py-3 text-xs text-slate-300"><span><i className="mr-2 inline-block h-1 w-7 bg-cyan-400 align-middle" />Modelled AMAN route</span><span><i className="mr-2 inline-block h-1 w-7 border-t-2 border-dashed border-yellow-300 align-middle" />Confirmed inferred direct</span><span><i className="mr-2 inline-block h-1 w-7 border-t-2 border-dashed border-orange-400 align-middle" />Candidate direct</span><span><i className="mr-2 inline-block h-1 w-7 bg-orange-400 align-middle" />Actual VATSIM track to selected report</span><span><i className="mr-2 inline-block h-1 w-7 border-t border-dashed border-orange-400 align-middle" />Remaining captured route</span><span><i className="mr-2 inline-block h-3 w-3 rounded-full bg-yellow-400 align-middle" />First threshold</span><span><i className="mr-2 inline-block h-3 w-3 rounded-full bg-rose-400 align-middle" />Go-around detected</span><span><i className="mr-2 inline-block h-3 w-3 rounded-full bg-violet-400 align-middle" />Return threshold</span><span><i className="mr-2 inline-block h-3 w-3 rounded-full bg-green-400 align-middle" />Selected feed report</span></div>
      </section>
      <aside className="space-y-4 border border-slate-700 bg-slate-900 p-4">
        <label className="grid gap-2 text-sm font-medium">Arrival
          <select className="rounded border border-slate-600 bg-slate-950 px-3 py-2 font-mono text-sm" onChange={(event) => { setSelectedCID(event.target.value); setReportIndex(0); }} value={flight.cid}>{data.flights.map((value) => <option key={value.cid} value={value.cid}>{value.callsign || value.cid} · {classifiedSTAR(value) || "unclassified"} · {formatTime(value.landed_at)}{value.events?.some((item) => item.type === "go_around_detected") ? " · GO-AROUND" : ""}</option>)}</select>
        </label>
        <label className="grid gap-2 text-sm font-medium">VATSIM position
          <input className="accent-cyan-400" max={Math.max(0, flight.track.length - 1)} min="0" onChange={(event) => setReportIndex(Number(event.target.value))} step="1" type="range" value={selectedReportIndex} />
          <span className="font-mono text-xs font-normal text-slate-400">{selectedReport ? `${formatTime(selectedReport.at)}Z · ${selectedReportIndex + 1}/${flight.track.length}` : "No report"}</span>
        </label>
        <dl className="grid gap-3 text-sm"><div><dt className="text-slate-400">Callsign</dt><dd className="font-mono">{flight.callsign || flight.cid}</dd></div><div><dt className="text-slate-400">STAR family</dt><dd>{snapshot?.star || flight.star || "Unclassified"}</dd></div><div><dt className="text-slate-400">Route adaptation</dt><dd className={directTarget ? "font-semibold text-yellow-200" : candidateTarget ? "font-semibold text-orange-300" : "text-slate-300"}>{directTarget ? `Confirmed inferred direct to ${directTarget}` : candidateTarget ? `Candidate direct to ${candidateTarget} — waiting for stable track` : adaptation?.kind === "recovery" ? `Conservative recovery via ${adaptation.target}` : "Published route"}</dd></div><div><dt className="text-slate-400">Actual landing proxy</dt><dd className="font-mono text-orange-200">{formatTime(flight.landed_at)}Z</dd></div><div><dt className="text-slate-400">Selected VATSIM report</dt><dd className="font-mono">{observedReport ? `${formatTime(observedReport.at)}Z · ${observedReport.latitude.toFixed(3)}, ${observedReport.longitude.toFixed(3)} · ${observedReport.altitude_feet} ft` : "Unavailable"}</dd></div><div><dt className="text-slate-400">Reports shown</dt><dd>{actualTrack.length}/{flight.track.length} positions</dd></div><div><dt className="text-slate-400">Freeze state</dt><dd>{snapshot?.freeze_reason || "none"}</dd></div>{goAroundDetections.length > 0 && <div><dt className="text-slate-400">Go-around status at report</dt><dd className={latestVisibleEvent?.type === "go_around_detected" ? "font-semibold text-rose-300" : ""}>{latestVisibleEvent ? eventLabel(latestVisibleEvent) : "Approach not yet reached"}</dd></div>}</dl>
        {goAroundDetections.length > 0 && <section className="border-y border-slate-700 py-4"><h2 className="mb-1 text-sm font-semibold">Go-around detection</h2><p className="mb-3 text-xs text-slate-500">Threshold times are interpolated between the surrounding 15-second reports. Detection is the AMAN multi-sample detector result.</p><ol className="grid gap-3 text-sm">
          {replayEvents.map((event, index) => {
            const crossing = event.type === "go_around_detected" ? priorThreshold(replayEvents, event) : null;
            const delay = crossing ? formatElapsed(crossing.at, event.at) : null;
            const dotClass = event.type === "go_around_detected" ? "bg-rose-400" : event.type === "landing_proxy" ? "bg-orange-400" : event.sequence === 1 ? "bg-yellow-400" : "bg-violet-400";
            const timeClass = event.type === "go_around_detected" ? "text-rose-200" : event.type === "landing_proxy" ? "text-orange-200" : event.sequence === 1 ? "text-yellow-200" : "text-violet-200";
            return <li className={new Date(event.at).getTime() <= new Date(selectedReport?.at ?? 0).getTime() ? "" : "opacity-45"} key={`${event.type}-${event.at}-${index}`}><button className="grid w-full grid-cols-[1fr_auto] text-left" onClick={() => setReportIndex(reportIndexForPrediction(flight.track, event.at))} type="button"><span><i className={`mr-2 inline-block h-2.5 w-2.5 rounded-full ${dotClass}`} />{timelineEventLabel(event, thresholdEvents.length)}</span><span className={`font-mono ${timeClass}`}>{formatTime(event.at)}Z</span></button>{event.type === "go_around_detected" && <p className="ml-5 mt-1 text-xs text-slate-500">{delay ? `${delay} after threshold · ` : ""}evidence {event.supporting_observation_times?.map((value) => `${formatTime(value)}Z`).join(" + ")}</p>}</li>;
          })}
        </ol></section>}
        <section className="border-y border-slate-700 py-4"><h2 className="mb-3 text-sm font-semibold">Speed check at selected report</h2><dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-2 text-sm"><dt className="text-slate-400">Actual groundspeed</dt><dd className="font-mono">{formatSpeed(actualGroundspeed)}</dd><dt className="text-slate-400">Expected GS at report</dt><dd className="font-mono">{formatSpeed(modelGroundspeed)}</dd><dt className="text-slate-400">Model − actual</dt><dd className={`font-mono ${speedDifferenceClass(speedDifference)}`}>{speedDifference === null ? "Unavailable" : `${speedDifference >= 0 ? "+" : "−"}${Math.abs(Math.round(speedDifference))} kt`}</dd><dt className="text-slate-400">Model phase</dt><dd className="max-w-48 text-right">{snapshot?.model_segment?.phase_name ?? "Unavailable"}</dd><dt className="text-slate-400">Model IAS</dt><dd className="font-mono">{formatSpeed(snapshot?.model_segment?.indicated_airspeed_knots ?? null)}</dd><dt className="text-slate-400">No-wind GS at report</dt><dd className="font-mono">{formatSpeed(snapshot?.model_segment?.current_no_wind_groundspeed_knots ?? snapshot?.model_segment?.no_wind_groundspeed_knots ?? null)}</dd><dt className="text-slate-400">Wind component</dt><dd className="font-mono">{formatWind(snapshot?.model_segment?.tailwind_knots)}</dd></dl><p className="mt-3 text-xs text-slate-500">The comparison evaluates AMAN’s first remaining segment at the selected report altitude, rather than using that segment’s midpoint or whole-leg average. Provider wind is preferred; if unavailable, AMAN estimates the along-track component from the observed track and groundspeed. A missing component means no wind correction was safe to apply.</p></section>
        <section className="border-y border-slate-700 py-4"><h2 className="mb-1 text-sm font-semibold">Prediction comparison</h2><p className="mb-3 text-xs text-slate-500">{snapshot ? `Latest AMAN calculation: ${formatTime(snapshot.at)}Z · prediction ${snapshotIndex + 1}/${flight.snapshots.length}` : "No AMAN prediction was retained yet for this earlier VATSIM report."}</p><dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-2 text-sm"><dt className="text-slate-400">Raw TETA</dt><dd className="font-mono">{snapshot ? `${formatTime(snapshot.raw_teta)}Z` : "Unavailable"}</dd><dt className="text-slate-400">Raw error</dt><dd className={`font-mono ${rawDifference === null ? "" : differenceClass(rawDifference)}`}>{rawDifference === null ? "Unavailable" : formatDifference(rawDifference)}</dd><dt className="text-slate-400">Operational TETA</dt><dd className="font-mono">{snapshot ? `${formatTime(snapshot.operational_teta)}Z` : "Unavailable"}</dd><dt className="text-slate-400">Operational reference</dt><dd className="text-right font-mono">{operationalReference ? operationalReferenceLabel(operationalReference, data.go_around_delay_seconds) : "Unavailable"}</dd><dt className="text-slate-400">Operational error</dt><dd className={`font-mono ${operationalDifference === null ? "" : differenceClass(operationalDifference)}`}>{operationalDifference === null ? "Unavailable" : formatDifference(operationalDifference)}</dd></dl><p className="mt-3 text-xs text-slate-500">Raw error remains relative to the final landing proxy. Operational error uses the first touchdown until a go-around is detected; after detection it uses the configured +{Math.round(data.go_around_delay_seconds / 60)} minute target.</p></section>
        <p className="text-xs leading-relaxed text-slate-400">Hover a white route point for its leg ID. Cyan is AMAN’s latest route at the selected VATSIM position; orange is the actual VATSIM route. Select any row below to jump to the matching actual report.</p>
      </aside>
      <section className="overflow-hidden border border-slate-700 bg-slate-900 lg:col-span-2" id="prediction-history"><header className="flex flex-wrap items-baseline justify-between gap-2 border-b border-slate-700 px-4 py-3"><div><h2 className="font-semibold">Prediction history — each AMAN feed step</h2><p className="text-xs text-slate-400">Compare landing prediction and current-segment model speed with the corresponding VATSIM report.</p></div><span className="text-xs text-slate-400">Speed Δ: green ≤20 kt · amber ≤40 kt · red &gt;40 kt</span></header><div className="max-h-[420px] overflow-auto"><table className="w-full min-w-[1760px] border-collapse text-left text-sm"><thead className="sticky top-0 bg-slate-800 text-xs uppercase tracking-wide text-slate-400"><tr><th className="px-4 py-3">Step</th><th className="px-4 py-3">Feed report</th><th className="px-4 py-3">Altitude</th><th className="px-4 py-3">Model phase</th><th className="px-4 py-3">Actual GS</th><th className="px-4 py-3">Expected GS</th><th className="px-4 py-3">Model − actual</th><th className="px-4 py-3">Raw TETA</th><th className="px-4 py-3">Raw − final landing</th><th className="px-4 py-3">Operational TETA</th><th className="px-4 py-3">Operational reference</th><th className="px-4 py-3">Operational − reference</th><th className="px-4 py-3">Freeze</th><th className="px-4 py-3">Go-around event</th></tr></thead><tbody>{flight.snapshots.map((value, index) => { const raw = differenceSeconds(value.raw_teta, flight.landed_at); const selected = index === snapshotIndex; const reportIndex = reportIndexForPrediction(flight.track, value.at); const report = flight.track[reportIndex]; const reference = operationalErrorReference(replayEvents, report?.at ?? value.at, flight.landed_at, data.go_around_delay_seconds); const operational = differenceSeconds(value.operational_teta, reference.at); const actualSpeed = report?.groundspeed_knots ?? null; const modelSpeed = expectedSpeed(value); const speedDelta = modelSpeed !== null && actualSpeed !== null ? modelSpeed - actualSpeed : null; const stepEvents = eventsAtReport(replayEvents, flight.track, reportIndex); return <tr className={selected ? "bg-cyan-950/70" : "border-t border-slate-800 hover:bg-slate-800/80"} key={`${value.at}-${index}`}><td className="px-4 py-2"><button className="font-mono text-cyan-200 underline-offset-2 hover:underline" onClick={() => setReportIndex(reportIndex)} type="button">{index + 1}</button></td><td className="px-4 py-2 font-mono">{formatTime(value.at)}Z</td><td className="px-4 py-2 font-mono">{report ? `${report.altitude_feet} ft` : "—"}</td><td className="px-4 py-2">{value.model_segment?.phase_id ?? "—"}</td><td className="px-4 py-2 font-mono">{formatSpeed(actualSpeed)}</td><td className="px-4 py-2 font-mono">{formatSpeed(modelSpeed)}</td><td className={`px-4 py-2 font-mono ${speedDifferenceClass(speedDelta)}`}>{speedDelta === null ? "—" : `${speedDelta >= 0 ? "+" : "−"}${Math.abs(Math.round(speedDelta))} kt`}</td><td className="px-4 py-2 font-mono">{formatTime(value.raw_teta)}Z</td><td className={`px-4 py-2 font-mono ${differenceClass(raw)}`}>{formatDifference(raw)}</td><td className="px-4 py-2 font-mono">{formatTime(value.operational_teta)}Z</td><td className="px-4 py-2 font-mono">{operationalReferenceLabel(reference, data.go_around_delay_seconds)}</td><td className={`px-4 py-2 font-mono ${differenceClass(operational)}`}>{formatDifference(operational)}</td><td className="px-4 py-2">{value.freeze_reason || "none"}</td><td className="px-4 py-2">{stepEvents.length ? stepEvents.map(eventLabel).join(" · ") : "—"}</td></tr>; })}</tbody></table></div></section>
    </div>
  </main>;
}
