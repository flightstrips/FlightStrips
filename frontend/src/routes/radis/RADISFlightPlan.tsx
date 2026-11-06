import type {FrontendStrip} from "@/api/models";
import {paperClock} from "@/api/strip-board";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";
import {RADISServices} from "./RADISServices";
import type {RADISEdit} from "./RADISStrip";

export function RADISFlightPlan({strip, onClose, edit}: {strip: FrontendStrip | undefined; onClose: () => void; edit: (strip: FrontendStrip, field: RADISEdit["field"], title: string, value: string) => void}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const draftContext = useWebSocketStore(s => `${s.boardContext?.subscription_id ?? ""}:${s.boardContext?.transport_generation ?? ""}:${s.boardContext?.generation ?? ""}:${s.readOnly}`);
  const annotate = useWebSocketStore(s => s.annotateStrip);
  const field = (label: string, content: string | number | undefined, action?: RADISEdit["field"]) => <label className="radis-fpl-field"><span>{label}</span><button type="button" disabled={!action || readOnly || (action !== "notes" && !strip?.has_fp)} onClick={() => strip && action && edit(strip, action, label, String(content ?? ""))}>{content || "—"}</button></label>;
  return <Dialog open={!!strip} onOpenChange={open => !open && onClose()}><DialogContent className="radis-dialog radis-fpl"><DialogTitle>Flight plan · {strip?.callsign}</DialogTitle>{strip && <>
    <fieldset><legend>DEPARTURE</legend><div className="radis-fpl-grid">
      {field("C/S", strip.callsign)}{field("ADES", strip.destination)}<label className="radis-fpl-field"><span>RNAV</span><select value={strip.board_state?.annotations.rnav ?? ""} disabled={readOnly} onChange={e => annotate(strip.callsign, "rnav", e.target.value)}><option value="">—</option><option value="NON">NON</option><option value="1">1</option></select></label>
      {field("SID", strip.sid, "sid")}{field("SSR", strip.assigned_squawk, "assigned_squawk")}{field("CTOT", strip.ctot)}{field("EOBT", strip.eobt, "eobt")}{field("RWY", strip.runway, "runway")}<label className="radis-fpl-field"><span>REA</span><button disabled={readOnly} onClick={() => annotate(strip.callsign, "rea", "", !strip.board_state?.annotations.rea)}>{paperClock(strip.board_state?.annotations.rea) || "—"}</button></label>
      {field("TYPE", strip.aircraft_type, "aircraft_type")}{field("RFL", strip.requested_altitude)}{field("STAND", strip.stand, "stand")}
    </div><div className="radis-fpl-wide">{field("ROUTE", strip.route, "route")}{field("ES REMARKS", strip.remarks, "remarks")}{field("RADIS NOTES", strip.board_state?.annotations.notes, "notes")}</div>
      <div className="radis-fpl-grid">{field("HDG", strip.heading, "heading")}{field("ALT FT", strip.cleared_altitude, "altitude")}<label><input type="checkbox" checked={strip.board_state?.annotations.deicing ?? false} disabled={readOnly} onChange={e => annotate(strip.callsign, "deicing", "", e.target.checked)}/> DE-ICE</label><label><input type="checkbox" checked={strip.board_state?.annotations.vfr ?? false} disabled={readOnly} onChange={e => annotate(strip.callsign, "vfr", "", e.target.checked)}/> VFR mark</label>{field("REG", strip.registration)}</div>
    </fieldset><fieldset><legend>ARRIVAL</legend><div className="radis-fpl-grid">{field("ADEP", strip.origin)}{field("RWY", strip.runway, "runway")}{field("ETA", strip.eldt)}{field("AOBT", strip.aobt)}{field("EET", strip.flight_plan_eet)}</div></fieldset><RADISServices key={`${strip.id}:${strip.version}:${strip.tobt ?? ""}:${draftContext}`} strip={strip} close={onClose}/><Button onClick={onClose}>OK</Button>
  </>}</DialogContent></Dialog>;
}
