import {useWebSocketStore} from "@/store/store-hooks";
import {inboundDepartureDescription} from "@/api/inbound-departures";

export function InboundCtotFlag({callsign}: {callsign: string}) {
  const flight = useWebSocketStore(state => {
    const strip = state.strips.find(item => item.callsign === callsign);
    if (!strip || strip.origin === state.airport || strip.destination !== state.airport) return undefined;
    return state.inboundDepartures?.flights.find(item => item.callsign === callsign && item.departure === strip.origin && item.arrival === strip.destination);
  });
  const stale = useWebSocketStore(state => state.inboundDepartures?.stale || state.amanConnectionState !== "connected");
  if (!flight) return null;
  const description = `CTOT ${flight.status} · ${inboundDepartureDescription(flight)}${stale ? " · Stale data" : ""}`;
  return <span aria-label={description} title={description} tabIndex={0} className="ml-1 shrink-0 rounded bg-amber-300 px-1 font-display text-[0.55vw] font-bold text-black [-webkit-text-stroke:0] focus-visible:outline focus-visible:outline-2 focus-visible:outline-black">CTOT {flight.status === "early" ? "E" : "L"}{stale ? "*" : ""}</span>;
}
