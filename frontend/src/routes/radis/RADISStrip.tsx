import {useSortable} from "@dnd-kit/sortable";
import {CSS} from "@dnd-kit/utilities";
import type {FrontendStrip} from "@/api/models";
import type {StripFieldEdit, StripRevision} from "@/api/strip-board";
import {paperClock} from "@/api/strip-board";
import {useWebSocketStore} from "@/store/store-hooks";
import {StripCell, SplitStripCell} from "@/components/strip/StripCell";
import arrivalIcon from "./assets/arrival.png";
import departureIcon from "./assets/departure.png";

export type RADISEdit = {revision?:StripRevision;callsign: string; field: keyof StripFieldEdit | "route_heading" | "notes" | "atd" | "endurance_pob"; title: string; value: string};
const border = "#404040";

export function RADISStrip({strip, open, edit}: {strip: FrontendStrip; open: () => void; edit: (field: RADISEdit["field"], title: string, value: string) => void}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const airport = useWebSocketStore(s => s.airport);
  const annotate = useWebSocketStore(s => s.annotateStrip);
  const selected = useWebSocketStore(s => s.selectedCallsign === strip.callsign);
  const select = useWebSocketStore(s => s.selectStrip);
  const metar = useWebSocketStore(s => s.metar);
  const {attributes, listeners, setNodeRef, transform, transition, isDragging} = useSortable({id: strip.callsign, disabled: readOnly});
  const a = strip.board_state?.annotations;
  const date = strip.board_state?.created_at;
  const dep = strip.origin === airport;
  const value = (label: string, content: string | number, onClick?: () => void, es = false) => <button type="button" aria-label={`${label} ${strip.callsign}`} disabled={!onClick || readOnly || (es && !strip.has_fp)} onClick={onClick} className="radis-cell-button"><small>{label}</small><strong>{content || ""}</strong></button>;
  const stampQNH = () => {const qnh = metar?.match(/\bQ\d{4}\b/)?.[0]; if (qnh) annotate(strip.callsign, "qnh", qnh);};
  return <article ref={setNodeRef} className={`radis-strip ${selected ? "is-selected" : ""} ${isDragging ? "is-dragging" : ""}`} style={{transform: CSS.Transform.toString(transform), transition}} data-testid={`radis-strip-${strip.callsign}`} onClick={() => select(strip.callsign)}>
    <StripCell width="27%" borderColor={border} className="radis-callsign">
      <button type="button" onClick={open} className="radis-callsign-title" aria-label={`Flight plan ${strip.callsign}`}>{strip.callsign}</button>
      <div className="radis-route-airports"><span>{strip.origin}</span><small>RADIS<br/>{airport}</small><span>{strip.destination}</span></div>
      <div className="radis-aircraft-line"><span>{strip.aircraft_type}</span><span>{a?.vfr ? "V" : strip.aircraft_category}</span><button disabled={readOnly || !strip.has_fp} onClick={() => edit("assigned_squawk", "Squawk", strip.assigned_squawk)} aria-label={`Squawk ${strip.callsign}`}>{strip.assigned_squawk}</button></div>
    </StripCell>
    <SplitStripCell width="6.2%" borderColor={border} top={value("RWY", strip.runway, () => edit("runway", "Runway", strip.runway), true)} bottom={value("QNH", a?.qnh ?? "", stampQNH)}/>
    <SplitStripCell width="12.3%" borderColor={border} top={value("HDG", strip.heading, () => edit("route_heading", "Route / heading", strip.heading ? String(strip.heading) : strip.route), true)} bottom={value("SID/POINT", strip.sid || strip.route.split(" ")[0], () => edit("sid", "SID", strip.sid), true)}/>
    <SplitStripCell width="12.3%" borderColor={border} top={value("RFL", strip.requested_altitude)} bottom={value("ALT FT", strip.cleared_altitude, () => edit("altitude", "Cleared altitude (feet)", String(strip.cleared_altitude)), true)}/>
    <div className="radis-times">
      <SplitStripCell width="33.333%" borderColor={border} top={value("EOBT", strip.eobt, () => edit("eobt", "EOBT (UTC HHMM)", strip.eobt), true)} bottom={value("ATD", a?.atd ?? "", () => edit("atd", "ATD (UTC HHMM)", a?.atd ?? ""))}/>
      <SplitStripCell width="33.333%" borderColor={border} top={value("EET", strip.flight_plan_eet ?? "")} bottom={value("END/POB", a?.endurance_pob ?? "", () => edit("endurance_pob", "Endurance / persons on board", a?.endurance_pob ?? ""))}/>
      <SplitStripCell width="33.334%" borderColor={border} top={value("ETA", strip.eldt)} bottom={value("ATA", paperClock(a?.ata), () => annotate(strip.callsign, "ata", "", !a?.ata))}/>
    </div>
    <StripCell width="17.4%" borderColor={border} className="radis-direction">
      <button className="radis-notes" disabled={readOnly} onClick={() => edit("notes", "RADIS notes", a?.notes ?? "")} aria-label={`Notes ${strip.callsign}`}>{a?.deicing ? "DE-ICE " : ""}{a?.notes}</button>
      <button type="button" className="radis-stand" onClick={() => edit("stand", "Stand", strip.stand)} disabled={readOnly || !strip.has_fp}>Stand<br/><strong>{strip.stand}</strong></button>
      <button type="button" className="radis-direction-icon" onClick={open} aria-label={`${dep ? "Departure" : "Arrival"} flight ${strip.callsign}`}><img src={dep ? departureIcon : arrivalIcon} alt=""/><strong>{airport.slice(-2)}</strong></button>
    </StripCell>
    <div className="radis-strip-date"><button type="button" {...attributes} {...listeners} aria-label={`Move ${strip.callsign}`} className="radis-drag-handle">{date ? `${date.slice(8,10)}/${date.slice(5,7)}/${date.slice(0,4)}` : strip.board_state?.local ? "LOCAL" : "MOVE"}</button></div>
  </article>;
}
