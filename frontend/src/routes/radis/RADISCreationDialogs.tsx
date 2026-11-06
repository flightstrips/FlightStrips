import {useState} from "react";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
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
  const field = (name: Exclude<keyof LocalStripCreation, "requested_altitude">, label: string) => <label className="radis-fpl-field"><span>{label}</span><input aria-label={label} value={local[name]} disabled={readOnly} onChange={e => setLocal({...local, [name]: name === "notes" ? e.target.value : e.target.value.toUpperCase()})}/></label>;
  return <Dialog open={!!mode} onOpenChange={open => !open && close()}><DialogContent className="radis-dialog radis-creation"><DialogTitle>{mode === "find" ? "FIND connected flights" : "NEW local strip"}</DialogTitle>
    {mode === "find" ? <><form className="radis-find-search" onSubmit={e => {e.preventDefault(); find(query);}}><input aria-label="Find callsign" value={query} onChange={e => setQuery(e.target.value)} disabled={readOnly}/><Button type="submit" disabled={readOnly}>FIND</Button></form><div className="radis-find-count" role="status">{loading ? "Searching…" : `${candidates.length} connected flights`}</div><div className="radis-find-list">{candidates.map(f => <button key={`${f.cid}-${f.logon_time}-${f.callsign}`} type="button" disabled={readOnly} onClick={() => {if (admit(f)) close();}}><strong>{f.callsign}</strong><span>{f.origin} → {f.destination}</span><span>{f.aircraft_type}</span></button>)}</div><Button onClick={close}>DISMISS</Button></>
      : <form onSubmit={e => {e.preventDefault(); if (create(local)) close();}}><div className="radis-local-grid">{field("callsign", "Callsign")}{field("origin", "Origin")}{field("destination", "Destination")}{field("aircraft_type", "Aircraft type")}{field("registration", "Registration")}<label className="radis-fpl-field"><span>Requested altitude (feet)</span><input type="number" min="0" max="60000" aria-label="Requested altitude (feet)" value={local.requested_altitude || ""} disabled={readOnly} onChange={e => setLocal({...local, requested_altitude: Number(e.target.value)})}/></label><label className="radis-fpl-field"><span>Runway</span><select value={local.runway} disabled={readOnly} onChange={e => setLocal({...local, runway: e.target.value})}><option value="">—</option>{runways?.map(r => <option key={r}>{r}</option>)}</select></label></div>{field("route", "Route")}{field("notes", "Notes")}<div className="radis-dialog-actions"><Button type="button" onClick={close}>ESC</Button><Button type="submit" disabled={readOnly}>CREATE LOCAL STRIP</Button></div></form>}
  </DialogContent></Dialog>;
}
