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

// RADIS paper notation preserves exact feet when the value is not a whole
// hundred, rather than rounding a controller's clearance for display.
function paperAltitude(feet: number, transitionAltitude: number) {
  if (!feet) return "";
  if (feet % 100) return String(feet);
  return `${feet > transitionAltitude ? "F" : "A"}${String(feet / 100).padStart(3, "0")}`;
}

type RADISStripProps = {
  strip: FrontendStrip;
  open: () => void;
  edit: (field: RADISEdit["field"], title: string, value: string) => void;
};

export function RADISStrip(props: RADISStripProps) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const sortable = useSortable({
    id: props.strip.callsign,
    disabled: readOnly,
    data: {radisBay: props.strip.bay},
  });
  return <RADISStripPaper {...props} sortable={sortable}/>;
}

export function RADISStripPreview({strip}: {strip: FrontendStrip}) {
  return <div className="radis-drag-preview" inert aria-hidden="true">
    <RADISStripPaper strip={strip}/>
  </div>;
}

function RADISStripPaper({strip, open, edit, sortable}: {
  strip: FrontendStrip;
  open?: RADISStripProps["open"];
  edit?: RADISStripProps["edit"];
  sortable?: ReturnType<typeof useSortable>;
}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const airport = useWebSocketStore(s => s.airport);
  const annotate = useWebSocketStore(s => s.annotateStrip);
  const selected = useWebSocketStore(s => s.selectedCallsign === strip.callsign);
  const select = useWebSocketStore(s => s.selectStrip);
  const metar = useWebSocketStore(s => s.metar);
  const transitionAltitude = useWebSocketStore(s => s.transitionAltitude);
  const {attributes, listeners, setNodeRef, transform, transition, isDragging} = sortable ?? {};
  const a = strip.board_state?.annotations;
  const date = strip.board_state?.created_at;
  const dep = strip.origin === airport;
  const circuit = dep && strip.destination === airport;
  const directions = circuit ? ["arrival", "departure"] : [dep ? "departure" : "arrival"];
  const vfr = a?.vfr || strip.flight_rules?.startsWith("V");
  const routePoints = (strip.sid || strip.star || strip.route).split(/\s+/).filter(Boolean).slice(0, 2);
  const value = (label: string, content: string | number, onClick?: () => void, es = false, showLabel = false) => <button type="button" aria-label={`${label} ${strip.callsign}`} disabled={!onClick || readOnly || (es && !strip.has_fp)} onClick={onClick} className={`radis-cell-button ${showLabel ? "radis-labelled-cell" : ""}`}><small className={showLabel ? undefined : "sr-only"}>{label}</small><strong>{content || ""}</strong></button>;
  const stampQNH = () => {const qnh = metar?.match(/\bQ\d{4}\b/)?.[0]; if (qnh) annotate(strip.callsign, "qnh", qnh);};
  return <article ref={setNodeRef} className={`radis-strip ${circuit ? "radis-circuit-strip" : ""} ${selected ? "is-selected" : ""} ${isDragging ? "is-dragging" : ""}`} style={{transform: CSS.Transform.toString(transform ?? null), transition}} data-testid={sortable ? `radis-strip-${strip.callsign}` : undefined} data-strip-version={strip.version} onClick={sortable ? () => select(strip.callsign) : undefined}>
    <StripCell borderColor={border} className="radis-callsign">
      <button type="button" onClick={open} className="radis-callsign-title" aria-label={`Flight plan ${strip.callsign}`}>{strip.callsign}</button>
      <div className="radis-route-airports"><span>{strip.origin}</span><small>{routePoints.map((point, index) => <span key={index}>{point}</span>)}</small><span>{strip.destination}</span></div>
      <div className="radis-aircraft-line"><span>{strip.aircraft_type}</span><span>{strip.aircraft_category}</span><button disabled={readOnly || !strip.has_fp} onClick={() => edit?.("assigned_squawk", "Squawk", strip.assigned_squawk)} aria-label={`Squawk ${strip.callsign}`}>{strip.assigned_squawk}</button></div>
    </StripCell>
    <SplitStripCell borderColor={border} top={value("RWY", strip.runway, () => edit?.("runway", "Runway", strip.runway), true)} bottom={value("QNH", a?.qnh?.replace(/^Q/, "") ?? "", stampQNH)}/>
    <SplitStripCell borderColor={border} className="radis-clearance-cell" top={value("HDG", strip.heading ? String(strip.heading).padStart(3, "0") : "", () => edit?.("route_heading", "Route / heading", strip.heading ? String(strip.heading) : strip.route), true)} bottom={value("SID/POINT", strip.sid, () => edit?.("sid", "SID", strip.sid), true)}/>
    <StripCell borderColor={border} className="radis-altitude-cell">
      <button type="button" className="radis-altitude-button" aria-label={`ALT FT ${strip.callsign}`} disabled={readOnly || !strip.has_fp} onClick={() => edit?.("altitude", "Cleared altitude (feet)", String(strip.cleared_altitude))}>
        {dep && !!strip.requested_altitude && <span className="radis-requested-altitude">{!vfr && <small>RFL</small>}<strong>{paperAltitude(strip.requested_altitude, transitionAltitude)}</strong></span>}
        {!!strip.cleared_altitude && <strong className={dep && strip.requested_altitude ? "radis-cleared-altitude" : "radis-arrival-altitude"}>{paperAltitude(strip.cleared_altitude, transitionAltitude)}</strong>}
      </button>
      <button type="button" className="radis-flight-rules" aria-label={`VFR mark ${strip.callsign}`} aria-pressed={!!a?.vfr} disabled={readOnly} onClick={() => annotate(strip.callsign, "vfr", "", !a?.vfr)}>{vfr ? "V" : dep ? strip.flight_rules?.slice(0, 1) : ""}</button>
    </StripCell>
    <div className="radis-times">
      <SplitStripCell width="33.333%" borderColor={border} top={value("EOBT", strip.eobt, () => edit?.("eobt", "EOBT (UTC HHMM)", strip.eobt), true, true)} bottom={value("ATD", a?.atd ?? "", () => edit?.("atd", "ATD (UTC HHMM)", a?.atd ?? ""), false, true)}/>
      <SplitStripCell width="33.333%" borderColor={border} top={value("EET", strip.flight_plan_eet ?? "", undefined, false, true)} bottom={value("END/POB", a?.endurance_pob ?? "", () => edit?.("endurance_pob", "Endurance / persons on board", a?.endurance_pob ?? ""), false, true)}/>
      <SplitStripCell width="33.334%" borderColor={border} top={value("ETA", strip.eldt, undefined, false, true)} bottom={value("ATA", paperClock(a?.ata), () => annotate(strip.callsign, "ata", "", !a?.ata), false, true)}/>
    </div>
    <StripCell borderColor={border} className="radis-direction">
      <button className="radis-notes" disabled={readOnly} onClick={() => edit?.("notes", "RADIS notes", a?.notes ?? "")} aria-label={`Notes ${strip.callsign}`}>{a?.deicing ? "DE-ICE " : ""}{a?.notes}</button>
      <button type="button" className="radis-stand" onClick={() => edit?.("stand", "Stand", strip.stand)} disabled={readOnly || !strip.has_fp}>Stand<br/><strong>{strip.stand || "NIL"}</strong></button>
      <button type="button" className={`radis-direction-icon ${circuit ? "radis-circuit" : ""}`} onClick={open} aria-label={`${circuit ? "Local" : dep ? "Departure" : "Arrival"} flight ${strip.callsign}`}>
        {directions.map(direction => <span className={`radis-flight-direction radis-${direction}-icon`} key={direction}><img src={direction === "departure" ? departureIcon : arrivalIcon} alt=""/><strong>{airport.slice(-2)}</strong></span>)}
      </button>
    </StripCell>
    <div className="radis-strip-date"><button type="button" {...attributes} {...listeners} aria-label={`Move ${strip.callsign}`} className="radis-drag-handle">{date ? `${date.slice(8,10)}/${date.slice(5,7)}/${date.slice(0,4)}` : strip.board_state?.local ? "LOCAL" : "MOVE"}</button></div>
  </article>;
}
