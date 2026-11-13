import {useState} from "react";
import type {AMANCommandIntent, AMANCommandRejection, AMANFlight, AMANPendingCommand, AMANState} from "@/api/aman";
import {canInsertAMANFlight, getActiveAMANRunwayGroups} from "@/api/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";

export function AMANInsertBehindDialog({flight, state, disabled, pendingCommands, commandRejections, onCommand, onClose}: {
  flight: AMANFlight;
  state: AMANState;
  disabled: boolean;
  pendingCommands: Record<string, AMANPendingCommand>;
  commandRejections: Record<string, AMANCommandRejection>;
  onCommand: (intent: AMANCommandIntent) => string | null;
  onClose: () => void;
}) {
  const [anchorCallsign, setAnchorCallsign] = useState("");
  const [starFamily, setStarFamily] = useState("");
  const [submission, setSubmission] = useState<{id: string; revision: number; anchor: string} | null>(null);
  const active = new Set(getActiveAMANRunwayGroups(state).map(group => group.id));
  const family = flight.star_selection_required ? null : flight.controller_star_family ?? flight.star_family;
  const anchors = state.flights.filter(other => other.callsign !== flight.callsign && other.slot
    && active.has(other.slot.runway_group_id) && other.lifecycle_state !== "landed" && other.lifecycle_state !== "removed"
    && other.sequence_disposition !== "desequenced" && !other.expedited
    && Date.parse(other.slot.time) >= Date.parse(state.generated_at)
    && (!flight.slot || flight.runway_group_id === other.slot.runway_group_id)
    && (!family || !(state.insertion_star_families?.[other.slot.runway_group_id]?.length)
      || state.insertion_star_families[other.slot.runway_group_id].includes(family)))
    .sort((a, b) => a.slot!.runway_group_id.localeCompare(b.slot!.runway_group_id) || a.slot!.sequence - b.slot!.sequence);
  const anchor = anchors.find(other => other.callsign === anchorCallsign);
  const options = anchor ? state.insertion_star_families?.[anchor.slot!.runway_group_id] ?? [] : [];
  const needsSTAR = !family || options.length > 0 && !options.includes(family);
  const selectedSTAR = options.includes(starFamily) ? starFamily : "";
  const pending = submission !== null && pendingCommands[submission.id] !== undefined;
  const rejection = submission ? commandRejections[submission.id]?.message : undefined;
  const committedTarget = state.flights.find(other => other.callsign === flight.callsign)?.slot;
  const committedAnchor = state.flights.find(other => other.callsign === submission?.anchor)?.slot;
  const placementApplied = Boolean(committedTarget && committedAnchor && committedTarget.runway_group_id === committedAnchor.runway_group_id && committedTarget.sequence > committedAnchor.sequence);
  const confirmed = Boolean(submission && state.revision > submission.revision && !pending && !rejection && placementApplied);
  const eligible = canInsertAMANFlight(flight);
  return <Dialog open onOpenChange={open => { if (!open) onClose(); }}>
    <DialogContent className="w-[min(30rem,calc(100vw-1rem))] border-2 border-[#dcdcdc] bg-[#5174b8] font-display text-sm text-white [&>button]:hidden">
      <DialogTitle>Insert behind · {flight.callsign}</DialogTitle>
      <p>Choose an aircraft that must land before {flight.callsign}. AMAN calculates the times and may put traffic from another STAR between them. Following traffic, including protected flights, may move later.</p>
      <label className="grid gap-1">Behind aircraft
        <select autoFocus aria-label="Behind aircraft" className="rounded border bg-[#244477] p-2" disabled={disabled || pending || confirmed || !eligible} value={anchor?.callsign ?? ""} onChange={event => setAnchorCallsign(event.target.value)}>
          <option value="">Select aircraft</option>
          {anchors.map(other => <option key={other.callsign} value={other.callsign}>{other.callsign} · {other.star_family ?? "STAR unknown"} · {other.slot!.runway_group_id} · {other.slot!.time.slice(11, 16)} UTC</option>)}
        </select>
      </label>
      {anchor && needsSTAR && <label className="grid gap-1">STAR entry
        <select aria-label="STAR entry" className="rounded border bg-[#244477] p-2" disabled={disabled || pending || confirmed} value={selectedSTAR} onChange={event => setStarFamily(event.target.value)}>
          <option value="">Select STAR entry</option>
          {options.map(family => <option key={family} value={family}>{family}</option>)}
        </select>
      </label>}
      {!eligible && <p role="status">Current airborne aircraft data is required for insertion.</p>}
      {anchors.length === 0 && <p role="status">No compatible sequenced aircraft is available.</p>}
      {anchor && needsSTAR && options.length === 0 && <p role="status">Configured STAR entries are unavailable for this runway.</p>}
      {disabled && <p role="status">Sequence changes are currently unavailable.</p>}
      {pending && <p role="status">Waiting for the updated sequence…</p>}
      {confirmed && <p role="status">Server confirmed insertion and updated the sequence.</p>}
      {rejection && <p role="alert" className="rounded bg-red-950 p-2">{rejection}</p>}
      <div className="flex justify-end gap-2">
        <button className="rounded border px-3 py-1" onClick={onClose} type="button">Close</button>
        <button className="rounded border bg-[#a3d5e8] px-3 py-1 font-bold text-[#10265c] disabled:opacity-40" disabled={disabled || pending || confirmed || !eligible || !anchor || needsSTAR && !selectedSTAR} onClick={() => {
          if (!anchor) return;
          const id = onCommand({type: "aman.insert_flight_after", callsign: flight.callsign, after_callsign: anchor.callsign, ...(needsSTAR ? {star_family: selectedSTAR} : {})});
          if (id) setSubmission({id, revision: state.revision, anchor: anchor.callsign});
        }} type="button">Insert behind {anchor?.callsign ?? "aircraft"}</button>
      </div>
    </DialogContent>
  </Dialog>;
}
