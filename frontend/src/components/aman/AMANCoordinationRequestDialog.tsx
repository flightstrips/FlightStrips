import {useEffect, useMemo, useRef, useState} from "react";

import type {AMANCoordinationKind, AMANCoordinationRequest} from "@/api/aman";
import {Dialog, DialogContent, DialogDescription, DialogTitle} from "@/components/ui/dialog";

type Submission =
  | {kind: "route_direct"; route?: string; direct_to?: string}
  | {kind: "speed"; requested: string};

const inputClass = "w-full rounded border border-slate-500 bg-slate-950 px-3 py-2 text-sm text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-cyan-300 disabled:opacity-60";
const buttonClass = "rounded border border-slate-400 px-3 py-2 text-sm font-semibold hover:bg-slate-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:cursor-not-allowed disabled:opacity-50";

function requestValue(request: AMANCoordinationRequest): string {
  if (request.kind === "speed") return request.payload.speed?.requested ?? "Unavailable";
  const route = request.payload.route_direct;
  return [route?.route && `Route ${route.route}`, route?.direct_to && `Direct ${route.direct_to}`].filter(Boolean).join(" · ") || "Unavailable";
}

export function AMANCoordinationRequestDialog({callsign, requests, canSubmit, submitting, rejection, onSubmit, onClose}: {
  callsign: string;
  requests: AMANCoordinationRequest[];
  canSubmit: boolean;
  submitting: boolean;
  rejection?: string | null;
  onSubmit: (submission: Submission) => void;
  onClose: () => void;
}) {
  const returnFocusRef = useRef(document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const [kind, setKind] = useState<AMANCoordinationKind>("route_direct");
  const [route, setRoute] = useState("");
  const [directTo, setDirectTo] = useState("");
  const [speed, setSpeed] = useState("");
  const relevant = useMemo(() => [...requests].sort((a, b) => b.created_at.localeCompare(a.created_at)), [requests]);
  const pending = relevant.find((request) => request.kind === kind && request.state === "pending");
  const valid = kind === "speed" ? speed.trim() !== "" : route.trim() !== "" || directTo.trim() !== "";
  useEffect(() => () => returnFocusRef.current?.focus(), []);

  return <Dialog onOpenChange={(open) => !open && onClose()} open>
    <DialogContent className="max-h-[calc(100dvh-2rem)] max-w-md overflow-y-auto border-slate-500 bg-[#161d27] text-slate-100">
      <DialogTitle>Coordinate {callsign}</DialogTitle>
      <DialogDescription className="text-slate-300">Send a request to the aircraft&apos;s authoritative tracking controller. Agreement is not a clearance and does not change the prediction.</DialogDescription>
      <form className="grid gap-4" onSubmit={(event) => {
        event.preventDefault();
        if (!valid || !canSubmit || submitting) return;
        onSubmit(kind === "speed" ? {kind, requested: speed.trim()} : {
          kind, ...(route.trim() ? {route: route.trim().toUpperCase()} : {}), ...(directTo.trim() ? {direct_to: directTo.trim().toUpperCase()} : {}),
        });
      }}>
        <fieldset className="grid grid-cols-2 gap-2" disabled={submitting}>
          <legend className="mb-2 text-sm font-semibold">Request kind</legend>
          <label className="flex items-center gap-2 rounded border border-slate-600 p-2"><input checked={kind === "route_direct"} name="request-kind" onChange={() => setKind("route_direct")} type="radio" /> Route / direct</label>
          <label className="flex items-center gap-2 rounded border border-slate-600 p-2"><input checked={kind === "speed"} name="request-kind" onChange={() => setKind("speed")} type="radio" /> Speed</label>
        </fieldset>
        {kind === "route_direct" ? <div className="grid gap-3">
          <label className="grid gap-1 text-sm" htmlFor="coordination-route">Requested route<input autoFocus className={inputClass} disabled={submitting} id="coordination-route" onChange={(event) => setRoute(event.target.value)} value={route} /></label>
          <label className="grid gap-1 text-sm" htmlFor="coordination-direct">Direct to<input className={inputClass} disabled={submitting} id="coordination-direct" onChange={(event) => setDirectTo(event.target.value)} value={directTo} /></label>
          <p className="text-xs text-slate-400">Enter a route, a direct-to fix, or both.</p>
        </div> : <label className="grid gap-1 text-sm" htmlFor="coordination-speed">Requested speed<input autoFocus className={inputClass} disabled={submitting} id="coordination-speed" onChange={(event) => setSpeed(event.target.value)} placeholder="e.g. 250 KT or M0.78" value={speed} /></label>}
        <div aria-live="polite" className="min-h-10 rounded border border-slate-600 bg-slate-900 p-2 text-sm">
          Recipient: <strong>{pending?.recipient_status === "assigned" ? pending.recipient_controller : pending ? "Unassigned — no tracking controller" : "Resolved authoritatively on submit"}</strong>
          {pending && <p className="mt-1 text-amber-300">Submitting replaces the existing pending {kind === "speed" ? "speed" : "route/direct"} request.</p>}
        </div>
        {rejection && <p className="rounded border border-red-500 bg-red-950 p-2 text-sm" role="alert">Request rejected by server: {rejection}</p>}
        {!canSubmit && <p className="rounded border border-amber-500 bg-amber-950 p-2 text-sm" role="status">Requests are read-only without connected authoritative FMP access.</p>}
        <div className="flex justify-end gap-2"><button className={buttonClass} onClick={onClose} type="button">Cancel</button><button className={buttonClass} disabled={!valid || !canSubmit || submitting} type="submit">{submitting ? "Submitting…" : pending ? "Replace request" : "Send request"}</button></div>
      </form>
      <section aria-label="Request history" className="border-t border-slate-600 pt-4"><h3 className="mb-2 font-semibold">Request history</h3>{relevant.length === 0 ? <p className="text-sm text-slate-400">No requests for this flight.</p> : <ol className="grid gap-2">{relevant.map((request) => <li className="rounded border border-slate-700 p-2 text-sm" key={request.id}><strong>{request.kind === "speed" ? "Speed" : "Route / direct"}: {request.state}</strong><br />{requestValue(request)}<br /><span className="text-slate-400">Recipient: {request.recipient_status === "assigned" ? request.recipient_controller : "Unassigned"}</span>{request.state === "accepted" && <p className="mt-1 text-cyan-200">Agreed only — awaiting authoritative clearance.</p>}</li>)}</ol>}</section>
    </DialogContent>
  </Dialog>;
}
