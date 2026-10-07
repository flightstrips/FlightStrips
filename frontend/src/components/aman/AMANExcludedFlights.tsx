import {useState} from "react";
import {isExcludedAMANFlight, type AMANCommandIntent, type AMANCommandRejection, type AMANFlight, type AMANPendingCommand} from "@/api/aman";

export function AMANExcludedFlights({flights, disabled, pendingCommands, commandRejections, onCommand}: {
  flights: AMANFlight[];
  disabled: boolean;
  pendingCommands: Record<string, AMANPendingCommand>;
  commandRejections: Record<string, AMANCommandRejection>;
  onCommand: (command: AMANCommandIntent) => string | null;
}) {
  const [commands, setCommands] = useState<Record<string, string | null>>({});
  const excluded = flights.filter(isExcludedAMANFlight);
  return <section aria-label="Excluded aircraft" className="grid gap-2 p-3 text-sm text-[#dcdcdc]">
    {excluded.length === 0 && <p>No excluded aircraft.</p>}
    {excluded.map(flight => {
      const commandID = commands[flight.callsign];
      const pending = commandID ? pendingCommands[commandID] !== undefined : false;
      const rejection = commandID ? commandRejections[commandID]?.message : undefined;
      return <div className="border border-[#777] p-2" key={flight.callsign}>
        <div className="flex items-center justify-between gap-3">
          <span><b>{flight.callsign}</b> · {flight.lifecycle_state === "removed" ? "Removed" : "Desequenced"}</span>
          <button className="border border-[#777] px-2 py-1 disabled:opacity-50" disabled={disabled || pending} onClick={() => {
            const id = onCommand({type: "aman.resume_flight", callsign: flight.callsign});
            setCommands(previous => ({...previous, [flight.callsign]: id}));
          }} type="button">{pending ? "Adding…" : "Add back to sequence"}</button>
        </div>
        {rejection && <p role="alert">{rejection}</p>}
      </div>;
    })}
  </section>;
}
