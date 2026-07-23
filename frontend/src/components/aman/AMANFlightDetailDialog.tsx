import {useAuth0} from "@auth0/auth0-react";
import {useEffect, useMemo, useState} from "react";
import {divIcon, type LatLngTuple} from "leaflet";
import {CircleMarker, MapContainer, Marker, Polyline, TileLayer, useMap} from "react-leaflet";
import "leaflet/dist/leaflet.css";

import {fetchAMANFlightDetail, type AMANCalculationLeg, type AMANFlightDetail} from "@/api/aman-detail";

function displayTime(value: string | null | undefined): string {
  if (!value) return "Unavailable";
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? value : parsed.toISOString().slice(11, 19);
}

function duration(seconds: number): string {
  const absolute = Math.abs(seconds);
  return `${Math.floor(absolute / 60)}:${String(absolute % 60).padStart(2, "0")}`;
}

function number(value: number | null, suffix = ""): string {
  return value === null ? "Unavailable" : `${value.toFixed(1)}${suffix}`;
}

// The custom aircraft glyph faces true north at zero rotation. AMAN supplies
// a derived true ground track, also clockwise from north.
function aircraftIconRotation(trackTrueDegrees: number | null): number {
  return (trackTrueDegrees ?? 0) % 360;
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>'"]/g, (character) => ({"&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", "\"": "&quot;"})[character] ?? character);
}

function routeLabelIcon(name: string) {
  return divIcon({
    className: "aman-route-label",
    html: `<div style="position:relative;width:124px;height:30px;pointer-events:none"><svg aria-hidden="true" width="18" height="18" viewBox="0 0 18 18" style="position:absolute;left:0;top:0;overflow:visible"><path d="M2 2 L13 15" fill="none" stroke="#64748b" stroke-width="1.5"/><circle cx="2" cy="2" r="2" fill="#94a3b8"/></svg><span style="position:absolute;left:19px;top:10px;max-width:104px;overflow:hidden;color:#a8b1be;font:500 10px/1.1 system-ui,sans-serif;letter-spacing:.025em;text-overflow:ellipsis;text-shadow:0 1px 2px #111827;white-space:nowrap">${escapeHTML(name)}</span></div>`,
    iconAnchor: [2, 2],
  });
}

function FitRoute({coordinates}: {coordinates: LatLngTuple[]}) {
  const map = useMap();
  useEffect(() => {
    if (coordinates.length > 1) map.fitBounds(coordinates, {padding: [34, 34], maxZoom: 9});
    else if (coordinates.length === 1) map.setView(coordinates[0], 9);
  }, [coordinates, map]);
  return null;
}

function RouteLabels({legs}: {legs: AMANCalculationLeg[]}) {
  const map = useMap();
  const [viewportRevision, setViewportRevision] = useState(0);

  useEffect(() => {
    const refresh = () => setViewportRevision((revision) => revision + 1);
    map.on("moveend zoomend resize", refresh);
    refresh();
    return () => {
      map.off("moveend zoomend resize", refresh);
    };
  }, [map]);

  const visibleLegs = useMemo(() => {
    void viewportRevision;
    const mapSize = map.getSize();
    const occupied: Array<{bottom: number; left: number; right: number; top: number}> = [];
    const visible: AMANCalculationLeg[] = [];

    for (const leg of [...legs].reverse()) {
      const point = map.latLngToContainerPoint([leg.end_latitude, leg.end_longitude]);
      const width = Math.max(44, Math.min(104, leg.to.length * 6.25 + 8));
      const bounds = {bottom: point.y + 25, left: point.x + 17, right: point.x + 17 + width, top: point.y + 7};
      const onMap = bounds.right >= 0 && bounds.left <= mapSize.x && bounds.bottom >= 0 && bounds.top <= mapSize.y;
      const overlaps = occupied.some((other) => bounds.left < other.right && bounds.right > other.left && bounds.top < other.bottom && bounds.bottom > other.top);
      if (!onMap || overlaps) continue;
      occupied.push(bounds);
      visible.push(leg);
    }

    return visible.reverse();
  }, [legs, map, viewportRevision]);

  return <>{visibleLegs.map((leg) => <Marker icon={routeLabelIcon(leg.to)} key={`fix-label-${leg.id}`} position={[leg.end_latitude, leg.end_longitude]} />)}</>;
}

