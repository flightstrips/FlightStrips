import {useState} from "react";

import {canSwapAMANFlight, getAMANSwapBlockReason, type AMANCommandIntent, type AMANFlight, type AMANMutationBlockReason} from "@/api/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";

export function AMANSwapDialog({flight, flights, mutationBlockReason, busy, rejection, onClose, onCommand}: {
  flight: AMANFlight;
  flights: AMANFlight[];
  mutationBlockReason: AMANMutationBlockReason | null;
  busy: boolean;
  rejection: string | null;
  onClose: () => void;
  onCommand: (intent: AMANCommandIntent) => void;
}) {
  const [otherCallsign, setOtherCallsign] = useState("");
  const candidates = flights.filter((other) => canSwapAMANFlight(other)
    && other.callsign !== flight.callsign && other.star_family === flight.star_family
    && other.runway_group_id === flight.runway_group_id)
    .sort((a, b) => a.slot!.sequence - b.slot!.sequence);
  const other = candidates.find((candidate) => candidate.callsign === otherCallsign) ?? null;
  const flightBlockReason = getAMANSwapBlockReason(flight, mutationBlockReason);
  const selectedFlight = flights.find((candidate) => candidate.callsign === otherCallsign);
  const selectionBlockReason = otherCallsign && other === null
    ? selectedFlight
      ? `${otherCallsign}: ${getAMANSwapBlockReason(selectedFlight) ?? "This aircraft is no longer on the same STAR and runway."}`
      : `${otherCallsign} is no longer in the arrival sequence.`
    : null;
  const blockReason = flightBlockReason
    ?? (busy ? "Waiting for the updated sequence…" : null)
    ?? selectionBlockReason
    ?? (candidates.length === 0 ? "No eligible aircraft on the same STAR and runway are available to swap with." : null)
    ?? (other === null ? "Select an aircraft to swap with." : null);

  return <Dialog onOpenChange={(open) => { if (!open) onClose(); }} open>
    <DialogContent className="w-[min(28rem,calc(100vw-1rem))] border-2 border-[#dcdcdc] bg-[#5174b8] font-display text-sm text-white">
      <DialogTitle>Swap sequence · {flight.callsign}</DialogTitle>
      <p>Exchange sequence positions with another aircraft on {flight.star_family ?? "this STAR"}, runway {flight.runway_group_id ?? "unavailable"}. Spacing may change the assigned times.</p>
      <p>Automatic superstable slots can be reassigned. Manual and TMA freezes remain protected.</p>
      <label className="grid gap-1">
        Swap with
        <select aria-describedby={blockReason ? "aman-swap-dialog-reason" : undefined} autoFocus className="rounded border border-[#dcdcdc] bg-[#244477] p-2 text-white disabled:opacity-50" disabled={flightBlockReason !== null || busy || candidates.length === 0} onChange={(event) => setOtherCallsign(event.target.value)} value={other?.callsign ?? ""}>
          <option value="">Select aircraft</option>
          {candidates.map((candidate) => <option key={candidate.callsign} value={candidate.callsign}>{candidate.callsign} · #{candidate.slot!.sequence} · {candidate.slot!.time.slice(11, 16)} UTC</option>)}
        </select>
      </label>
      {other && <p>#{flight.slot?.sequence} {flight.callsign} ↔ #{other.slot?.sequence} {other.callsign}</p>}
      {blockReason && <p id="aman-swap-dialog-reason" role="status">{blockReason}</p>}
      {rejection && <p className="rounded border border-red-200 bg-red-950 p-2" role="alert">{rejection}</p>}
      <div className="flex justify-end gap-2">
        <button className="rounded border border-[#dcdcdc] px-3 py-1" onClick={onClose} type="button">Close</button>
        <button aria-describedby={blockReason ? "aman-swap-dialog-reason" : undefined} className="rounded border border-[#dcdcdc] bg-[#a3d5e8] px-3 py-1 font-bold text-[#10265c] disabled:opacity-40" disabled={blockReason !== null} onClick={() => {
          if (blockReason !== null || other === null) return;
          onCommand({type: "aman.swap_flights", callsign: flight.callsign, other_callsign: other.callsign});
        }} type="button">Swap aircraft</button>
      </div>
    </DialogContent>
  </Dialog>;
}
