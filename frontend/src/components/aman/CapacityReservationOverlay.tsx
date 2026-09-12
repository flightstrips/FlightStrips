import type {AMANCapacityReservation, AMANDataStatus} from "@/api/aman";
import type {AMANTimelineRange} from "./presentation";

export function CapacityReservationOverlay({reservations, range, runway, status}: {reservations: AMANCapacityReservation[]; range: AMANTimelineRange; runway: string; status: AMANDataStatus}) {
  return reservations.map((item) => {
    const start = Date.parse(item.start), end = Date.parse(item.end), span = range.endMs - range.startMs;
    if (end <= range.startMs || start >= range.endMs) return null;
    const top = (Math.max(start, range.startMs) - range.startMs) / span * 100;
    const height = (Math.min(end, range.endMs) - Math.max(start, range.startMs)) / span * 100;
    return <div aria-label={`Extra Flight ${item.label} on ${runway}, server confirmed immutable interval ${item.start} to ${item.end}, end exclusive${status === "fresh" ? "" : `; ${status} state`}`} className="pointer-events-none absolute inset-x-2 z-[9] overflow-hidden border-2 border-dashed border-cyan-100 bg-cyan-800/80 px-1 text-center text-[10px] font-black text-white" data-reservation-id={item.id} key={item.id} role="note" style={{height: `${Math.max(.8, height)}%`, top: `${top}%`}}>✚ EXTRA FLIGHT · {item.label}{status === "fresh" ? "" : ` · ${status.toUpperCase()}`}</div>;
  });
}