function RouteMap({detail}: {detail: AMANFlightDetail}) {
  const legs = detail.calculation?.legs ?? [];
  const routeCoordinates: LatLngTuple[] = legs.flatMap((leg) => [
    [leg.start_latitude, leg.start_longitude] as LatLngTuple,
    [leg.end_latitude, leg.end_longitude] as LatLngTuple,
  ]);
  const allCoordinates = detail.position
    ? [[detail.position.latitude, detail.position.longitude] as LatLngTuple, ...routeCoordinates]
    : routeCoordinates;
  if (allCoordinates.length === 0) return <div className="grid min-h-72 place-items-center bg-slate-950 text-sm text-slate-400">Route geometry is not available for this prediction.</div>;
  const initialCenter = allCoordinates[0];
  const aircraftIcon = detail.position ? divIcon({
    className: "",
    html: `<svg aria-label="Aircraft" viewBox="0 0 24 24" width="30" height="30" style="display:block;filter:drop-shadow(0 1px 2px #000);transform:rotate(${aircraftIconRotation(detail.position.track_true_degrees)}deg)"><path d="M12 1 15 10 21 14 21 16 14 14 14 21 17 23 17 24 7 24 7 23 10 21 10 14 3 16 3 14 9 10Z" fill="#f3d02e" stroke="#111827" stroke-width="1.4" stroke-linejoin="round"/></svg>`,
    iconAnchor: [15, 15],
  }) : undefined;

  return (
    <div className="relative min-h-[360px] overflow-hidden border border-slate-600 bg-slate-950" data-testid="aman-route-map">
      <MapContainer aria-label="OpenStreetMap aircraft route" center={initialCenter} className="h-full min-h-[360px] w-full" scrollWheelZoom zoom={8}>
        <TileLayer attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>' url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png" />
        <FitRoute coordinates={allCoordinates} />
        {legs.map((leg, index) => <Polyline key={leg.id} pathOptions={{color: index === 0 ? "#f3d02e" : "#0891b2", weight: 4, opacity: 0.9}} positions={[[leg.start_latitude, leg.start_longitude], [leg.end_latitude, leg.end_longitude]]} />)}
        {legs.map((leg) => <CircleMarker center={[leg.end_latitude, leg.end_longitude]} key={`fix-${leg.id}`} pathOptions={{color: "#0f172a", fillColor: "#dbe4ef", fillOpacity: 1, weight: 1}} radius={3.5} />)}
        <RouteLabels legs={legs} />
        {detail.position && <Marker icon={aircraftIcon} position={[detail.position.latitude, detail.position.longitude]} />}
      </MapContainer>
      <div className="pointer-events-none absolute bottom-6 left-2 z-[500] rounded bg-slate-950/90 px-2 py-1 text-xs text-slate-100">Aircraft: {detail.position ? `${number(detail.position.altitude_feet, " ft")} · ${number(detail.position.groundspeed_knots, " kt")}` : "unavailable"}</div>
    </div>
  );
}

function LegTable({legs}: {legs: AMANCalculationLeg[]}) {
  const rows = legs.map((leg, index) => ({
    leg,
    cumulative: legs.slice(0, index + 1).reduce((total, current) => total + current.duration_seconds, 0),
  }));
  return <div className="overflow-auto border border-slate-700"><table className="w-full min-w-[760px] text-left text-xs"><thead className="sticky top-0 bg-slate-800 text-slate-300"><tr><th className="p-2">Section</th><th className="p-2">Distance</th><th className="p-2">Course</th><th className="p-2">No wind</th><th className="p-2">Wind model</th><th className="p-2">Wind delta</th><th className="p-2">Cumulative</th></tr></thead><tbody>{rows.map(({leg, cumulative}) => { const delta = leg.no_wind_duration_seconds === null ? null : leg.duration_seconds - leg.no_wind_duration_seconds; return <tr className="border-t border-slate-800" key={leg.id}><td className="p-2 font-medium text-white">{leg.from} → {leg.to}</td><td className="p-2">{leg.distance_nm.toFixed(1)} NM</td><td className="p-2">{Math.round(leg.course_true_degrees).toString().padStart(3, "0")}°T</td><td className="p-2">{leg.no_wind_duration_seconds === null ? "Unavailable" : duration(leg.no_wind_duration_seconds)}</td><td className="p-2">{duration(leg.duration_seconds)}</td><td className={delta === null || delta === 0 ? "p-2" : delta < 0 ? "p-2 text-emerald-300" : "p-2 text-amber-300"}>{delta === null ? "—" : <>{delta > 0 ? "+" : delta < 0 ? "−" : ""}{duration(Math.abs(delta))}</>}</td><td className="p-2">{duration(cumulative)}</td></tr>; })}</tbody></table></div>;
}

