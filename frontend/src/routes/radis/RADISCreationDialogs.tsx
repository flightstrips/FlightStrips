import {useState} from "react";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog} from "@/components/ui/dialog";
import {FlightStripsDialogContent} from "@/components/ui/flightstrips-dialog";
import {Button} from "@/components/ui/button";
import type {LocalStripCreation} from "@/api/strip-board";

export function RADISCreationDialogs({mode, close}: {mode: "new" | "find" | null; close: () => void}) {
  const airport = useWebSocketStore(s => s.airport);
  const readOnly = useWebSocketStore(s => s.readOnly);
  const runways = useWebSocketStore(s => s.boardDefinition?.runways);
  const create = useWebSocketStore(s => s.createLocalStrip);
  const find = useWebSocketStore(s => s.findConnectedFlights);
  const admit = useWebSocketStore(s => s.admitConnectedFlight);
  const candidates = useWebSocketStore(s => s.connectedFlightCandidates);
  const loading = useWebSocketStore(s => s.connectedFlightSearchRequest !== null);
  const [query, setQuery] = useState("");
  const [local, setLocal] = useState<LocalStripCreation>({callsign: "", origin: airport, destination: airport, aircraft_type: "", route: "", registration: "", requested_altitude: 0, runway: "", notes: ""});
  const field = (name: Exclude<keyof LocalStripCreation, "requested_altitude">, label: string) => (
    <label className="radis-form-field">
      {label}
      <input value={local[name]} disabled={readOnly} onChange={e =>
        setLocal({...local, [name]: name === "notes" ? e.target.value : e.target.value.toUpperCase()})}/>
    </label>
  );

  return <Dialog open={!!mode} onOpenChange={open => !open && close()}>
    <FlightStripsDialogContent
      title={mode === "find" ? "FIND connected flights" : "NEW local strip"}
      className="radis-dialog-font radis-support-dialog w-[min(640px,95vw)]"
    >
      {mode === "find" ? <>
        <form className="radis-find-search" onSubmit={e => {e.preventDefault(); find(query);}}>
          <input aria-label="Find callsign" value={query} onChange={e => setQuery(e.target.value)} disabled={readOnly}/>
          <Button variant="darkaction" type="submit" disabled={readOnly}>FIND</Button>
        </form>
        <div role="status">{loading ? "Searching…" : `${candidates.length} connected flights`}</div>
        <div className="radis-find-list">
          {candidates.map(f => <button key={`${f.cid}-${f.logon_time}-${f.callsign}`} type="button" disabled={readOnly} onClick={() => {if (admit(f)) close();}}>
            <strong>{f.callsign}</strong><span>{f.origin} → {f.destination}</span><span>{f.aircraft_type}</span>
          </button>)}
        </div>
        <Button variant="darkaction" className="justify-self-center" onClick={close}>DISMISS</Button>
      </> : <form className="grid gap-3" onSubmit={e => {e.preventDefault(); if (create(local)) close();}}>
        <div className="radis-local-grid">
          {field("callsign", "Callsign")}
          {field("origin", "Origin")}
          {field("destination", "Destination")}
          {field("aircraft_type", "Aircraft type")}
          {field("registration", "Registration")}
          <label className="radis-form-field">
            Requested altitude (feet)
            <input type="number" min="0" max="60000" value={local.requested_altitude || ""} disabled={readOnly}
              onChange={e => setLocal({...local, requested_altitude: Number(e.target.value)})}/>
          </label>
          <label className="radis-form-field">
            Runway
            <select value={local.runway} disabled={readOnly} onChange={e => setLocal({...local, runway: e.target.value})}>
              <option value="">—</option>
              {runways?.map(r => <option key={r}>{r}</option>)}
            </select>
          </label>
        </div>
        {field("route", "Route")}
        {field("notes", "Notes")}
        <div className="flex flex-wrap justify-between gap-2">
          <Button variant="darkaction" type="button" onClick={close}>ESC</Button>
          <Button variant="darkaction" type="submit" disabled={readOnly}>CREATE LOCAL STRIP</Button>
        </div>
      </form>}
    </FlightStripsDialogContent>
  </Dialog>;
}
