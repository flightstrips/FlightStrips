import type {FrontendStrip} from "@/api/models";
import {paperClock} from "@/api/strip-board";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";
import {RADISServices} from "./RADISServices";
import type {RADISEdit} from "./RADISStrip";
import {formatAltitude} from "@/lib/utils";

export function RADISFlightPlan({strip, onClose, edit}: {strip: FrontendStrip | undefined; onClose: () => void; edit: (strip: FrontendStrip, field: RADISEdit["field"], title: string, value: string) => void}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const draftContext = useWebSocketStore(s => `${s.boardContext?.subscription_id ?? ""}:${s.boardContext?.transport_generation ?? ""}:${s.boardContext?.generation ?? ""}:${s.readOnly}`);
  const annotate = useWebSocketStore(s => s.annotateStrip);
  const capabilities = useWebSocketStore(s => s.boardDefinition?.capabilities);
  const transitionAltitude = useWebSocketStore(s => s.transitionAltitude);
  const field = (label: string, content: string | number | undefined, action?: RADISEdit["field"], display = content) => <label className="radis-fpl-field"><span>{label}</span><button type="button" disabled={!action || readOnly || (action !== "notes" && !strip?.has_fp)} onClick={() => strip && action && edit(strip, action, label, String(content ?? ""))}>{display || ""}</button></label>;
  return <Dialog open={!!strip} onOpenChange={open => !open && onClose()}><DialogContent className="radis-dialog radis-fpl"><DialogTitle className="sr-only">Flight plan · {strip?.callsign}</DialogTitle>{strip && <>
    <div className="radis-fpl-sheet">
      <fieldset className="radis-fpl-departure"><legend>DEPARTURE</legend>
        <div className="radis-fpl-grid radis-fpl-identity">
          {field("C/S", strip.callsign)}{field("ADES", strip.destination)}
          <label className="radis-fpl-field"><span>RNAV</span><select value={strip.board_state?.annotations.rnav ?? ""} disabled={readOnly} onChange={e => annotate(strip.callsign, "rnav", e.target.value)}><option value=""></option><option value="NON">NON</option><option value="1">1</option></select></label>
          {field("SID", strip.sid, "sid")}{field("SSR", strip.assigned_squawk, "assigned_squawk")}
          <div className="radis-fpl-field"><span>CTOT</span><div className="radis-fpl-slot"><button type="button" aria-label="CTOT source" disabled>{strip.ctot_source || ""}</button><button type="button" aria-label="CTOT UTC" disabled>{strip.ctot}</button></div></div>
        </div>
        <div className="radis-fpl-timing">{field("EOBT", strip.eobt, "eobt")}{field("RWY", strip.runway, "runway")}<label className="radis-fpl-field"><span>REA</span><button disabled={readOnly} onClick={() => annotate(strip.callsign, "rea", "", !strip.board_state?.annotations.rea)}>{paperClock(strip.board_state?.annotations.rea)}</button></label></div>
        <div className="radis-fpl-performance">{field("TYPE", strip.aircraft_type, "aircraft_type")}{field("FL", strip.requested_altitude ? strip.requested_altitude / 100 : "")}{field("SPEED", undefined)}{field("STS", strip.status)}</div>
        <div className="radis-fpl-route">{field("ROUTE", strip.route, "route")}</div>
        <div className="radis-fpl-remarks">{field("COOPANS REMARKS", strip.remarks, "remarks")}</div>
        <div className="radis-fpl-notes">{field("NITOS REMARKS", strip.board_state?.annotations.notes, "notes")}{field("IATA TYPE", undefined)}</div>
        <div className="radis-fpl-grid radis-fpl-clearance">
          {field("CLIMB GR.", undefined)}{field("HDG", strip.heading, "heading")}{field("ALT", strip.cleared_altitude, "altitude", strip.cleared_altitude ? formatAltitude(strip.cleared_altitude, transitionAltitude).replace(/^FL(\d+)$/, (_, level: string) => `FL${level.padStart(3, "0")}`) : "")}
          <label className="radis-fpl-field"><span>DE-ICE</span><input type="checkbox" checked={strip.board_state?.annotations.deicing ?? false} disabled={readOnly} onChange={e => annotate(strip.callsign, "deicing", "", e.target.checked)}/></label>
          {field("REG", strip.registration)}{field("STAND", strip.stand, "stand")}
        </div>
      </fieldset>
      <fieldset className="radis-fpl-arrival"><legend>ARRIVAL</legend><div className="radis-fpl-grid">{field("ADEP", strip.origin)}{field("STAR", strip.star)}{field("RWY", strip.runway, "runway")}{field("ETA", strip.eldt)}{field("AOBT", strip.aobt)}</div></fieldset>
      <Button variant="darkaction" className="radis-fpl-dismiss" onClick={onClose}>OK</Button>
    </div>
    {(capabilities?.cdm || capabilities?.text_pilot) && <RADISServices key={`${strip.id}:${strip.version}:${strip.tobt ?? ""}:${draftContext}`} strip={strip} close={onClose}/>}
  </>}</DialogContent></Dialog>;
}