export function AMANFlightDetailDialog({airport, flightID, onClose}: {airport: string; flightID: string; onClose: () => void}) {
  const {getAccessTokenSilently} = useAuth0();
  const [detail, setDetail] = useState<AMANFlightDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const detailKey = useMemo(() => `${airport}/${flightID}`, [airport, flightID]);

  useEffect(() => {
    const abort = new AbortController();
    void (async () => {
      setLoading(true); setError(null); setDetail(null);
      try {
        const token = await getAccessTokenSilently();
        const value = await fetchAMANFlightDetail(token, airport, flightID, abort.signal);
        if (!abort.signal.aborted) setDetail(value);
      } catch (reason) {
        if (!abort.signal.aborted) setError(reason instanceof Error ? reason.message : "Unable to load AMAN flight detail.");
      } finally {
        if (!abort.signal.aborted) setLoading(false);
      }
    })();
    return () => abort.abort();
  }, [airport, flightID, getAccessTokenSilently, detailKey]);

  return <div aria-modal="true" aria-label="AMAN route and prediction detail" className="fixed inset-0 z-[100] grid place-items-center bg-black/70 p-5" role="dialog" onMouseDown={onClose}>
    <section className="flex max-h-[calc(100dvh-2.5rem)] w-full max-w-[1400px] flex-col overflow-hidden border border-slate-500 bg-[#161d27] text-slate-100 shadow-2xl" onMouseDown={(event) => event.stopPropagation()}>
      <header className="flex items-center justify-between border-b border-slate-600 bg-[#242d3a] px-5 py-3"><div><h2 className="text-lg font-semibold">{detail?.flight.callsign ?? flightID} — route & prediction detail</h2><p className="text-xs text-slate-400">On-demand AMAN evidence · state revision {detail?.revision ?? "—"}</p></div><button className="rounded border border-slate-400 px-3 py-1 text-sm hover:bg-slate-700" onClick={onClose} type="button">Close</button></header>
      <div className="min-h-0 overflow-x-hidden overflow-y-auto p-3 sm:p-5">
        {loading && <div className="grid min-h-80 place-items-center text-slate-300">Loading current AMAN detail…</div>}
        {error && <div role="alert" className="rounded border border-red-500 bg-red-950 p-4 text-red-100">{error}</div>}
        {detail && <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(360px,0.8fr)]">
          <div className="grid gap-6"><RouteMap detail={detail} /><section><h3 className="mb-3 font-semibold">Prediction sections</h3>{detail.calculation ? <><div className="mb-3 grid gap-3 sm:grid-cols-4 text-sm"><span className="rounded bg-slate-800 p-3">Distance <b>{number(detail.calculation.distance_to_go_nm, " NM")}</b></span><span className="rounded bg-slate-800 p-3">No wind <b>{duration(detail.calculation.no_wind_duration_seconds)}</b></span><span className="rounded bg-slate-800 p-3">Wind model <b>{duration(detail.calculation.duration_seconds)}</b></span><span className="rounded bg-slate-800 p-3">Wind delta <b className={detail.calculation.duration_seconds > detail.calculation.no_wind_duration_seconds ? "text-amber-300" : detail.calculation.duration_seconds < detail.calculation.no_wind_duration_seconds ? "text-emerald-300" : ""}>{detail.calculation.duration_seconds > detail.calculation.no_wind_duration_seconds ? "+" : detail.calculation.duration_seconds < detail.calculation.no_wind_duration_seconds ? "−" : ""}{duration(Math.abs(detail.calculation.duration_seconds - detail.calculation.no_wind_duration_seconds))}</b></span></div><LegTable legs={detail.calculation.legs} /></> : <div className="rounded bg-slate-800 p-3 text-sm text-slate-300">No publishable route calculation is available.</div>}</section></div>
          <div className="grid content-start gap-6"><section className="rounded border border-slate-700 p-4"><h3 className="mb-3 font-semibold">Aircraft & performance input</h3><dl className="grid grid-cols-[1fr_auto] gap-x-6 gap-y-3 text-sm"><dt>Aircraft ICAO</dt><dd>{detail.flight.aircraft_type ?? "Unavailable"}</dd><dt>Wake category</dt><dd>{detail.flight.wake_category ?? "Unavailable"}</dd><dt>Observed altitude</dt><dd>{number(detail.position?.altitude_feet ?? null, " ft")}</dd><dt>Observed groundspeed</dt><dd>{number(detail.position?.groundspeed_knots ?? null, " kt")}</dd><dt>Derived true track</dt><dd>{number(detail.position?.track_true_degrees ?? null, "°T")}</dd><dt>Performance profile</dt><dd>{detail.teta_basis?.performance_profile_id ?? "Unavailable"}</dd><dt>Wind source</dt><dd>{detail.teta_basis?.weather_source ?? "Unavailable"}</dd></dl></section><section className="rounded border border-slate-700 p-4"><h3 className="mb-3 font-semibold">Operational TETA basis</h3>{detail.teta_basis ? <><dl className="grid grid-cols-[1fr_auto] gap-x-6 gap-y-3 text-sm"><dt>Raw model TETA</dt><dd>{displayTime(detail.teta_basis.raw_teta)}</dd><dt>Raw no-wind RETA</dt><dd>{displayTime(detail.teta_basis.raw_reta)}</dd><dt>Operational TETA</dt><dd className="font-semibold text-amber-300">{displayTime(detail.teta_basis.operational_teta)}</dd><dt>Operational rule</dt><dd>{detail.teta_basis.operational_reason}</dd><dt>Freeze</dt><dd>{detail.teta_basis.freeze_reason}</dd><dt>Prediction input</dt><dd>{displayTime(detail.teta_basis.input_observed_at)}</dd></dl><div className="mt-4 border-t border-slate-700 pt-3 text-xs text-slate-300">Model {detail.teta_basis.model_version} · config {detail.teta_basis.config_version} · confidence {detail.teta_basis.confidence}<br />Profile {detail.teta_basis.performance_profile_id ?? "unavailable"} · weather {detail.teta_basis.weather_source ?? "unavailable"}<br />Sources: {detail.teta_basis.sources.join(", ") || "none"}{detail.teta_basis.degradation_reason && <><br /><span className="text-amber-300">Degraded: {detail.teta_basis.degradation_reason}</span></>}</div></> : <p className="text-sm text-slate-400">No publishable TETA calculation.</p>}</section>
            <section className="rounded border border-slate-700 p-4"><h3 className="mb-3 font-semibold">Operational slot basis</h3>{detail.slot_basis ? <dl className="grid grid-cols-[1fr_auto] gap-x-6 gap-y-3 text-sm"><dt>Committed slot</dt><dd className="font-semibold text-cyan-300">{displayTime(detail.slot_basis.time)}</dd><dt>Runway group / sequence</dt><dd>{detail.slot_basis.runway_group_id} / {detail.slot_basis.sequence}</dd><dt>Slot reason</dt><dd>{detail.slot_basis.reason}</dd><dt>Active rate</dt><dd>{detail.slot_basis.rate_per_hour || "Unavailable"} / hour</dd><dt>Preceding slot</dt><dd>{detail.slot_basis.previous_flight ? `${detail.slot_basis.previous_flight.callsign} ${displayTime(detail.slot_basis.previous_flight.slot_time)}` : "None"}</dd><dt>Protected by freeze</dt><dd>{detail.slot_basis.frozen ? "Yes" : "No"}</dd></dl> : <p className="text-sm text-slate-400">No committed operational slot.</p>}</section>
            {detail.teta_basis?.baseline && <section className="rounded border border-slate-700 p-4 text-sm"><h3 className="mb-3 font-semibold">Initial baseline & review</h3><div>Baseline: {displayTime(detail.teta_basis.baseline.arrival_at)} ({detail.teta_basis.baseline.source})</div>{detail.teta_basis.eta_review && <div className="mt-3 rounded bg-amber-950/60 p-3">Review: {detail.teta_basis.eta_review.status}; selected {displayTime(detail.teta_basis.eta_review.selected_teta)}.</div>}</section>}
            {detail.teta_basis && <section className="rounded border border-slate-700 p-4 text-sm"><h3 className="mb-3 font-semibold">Raw prediction samples</h3>{detail.teta_basis.raw_samples.length ? <ul className="space-y-2 text-slate-300">{detail.teta_basis.raw_samples.map((sample) => <li key={sample.generated_at}>{displayTime(sample.generated_at)} → {displayTime(sample.teta)}</li>)}</ul> : <p className="text-slate-400">No smoothing samples retained.</p>}</section>}
          </div>
        </div>}
      </div>
    </section>
  </div>;
}
