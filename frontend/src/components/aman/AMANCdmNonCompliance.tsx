import {useState} from "react";
import {X} from "lucide-react";
import {inboundDepartureDescription, inboundDuration, inboundTime, type InboundDeparture, type InboundDeparturesSnapshot} from "@/api/inbound-departures";

const flightKey = (flight: InboundDeparture) => `${flight.callsign}/${flight.departure}/${flight.atot}`;

export function AMANCdmNonCompliance({snapshot, connected}: {snapshot: InboundDeparturesSnapshot | null; connected: boolean}) {
  const [dismissed, setDismissed] = useState<ReadonlySet<string>>(new Set());
  const flights = (snapshot?.flights ?? []).filter(flight => !dismissed.has(flightKey(flight)));
  const stale = snapshot?.stale || !connected;
  const awaiting = snapshot?.enabled && !snapshot.updated_at;
  const status = !snapshot?.enabled ? "vIFF arrival monitoring is unavailable."
    : awaiting ? "Awaiting vIFF arrival data."
    : `Updated ${new Date(snapshot.updated_at).toISOString().slice(11, 19)}Z${stale ? " · STALE — showing last successful update" : ""}`;
  return <section aria-label="CDM non-compliance" className="flex h-full min-h-0 flex-col border-[3px] border-[#dcdcdc] bg-[#3c3c3c] text-[#dcdcdc]">
    <h2 className="flex items-baseline justify-between gap-2 border-b border-[#777] px-2 py-1 font-display text-xs font-bold tracking-wide">
      <span>CDM NON-COMPLIANCE</span>
      <span className={`font-mono text-[10px] font-normal ${stale || awaiting ? "text-amber-300" : ""}`} role="status">{status}</span>
    </h2>
    <div className="min-h-0 flex-1 overflow-auto">
      {awaiting ? null : flights.length === 0 ? <p className="px-2 py-1 text-xs">{snapshot?.enabled ? (stale ? "No non-compliant inbound aircraft in the last successful update." : "No non-compliant inbound aircraft.") : ""}</p> :
        <table className="w-full border-collapse whitespace-nowrap text-left font-mono text-[11px]">
          <thead><tr className="border-b border-[#777]">{["Aircraft", "From", "To", "Status", "CTOT", "ATOT", "Dev.", "ETA", "SEND MSG", "Close"].map(label => <th className="px-2 py-0.5 font-semibold" key={label} scope="col">{label}</th>)}</tr></thead>
          <tbody>{flights.map(flight => <tr className="border-b border-[#555]" key={flightKey(flight)} title={inboundDepartureDescription(flight)}>
            <th className="px-2 py-0.5 font-semibold" scope="row">{flight.callsign}</th>
            <td className="px-2 py-0.5">{flight.departure}</td>
            <td className="px-2 py-0.5">{flight.arrival}</td>
            <td className="px-2 py-0.5 font-bold text-amber-300">NON-COMPLY</td>
            <td className="px-2 py-0.5">{inboundTime(flight.ctot)}</td>
            <td className="px-2 py-0.5">{inboundTime(flight.atot)}</td>
            <td className="px-2 py-0.5">{flight.deviation_seconds < 0 ? "−" : "+"}{inboundDuration(flight.deviation_seconds)}</td>
            <td className="px-2 py-0.5">{inboundTime(flight.eta)}</td>
            <td className="px-2 py-0.5"><button className="rounded border border-[#4d86e8] px-2 py-0.5 text-[10px] font-bold text-white" style={{backgroundColor: "#004FD6"}} type="button">SEND MSG</button></td>
            <td className="px-2 py-0.5"><button aria-label={`Remove ${flight.callsign}`} className="flex items-center hover:text-red-400" onClick={() => setDismissed(prev => new Set(prev).add(flightKey(flight)))} type="button"><X size={14} /></button></td>
          </tr>)}</tbody>
        </table>}
    </div>
  </section>;
}
