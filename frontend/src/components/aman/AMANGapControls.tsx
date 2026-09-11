import {useState} from "react";
import type {AMANCommandIntent, AMANCommandRejection, AMANPendingCommand, AMANRunwayGroup} from "@/api/aman";
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";

const field = "rounded border border-slate-500 bg-slate-950 px-2 py-1 text-sm text-white";
const button = "rounded border border-slate-500 bg-slate-700 px-2 py-1 text-sm text-white disabled:cursor-not-allowed disabled:opacity-40";
const utc = (value: string) => { const parsed = new Date(`${value}Z`); return Number.isNaN(parsed.valueOf()) ? null : parsed.toISOString(); };
const shown = (value: string) => new Date(value).toISOString().slice(11, 16);

export function AMANGapControls({disabled, groups, pending, rejections, onCommand}: {
  disabled: boolean;
  groups: AMANRunwayGroup[];
  pending: AMANPendingCommand[];
  rejections: AMANCommandRejection[];
  onCommand: (intent: AMANCommandIntent) => void;
}) {
  const [open, setOpen] = useState(false);
  const [runway, setRunway] = useState("");
  const [mode, setMode] = useState<"" | "absolute" | "slots">("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [slots, setSlots] = useState("");
  const [label, setLabel] = useState("");
  const selected = groups.some((group) => group.id === runway) ? runway : groups[0]?.id ?? "";
  const busy = pending.some((command) => command.type === "aman.create_gap" || command.type === "aman.remove_gap");
  const gapRejections = rejections.filter((item) => item.command_type === "aman.create_gap" || item.command_type === "aman.remove_gap");
  const valid = Boolean(selected && label.trim() && utc(start) && (mode === "absolute" ? utc(end) && Date.parse(end) > Date.parse(start) : mode === "slots" && Number.isInteger(Number(slots)) && Number(slots) > 0));
  return <section aria-label="Runway GAP controls" className="grid gap-2 rounded border border-amber-400/70 bg-amber-950/20 p-3">
    <div className="flex items-center justify-between gap-2"><div><h3 className="font-semibold">Runway GAPs</h3><p className="text-xs text-amber-100">Manual protected capacity; not aircraft or automatic spacing.</p></div><button className={button} disabled={disabled} onClick={() => setOpen(true)}>Manage GAPs…</button></div>
    {groups.flatMap((group) => (group.gaps ?? []).map((gap) => <div className="flex items-center gap-2 text-sm" key={gap.id}>
      <span className="grow"><b>Server-confirmed union:</b> {group.id} {shown(gap.start)}–{shown(gap.end)} UTC [start,end) · {gap.label} · ID {gap.id} · by {gap.created_by}</span>
      <button aria-label={`Remove GAP ${gap.label} from ${group.id}`} className={button} disabled={disabled || busy} onClick={() => onCommand({type: "aman.remove_gap", runway_group_id: group.id, gap_id: gap.id})}>Remove</button>
    </div>))}
    {groups.every((group) => group.gaps === undefined) && <p className="text-xs text-slate-300">GAP state unavailable from this older server.</p>}
    {busy && <p aria-live="polite" role="status" className="text-sm text-sky-200">GAP change pending; waiting for a server-confirmed revision.</p>}
    <Dialog open={open} onOpenChange={setOpen}><DialogContent className="w-[32rem] max-w-[calc(100vw-2rem)] border-slate-600 bg-slate-900 text-slate-100">
      <DialogHeader><DialogTitle>Create runway GAP</DialogTitle></DialogHeader>
      <div className="grid gap-3 text-sm">
        <p role="note" className="rounded border border-amber-500 p-2 text-amber-100">Creation atomically displaces all affected flights, including protected traffic. A rejected displacement changes nothing.</p>
        <label>Runway group<select autoFocus aria-label="GAP runway group" className={`${field} ml-2`} onChange={(event) => setRunway(event.target.value)} value={selected}>{groups.map((group) => <option key={group.id}>{group.id}</option>)}</select></label>
        <label>Input mode<select aria-label="GAP input mode" className={`${field} ml-2`} onChange={(event) => setMode(event.target.value as typeof mode)} value={mode}><option value="">Select input mode</option><option value="absolute">Absolute UTC interval</option><option value="slots">Start plus slot count</option></select></label>
        <label>Start (UTC)<input aria-label="GAP start UTC" className={`${field} ml-2`} onChange={(event) => setStart(event.target.value)} type="datetime-local" value={start} /></label>
        {mode === "absolute" && <label>End, exclusive (UTC)<input aria-label="GAP end UTC exclusive" className={`${field} ml-2`} onChange={(event) => setEnd(event.target.value)} type="datetime-local" value={end} /></label>}
        {mode === "slots" && <label>Slot count<input aria-label="GAP slot count" className={`${field} ml-2`} min="1" onChange={(event) => setSlots(event.target.value)} type="number" value={slots} /></label>}
        <label>Operational reason<input aria-label="GAP operational reason" className={`${field} ml-2`} onChange={(event) => setLabel(event.target.value)} value={label} /></label>
        {gapRejections.map((item) => <p key={item.command_id} role="alert" className="rounded border border-red-500 bg-red-950 p-2">GAP rejected: {item.message} ({item.code}; server revision {item.current_revision}). No displacement was applied.</p>)}
      </div>
      <DialogFooter><button className={button} disabled={disabled || busy || !valid} onClick={() => {
        const startAt = utc(start); if (!startAt) return;
        onCommand(mode === "absolute" ? {type: "aman.create_gap", runway_group_id: selected, start: startAt, end: utc(end)!, label: label.trim()} : {type: "aman.create_gap", runway_group_id: selected, start: startAt, slot_count: Number(slots), label: label.trim()});
      }}>Create GAP</button></DialogFooter>
    </DialogContent></Dialog>
  </section>;
}
