import {useState} from "react";
import type {AMANCommandIntent, AMANCommandRejection, AMANFlight, AMANPendingCommand, AMANRunwayGroup} from "@/api/aman";

const field = "rounded border border-slate-500 bg-slate-950 px-2 py-1 text-sm text-white";
const button = "rounded border border-slate-500 bg-slate-700 px-2 py-1 text-sm text-white disabled:opacity-40";
const shown = (value: string) => new Date(value).toISOString().slice(11, 16);

export function AMANCapacityControls({disabled, groups, flights, pending, rejections, onCommand}: {disabled: boolean; groups: AMANRunwayGroup[]; flights: AMANFlight[]; pending: AMANPendingCommand[]; rejections: AMANCommandRejection[]; onCommand: (intent: AMANCommandIntent) => void}) {
  const [runway, setRunway] = useState(""), [after, setAfter] = useState(""), [label, setLabel] = useState(""), [reason, setReason] = useState("");
  const selected = groups.some((group) => group.id === runway) ? runway : groups[0]?.id ?? "";
  const anchors = flights.filter((flight) => flight.runway_group_id === selected && flight.slot !== null);
  const anchor = anchors.find((flight) => flight.flight_id === after);
  const protectedFollowing = anchor && anchors.some((flight) => flight.slot!.time > anchor.slot!.time && (flight.freeze_reason !== "none" || flight.lifecycle_state === "stable"));
  const busy = pending.some((item) => item.type.includes("capacity_reservation"));
  const rejected = rejections.filter((item) => item.command_type?.includes("capacity_reservation"));
  return <section aria-label="Extra Flight capacity controls" className="grid gap-2 rounded border border-cyan-400/70 bg-cyan-950/20 p-3">
    <div><h3 className="font-semibold">Extra Flight capacity</h3><p className="text-xs text-cyan-100">Reserves capacity only; no AMAN flight or surveillance target is created.</p></div>
    {groups.flatMap((group) => (group.capacity_reservations ?? []).map((item) => <div className="flex gap-2 text-sm" key={item.id}><span className="grow"><b>Server-confirmed immutable interval:</b> {group.id} {shown(item.start)}–{shown(item.end)} UTC [start,end) · {item.label || "FLIGHT"}</span><button aria-label={`Remove Extra Flight ${item.label} from ${group.id}`} className={button} disabled={disabled || busy || !reason.trim()} onClick={() => onCommand({type: "aman.remove_capacity_reservation", runway_group_id: group.id, reservation_id: item.id, reason: reason.trim()})}>Remove</button></div>))}
    <div className="flex flex-wrap gap-2"><select aria-label="Extra Flight runway" className={field} value={selected} onChange={(event) => setRunway(event.target.value)}>{groups.map((group) => <option key={group.id}>{group.id}</option>)}</select><select aria-label="Insert Extra Flight after" className={field} value={after} onChange={(event) => setAfter(event.target.value)}><option value="">Select anchor aircraft</option>{anchors.map((flight) => <option key={flight.flight_id} value={flight.flight_id}>{flight.callsign}</option>)}</select><input aria-label="Extra Flight label" className={field} placeholder="FLIGHT (default)" value={label} onChange={(event) => setLabel(event.target.value)} /><input aria-label="Extra Flight reason" className={field} value={reason} onChange={(event) => setReason(event.target.value)} /><button className={button} disabled={disabled || busy || !after || !reason.trim()} onClick={() => onCommand({type: "aman.create_capacity_reservation", runway_group_id: selected, after_flight_id: after, ...(label.trim() ? {label: label.trim()} : {}), reason: reason.trim()})}>Insert Extra Flight</button></div>
    {protectedFollowing && <p role="alert" className="text-sm text-amber-200">Warning: insertion can atomically displace protected traffic; rejection changes nothing.</p>}
    {busy && <p aria-live="polite" role="status" className="text-sm text-sky-200">Extra Flight change pending; overlays remain server-confirmed.</p>}
    {rejected.map((item) => <p role="alert" key={item.command_id} className="text-sm text-red-200">Extra Flight rejected: {item.message} ({item.code}; server revision {item.current_revision}).</p>)}
  </section>;
}
