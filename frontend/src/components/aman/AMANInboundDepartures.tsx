import {inboundDepartureDescription, inboundDuration, inboundTime, type InboundDeparturesSnapshot} from "@/api/inbound-departures";

export function AMANInboundDepartures({snapshot, connected}: {snapshot: InboundDeparturesSnapshot | null; connected: boolean}) {
  const stale = snapshot?.stale || !connected;
  return <section aria-label="CTOT nonconforming inbound aircraft" className="grid gap-3 p-3 text-sm text-[#dcdcdc]">
    <p>Inbound aircraft that departed before CTOT −5 minutes or after CTOT +10 minutes. All times UTC.</p>
    {!snapshot?.enabled ? <p>vIFF arrival monitoring is unavailable for this session.</p> : <>
      <p role="status">{snapshot.updated_at ? `Last updated ${new Date(snapshot.updated_at).toISOString().slice(11, 19)}Z` : "Awaiting vIFF arrival data."}{stale && snapshot.updated_at ? " · Stale data — showing the last successful update." : ""}</p>
      {snapshot.updated_at && snapshot.flights.length === 0 && <p>{stale ? "No nonconforming departures in the last successful update." : "No inbound aircraft departed outside their CTOT window."}</p>}
      {snapshot.flights.length > 0 && <div className="overflow-x-auto"><table className="w-full border-collapse text-left text-xs">
        <thead><tr className="border-b border-[#777]">{["Aircraft", "From", "Status", "CTOT", "ATOT", "Deviation", "Outside window"].map(label => <th className="p-2" key={label} scope="col">{label}</th>)}</tr></thead>
        <tbody>{snapshot.flights.map(flight => <tr className="border-b border-[#555]" key={`${flight.callsign}/${flight.departure}/${flight.atot}`} title={inboundDepartureDescription(flight)}>
          <th className="p-2" scope="row">{flight.callsign}</th><td className="p-2">{flight.departure}</td>
          <td className="p-2 font-bold text-amber-300">{flight.status === "early" ? "Early" : "Late"}</td>
          <td className="whitespace-nowrap p-2">{inboundTime(flight.ctot)}</td><td className="whitespace-nowrap p-2">{inboundTime(flight.atot)}</td>
          <td className="whitespace-nowrap p-2">{flight.deviation_seconds < 0 ? "−" : "+"}{inboundDuration(flight.deviation_seconds)}</td>
          <td className="whitespace-nowrap p-2">{inboundDuration(flight.outside_window_seconds)}</td>
        </tr>)}</tbody>
      </table></div>}
    </>}
  </section>;
}
