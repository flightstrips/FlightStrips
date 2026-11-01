import {inboundDuration, inboundTime, type InboundCtotFlight, type InboundDeparturesSnapshot} from "@/api/inbound-departures";

const STATUS_LABEL: Record<InboundCtotFlight["status"], string> = {pending: "Pending", compliant: "OK", early: "Early", late: "Late"};

function statusClass(status: InboundCtotFlight["status"]): string {
  if (status === "early" || status === "late") return "font-bold text-amber-300";
  return status === "compliant" ? "text-[#96d796]" : "";
}

export function AMANInboundDepartures({snapshot, connected}: {snapshot: InboundDeparturesSnapshot | null; connected: boolean}) {
  const stale = snapshot?.stale || !connected;
  const flights = snapshot?.ctot_flights ?? [];
  return <section aria-label="Inbound aircraft with CTOT" className="grid gap-3 p-3 text-sm text-[#dcdcdc]">
    <p>All inbound aircraft holding a CTOT, including those already departed. All times UTC.</p>
    {!snapshot?.enabled ? <p>vIFF arrival monitoring is unavailable for this session.</p> : <>
      <p role="status">{snapshot.updated_at ? `Last updated ${new Date(snapshot.updated_at).toISOString().slice(11, 19)}Z` : "Awaiting vIFF arrival data."}{stale && snapshot.updated_at ? " · Stale data — showing the last successful update." : ""}</p>
      {snapshot.updated_at && flights.length === 0 && <p>{stale ? "No inbound aircraft hold a CTOT in the last successful update." : "No inbound aircraft hold a CTOT."}</p>}
      {flights.length > 0 && <div className="overflow-x-auto"><table className="w-full border-collapse text-left text-xs">
        <thead><tr className="border-b border-[#777]">{["Aircraft", "From", "CTOT", "ATOT", "ETA", "Status", "Deviation", "CDM", "Regulation"].map(label => <th className="p-2" key={label} scope="col">{label}</th>)}</tr></thead>
        <tbody>{flights.map(flight => <tr className="border-b border-[#555]" key={`${flight.callsign}/${flight.departure}/${flight.atot}`}>
          <th className="p-2" scope="row">{flight.callsign}</th><td className="p-2">{flight.departure}</td>
          <td className="whitespace-nowrap p-2">{inboundTime(flight.ctot)}</td>
          <td className="whitespace-nowrap p-2">{flight.atot ? inboundTime(flight.atot) : "—"}</td>
          <td className="whitespace-nowrap p-2">{inboundTime(flight.eta)}</td>
          <td className={`p-2 ${statusClass(flight.status)}`}>{STATUS_LABEL[flight.status]}</td>
          <td className="whitespace-nowrap p-2">{flight.status === "pending" ? "—" : `${flight.deviation_seconds < 0 ? "−" : "+"}${inboundDuration(flight.deviation_seconds)}`}</td>
          <td className="p-2">{flight.cdm_status || "—"}</td>
          <td className="p-2">{flight.regulation || "—"}</td>
        </tr>)}</tbody>
      </table></div>}
    </>}
  </section>;
}
