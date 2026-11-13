import {canInsertAMANFlight, isUnslottedAMANFlight, type AMANFlight} from "@/api/aman";

export function AMANUnslottedFlights({flights, disabled, onInsert}: {flights: AMANFlight[]; disabled: boolean; onInsert: (callsign: string) => void}) {
  const unslotted = flights.filter(isUnslottedAMANFlight).sort((a, b) => a.callsign.localeCompare(b.callsign));
  return <section aria-label="Aircraft without slots" className="grid gap-2 p-3 text-sm text-[#dcdcdc]">
    {unslotted.length === 0 && <p>No aircraft without slots.</p>}
    {unslotted.map(flight => <div className="grid gap-2 border border-[#777] p-2" key={flight.callsign}>
      <div className="flex items-center justify-between gap-3">
        <span><b>{flight.callsign}</b> · {flight.aircraft_type ?? "Type unknown"} · {flight.star_family ?? "STAR selection required"}</span>
        <button className="border border-[#777] px-2 py-1 disabled:opacity-50" disabled={disabled || !canInsertAMANFlight(flight)} onClick={() => onInsert(flight.callsign)} type="button">Insert behind…</button>
      </div>
      <p>{flight.operational_teta ? `${flight.basic_estimate ? "Basic estimate" : "Estimate"}: ${flight.operational_teta.slice(11, 16)} UTC` : "Estimate unavailable"} · Data: {flight.data_status}</p>
      {flight.prediction_reason && <p>{flight.prediction_reason}</p>}
    </div>)}
  </section>;
}
