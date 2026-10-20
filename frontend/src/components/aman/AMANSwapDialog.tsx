import {useState} from "react";

import {canSwapAMANFlight, type AMANCommandIntent, type AMANFlight} from "@/api/aman";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";

export function AMANSwapDialog({flight, flights, disabled, busy, rejection, onClose, onCommand}: {
  flight: AMANFlight;
  flights: AMANFlight[];
  disabled: boolean;
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
  const eligible = canSwapAMANFlight(flight);

  return <Dialog onOpenChange={(open) => { if (!open) onClose(); }} open>
    <DialogContent className="w-[min(28rem,calc(100vw-1rem))] border-2 border-[#dcdcdc] bg-[#5174b8] font-display text-sm text-white">
      <DialogTitle>Swap sequence · {flight.callsign}</DialogTitle>
      <p>Exchange sequence positions with another aircraft on {flight.star_family ?? "this STAR"}, runway {flight.runway_group_id ?? "unavailable"}. Spacing may change the assigned times.</p>
      <label className="grid gap-1">
        Swap with
        <select autoFocus className="rounded border border-[#dcdcdc] bg-[#244477] p-2 text-white disabled:opacity-50" disabled={disabled || busy || !eligible} onChange={(event) => setOtherCallsign(event.target.value)} value={other?.callsign ?? ""}>
          <option value="">Select aircraft</option>
          {candidates.map((candidate) => <option key={candidate.callsign} value={candidate.callsign}>{candidate.callsign} · #{candidate.slot!.sequence} · {candidate.slot!.time.slice(11, 16)} UTC</option>)}
        </select>
      </label>
      {other && <p>#{flight.slot?.sequence} {flight.callsign} ↔ #{other.slot?.sequence} {other.callsign}</p>}
      {(!eligible || candidates.length === 0) && <p role="status">Two sequenced, unfrozen aircraft on the same STAR and runway are required.</p>}
      {disabled && <p role="status">Sequence changes are currently unavailable.</p>}
      {busy && <p role="status">Waiting for the updated sequence…</p>}
      {rejection && <p className="rounded border border-red-200 bg-red-950 p-2" role="alert">{rejection}</p>}
      <div className="flex justify-end gap-2">
        <button className="rounded border border-[#dcdcdc] px-3 py-1" onClick={onClose} type="button">Close</button>
        <button className="rounded border border-[#dcdcdc] bg-[#a3d5e8] px-3 py-1 font-bold text-[#10265c] disabled:opacity-40" disabled={disabled || busy || !eligible || other === null} onClick={() => {
          if (!eligible || other === null || disabled || busy) return;
          onCommand({type: "aman.swap_flights", callsign: flight.callsign, other_callsign: other.callsign});
        }} type="button">Swap aircraft</button>
      </div>
    </DialogContent>
  </Dialog>;
}
