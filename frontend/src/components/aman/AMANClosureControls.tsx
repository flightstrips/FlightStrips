import {useState} from "react";
import type {AMANCommandIntent, AMANFlight, AMANPendingCommand, AMANRunwayGroup} from "@/api/aman";

const field = "rounded border border-slate-500 bg-slate-950 px-2 py-1 text-sm text-white";
const button = "rounded border border-slate-500 bg-slate-700 px-2 py-1 text-sm text-white disabled:opacity-40";
const utc = (value: string) => { const parsed = new Date(`${value}Z`); return Number.isNaN(parsed.valueOf()) ? null : parsed.toISOString(); };

export function AMANClosureControls({disabled, groups, flights, pending, onCommand}: {disabled: boolean; groups: AMANRunwayGroup[]; flights: AMANFlight[]; pending: AMANPendingCommand[]; onCommand: (intent: AMANCommandIntent) => void}) {
  const [runway, setRunway] = useState(""); const [mode, setMode] = useState<"absolute" | "after">("absolute");
  const [start, setStart] = useState(""); const [after, setAfter] = useState(""); const [end, setEnd] = useState(""); const [reason, setReason] = useState("");
  const selected = groups.some((group) => group.id === runway) ? runway : groups[0]?.id ?? "";
  const anchors = flights.filter((flight) => flight.runway_group_id === selected && flight.slot !== null);
  const busy = pending.some((command) => command.type === "aman.create_runway_closure" || command.type === "aman.remove_runway_closure");
  const valid = Boolean(reason.trim() && (mode === "absolute" ? utc(start) : anchors.some((flight) => flight.flight_id === after)) && (!end || utc(end)));
  const send = () => {
    const endAt = end ? utc(end) ?? undefined : undefined; if (!selected || !reason.trim() || (end && !endAt)) return;
    if (mode === "absolute") { const startAt = utc(start); if (startAt) onCommand({type: "aman.create_runway_closure", runway_group_id: selected, start: startAt, end: endAt, reason: reason.trim()}); }
    else if (after) onCommand({type: "aman.create_runway_closure", runway_group_id: selected, after_flight_id: after, end: endAt, reason: reason.trim()});
  };
  return <section aria-label="Runway closures" className="grid gap-2 rounded border-2 border-red-700 p-3">
    <h3 className="font-semibold">Runway closures</h3>
    {groups.flatMap((group) => (group.closures ?? []).map((closure) => <div className="flex flex-wrap items-center justify-between gap-2 text-sm" key={closure.id}>
      <span>⛔ <b>{group.id}</b> {closure.reason}: {closure.start} → {closure.end ?? "INDEFINITE"} (server confirmed)</span>
      <button aria-label={`Remove closure ${closure.id} from ${group.id}`} className={button} disabled={disabled || busy || !reason.trim()} onClick={() => onCommand({type: "aman.remove_runway_closure", runway_group_id: group.id, closure_id: closure.id, reason: reason.trim()})}>Remove</button>
    </div>))}
    <div className="flex flex-wrap gap-2">
      <select aria-label="Closure runway" className={field} value={selected} onChange={(event) => setRunway(event.target.value)}>{groups.map((group) => <option key={group.id}>{group.id}</option>)}</select>
      <select aria-label="Closure start mode" className={field} value={mode} onChange={(event) => setMode(event.target.value as "absolute" | "after")}><option value="absolute">Absolute UTC</option><option value="after">After aircraft</option></select>
      {mode === "absolute" ? <input aria-label="Closure start" className={field} type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)} /> : <select aria-label="Closure anchor aircraft" className={field} value={after} onChange={(event) => setAfter(event.target.value)}><option value="">Select aircraft</option>{anchors.map((flight) => <option key={flight.flight_id} value={flight.flight_id}>{flight.callsign}</option>)}</select>}
      <input aria-label="Closure end; blank means indefinite" className={field} type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)} />
      <input aria-label="Closure reason" className={field} placeholder="Reason (also required to remove)" value={reason} onChange={(event) => setReason(event.target.value)} />
      <button className={button} disabled={disabled || busy || !valid} onClick={send}>Insert closure</button>
    </div>
    {busy && <p aria-live="polite" role="status" className="text-sm text-sky-200">Closure change pending; displayed overlays remain server-confirmed.</p>}
  </section>;
}
