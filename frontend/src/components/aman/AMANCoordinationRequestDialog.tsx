import {useEffect, useMemo, useRef, useState} from "react";

import type {AMANCoordinationKind, AMANCoordinationRequest} from "@/api/aman";
import {Dialog, DialogContent, DialogDescription, DialogTitle} from "@/components/ui/dialog";

type Submission =
  | {kind: "route_direct"; route?: string; direct_to?: string}
  | {kind: "speed"; requested: string};

const inputClass = "w-full border-2 border-[#dcdcdc] bg-[#d6d6d6] px-3 py-2 text-sm font-bold text-black focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:opacity-60";
const buttonClass = "border border-[#dcdcdc] bg-[#6b7f9f] px-3 py-2 text-sm font-bold uppercase hover:bg-[#a3d5e8] focus-visible:outline focus-visible:outline-2 focus-visible:outline-white disabled:cursor-not-allowed disabled:opacity-50";

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
    <DialogContent className="max-h-[calc(100dvh-2rem)] w-[min(370px,calc(100vw-2rem))] max-w-none gap-0 overflow-y-auto rounded-none border-2 border-[#dcdcdc] bg-[#5174b8] p-0 font-display text-white [&>button]:hidden">
      <DialogTitle className="border-b-2 border-[#dcdcdc] px-4 py-3 text-center text-lg font-bold uppercase"><span aria-hidden="true">Coordination</span><span className="sr-only">Coordinate {callsign}</span></DialogTitle>
      <DialogDescription className="sr-only">Send a request to the aircraft&apos;s authoritative tracking controller. Agreement is not a clearance and does not change the prediction.</DialogDescription>
      <form className="grid gap-4 px-5 py-4" onSubmit={(event) => {
        event.preventDefault();
        if (!valid || !canSubmit || submitting) return;
        onSubmit(kind === "speed" ? {kind, requested: speed.trim()} : {
          kind, ...(route.trim() ? {route: route.trim().toUpperCase()} : {}), ...(directTo.trim() ? {direct_to: directTo.trim().toUpperCase()} : {}),
        });
      }}>
        <fieldset className="grid grid-cols-2 gap-2" disabled={submitting}>
          <legend className="sr-only">Request kind</legend>
          <label className="flex items-center gap-2 border border-[#dcdcdc] bg-[#6b7f9f] p-2 uppercase"><input checked={kind === "route_direct"} name="request-kind" onChange={() => setKind("route_direct")} type="radio" /> Route / direct</label>
          <label className="flex items-center gap-2 border border-[#dcdcdc] bg-[#6b7f9f] p-2 uppercase"><input checked={kind === "speed"} name="request-kind" onChange={() => setKind("speed")} type="radio" /> Speed</label>
        </fieldset>
        {kind === "route_direct" ? <div className="grid gap-3">
          <label className="grid gap-1 text-sm font-bold" htmlFor="coordination-route">Request Routing<input autoFocus className={inputClass} disabled={submitting} id="coordination-route" onChange={(event) => setRoute(event.target.value)} value={route} /></label>
          <label className="grid gap-1 text-sm font-bold" htmlFor="coordination-direct">Direct to<input className={inputClass} disabled={submitting} id="coordination-direct" onChange={(event) => setDirectTo(event.target.value)} value={directTo} /></label>
        </div> : <label className="grid gap-1 text-sm font-bold" htmlFor="coordination-speed">Request Speed<input aria-label="Requested speed" autoFocus className={inputClass} disabled={submitting} id="coordination-speed" onChange={(event) => setSpeed(event.target.value)} placeholder="250 KT or M0.78" value={speed} /></label>}
        <div aria-live="polite" className="min-h-10 border border-[#dcdcdc] bg-[#6b7f9f] p-2 text-sm">
          Recipient: <strong>{pending?.recipient_status === "assigned" ? pending.recipient_controller : pending ? "Unassigned — no tracking controller" : "Resolved authoritatively on submit"}</strong>
          {pending && <p className="mt-1 text-amber-300">Submitting replaces the existing pending {kind === "speed" ? "speed" : "route/direct"} request.</p>}
        </div>
        {rejection && <p className="border border-amber-200 p-2 text-sm" role="alert">Request rejected by server: {rejection}</p>}
        {!canSubmit && <p className="border border-amber-200 p-2 text-sm" role="status">Requests are read-only without connected authoritative FMP access.</p>}
        <div className="grid grid-cols-2 gap-2"><button aria-label={submitting ? "Submitting…" : pending ? "Replace request" : "Send request"} className={buttonClass} disabled={!valid || !canSubmit || submitting} type="submit">{submitting ? "Submitting…" : "Send"}</button><button aria-label="Cancel" className={buttonClass} onClick={onClose} type="button">ESC</button></div>
      </form>
	  {relevant.length > 0 && <section aria-label="Request history" className="border-t-2 border-[#dcdcdc] px-5 py-3"><h3 className="mb-2 font-semibold">Request history</h3><ol className="grid gap-2">{relevant.map((request) => <li className="border border-[#dcdcdc] bg-[#6b7f9f] p-2 text-sm" key={request.id}><strong>{request.kind === "speed" ? "Speed" : "Route / direct"}: {request.state}</strong><br />{requestValue(request)}<br /><span>Recipient: {request.recipient_status === "assigned" ? request.recipient_controller : "Unassigned"}</span>{request.clearance ? <p className="mt-1" role="status">Authoritative clearance observed: {request.clearance.value} · {request.clearance.issuer}</p> : request.state === "accepted" && <p className="mt-1">Agreed only — awaiting authoritative clearance.</p>}</li>)}</ol></section>}
    </DialogContent>
  </Dialog>;
}
