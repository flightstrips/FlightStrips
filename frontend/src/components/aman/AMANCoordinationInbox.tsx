import {useEffect, useRef, useState} from "react";

import type {AMANCoordinationRequest, AMANFlight} from "@/api/aman";
import {Dialog, DialogContent, DialogDescription, DialogTitle} from "@/components/ui/dialog";

const button = "rounded border border-slate-400 px-3 py-1.5 text-sm font-semibold hover:bg-slate-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:cursor-not-allowed disabled:opacity-50";

function value(request: AMANCoordinationRequest) {
  if (request.kind === "speed") return request.payload.speed?.requested ?? "Unavailable";
  const route = request.payload.route_direct;
  return [route?.route && `Route ${route.route}`, route?.direct_to && `Direct ${route.direct_to}`].filter(Boolean).join(" · ") || "Unavailable";
}

export function AMANCoordinationInbox({requests, flights, canDecide, deciding, rejection, onDecision}: {
  requests: AMANCoordinationRequest[];
  flights: AMANFlight[];
  canDecide: boolean;
  deciding: boolean;
  rejection?: string | null;
  onDecision: (requestID: string, decision: "accept" | "reject", reason?: string) => void;
}) {
  const pending = requests.filter((request) => request.state === "pending");
  const [selected, setSelected] = useState<AMANCoordinationRequest | null>(null);
  const [decision, setDecision] = useState<"accept" | "reject" | null>(null);
  const [reason, setReason] = useState("");
  const returnFocus = useRef<HTMLElement | null>(null);
  const current = selected && pending.find((request) => request.id === selected.id);

  useEffect(() => {
    if (selected && !current) returnFocus.current?.focus();
  }, [current, selected]);

  const open = (request: AMANCoordinationRequest) => {
    returnFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setSelected(request); setDecision(null); setReason("");
  };
  const close = () => {
    setSelected(null); setDecision(null); setReason("");
    queueMicrotask(() => returnFocus.current?.focus());
  };

  return <section aria-label="Coordination inbox" className="rounded border border-slate-600 bg-slate-900/70 p-3 text-slate-100">
    <div className="flex items-center justify-between gap-3"><div><h2 className="font-semibold">Coordination inbox</h2><p aria-live="polite" className="text-xs text-slate-300">{pending.length} pending request{pending.length === 1 ? "" : "s"}</p></div></div>
    {pending.length === 0 ? <p className="mt-2 text-sm text-slate-400">No requests assigned to this tracking position.</p> : <ul className="mt-3 grid gap-2">{pending.map((request) => {
      const callsign = flights.find((flight) => flight.flight_id === request.flight_id)?.callsign ?? request.flight_id;
      return <li className="flex items-center justify-between gap-3 rounded border border-slate-700 p-2 text-sm" key={request.id}><span><strong>{callsign} · {request.kind === "speed" ? "Speed" : "Route / direct"}</strong><br />{value(request)} · <span>Pending</span></span><button className={button} onClick={() => open(request)} type="button">Review</button></li>;
    })}</ul>}
    {selected && current && <Dialog onOpenChange={(isOpen) => !isOpen && close()} open>
      <DialogContent className="max-w-md border-slate-500 bg-[#161d27] text-slate-100">
        <DialogTitle>Review coordination request</DialogTitle>
        <DialogDescription className="text-slate-300">Only the current authoritative tracking controller may decide. Acceptance records agreement only; it does not issue a clearance or alter the prediction.</DialogDescription>
        <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 rounded border border-slate-600 p-3 text-sm"><dt>Aircraft</dt><dd>{flights.find((flight) => flight.flight_id === selected.flight_id)?.callsign ?? selected.flight_id}</dd><dt>Kind</dt><dd>{selected.kind === "speed" ? "Speed" : "Route / direct"}</dd><dt>Request</dt><dd>{value(selected)}</dd><dt>Status</dt><dd>Pending</dd></dl>
        {!canDecide && <p role="status" className="rounded border border-amber-500 bg-amber-950 p-2 text-sm">Decision controls are read-only without a connected authoritative AMAN session.</p>}
        {rejection && <p role="alert" className="rounded border border-red-500 bg-red-950 p-2 text-sm">Decision rejected by server: {rejection}</p>}
        {decision === "reject" && <label className="grid gap-1 text-sm" htmlFor="coordination-rejection-reason">Rejection reason<input autoFocus className="rounded border border-slate-500 bg-slate-950 px-3 py-2 text-white" id="coordination-rejection-reason" onChange={(event) => setReason(event.target.value)} value={reason} /></label>}
        <div className="flex justify-end gap-2"><button className={button} onClick={close} type="button">Cancel</button>{decision === null ? <><button className={button} disabled={!canDecide || deciding} onClick={() => setDecision("reject")} type="button">Reject…</button><button className={button} disabled={!canDecide || deciding} onClick={() => onDecision(selected.id, "accept")} type="button">Accept agreement</button></> : <button className={button} disabled={!canDecide || deciding || !reason.trim()} onClick={() => onDecision(selected.id, "reject", reason.trim())} type="button">{deciding ? "Rejecting…" : "Confirm rejection"}</button>}</div>
      </DialogContent>
    </Dialog>}
  </section>;
}
