import {useState} from "react";
import type {FrontendStrip} from "@/api/models";
import {useWebSocketStore} from "@/store/store-hooks";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";

// Service commands use the host's scoped orchestration and versioned SAT APIs.
// RADIS paper timestamps/deicing remain separate from CDM operational state.
export function RADISServices({strip, close}: {strip: FrontendStrip; close: () => void}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const airport = useWebSocketStore(s => s.airport);
  const capabilities = useWebSocketStore(s => s.boardDefinition?.capabilities);
  const ready = useWebSocketStore(s => s.cdmReady);
  const updateTobt = useWebSocketStore(s => s.clxUpdateTobt);
  const textInputs = useWebSocketStore(s => s.textPilotInputs);
  const openText = useWebSocketStore(s => s.openTextPilotMenu);
  const [tobt, setTobt] = useState(strip.tobt ?? "");
  const blocked = readOnly || !strip.has_fp;
  return <fieldset><legend>SERVICES</legend>
    {capabilities?.cdm && strip.origin === airport && <div className="radis-service-controls">
      <span>TSAT {strip.tsat || "—"} · TTOT {strip.ttot || "—"} · CTOT {strip.ctot || "—"}</span>
      <Button disabled={blocked} onClick={() => ready(strip.callsign)}>CDM READY</Button>
      <label>TOBT (UTC HHMM)<input aria-label="CDM TOBT" value={tobt} maxLength={4} disabled={blocked} onChange={e => setTobt(e.target.value)}/></label>
      <Button disabled={blocked || !/^(?:[01]\d|2[0-3])[0-5]\d$/.test(tobt)} onClick={() => updateTobt(strip.callsign, tobt)}>SET TOBT</Button>
    </div>}
    {capabilities?.text_pilot && textInputs?.airport === airport && <Button disabled={blocked} onClick={() => {close(); openText({callsign: strip.callsign, variant: "si"});}}>TEXT PILOT</Button>}
    {!capabilities?.cdm && !capabilities?.text_pilot && <span>No services enabled.</span>}
  </fieldset>;
}

export function RADISStandDialog({strip, close}: {strip: FrontendStrip; close: () => void}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const enabled = useWebSocketStore(s => s.satEnabled && !!s.boardDefinition?.capabilities.stands);
  const assignment = useWebSocketStore(s => s.standAssignments.find(a => a.callsign === strip.callsign));
  const rejection = useWebSocketStore(s => s.standActionRejection);
  const manual = useWebSocketStore(s => s.requestManualStand);
  const automatic = useWebSocketStore(s => s.requestAutomaticStand);
  const override = useWebSocketStore(s => s.confirmStandOverride);
  const [reason, setReason] = useState("");
  const acknowledge = useWebSocketStore(s => s.acknowledgeStandAssignment);
  const [version] = useState(assignment?.version ?? 0);
  const [stand, setStand] = useState(assignment?.stand ?? strip.stand);
  const changed = (assignment?.version ?? 0) !== version;
  const blocked = readOnly || !enabled || !strip.has_fp || changed;
  return <Dialog open onOpenChange={open => !open && close()}><DialogContent className="radis-dialog"><DialogTitle>Stand · {strip.callsign}</DialogTitle>
    {!enabled && <p role="status">Stand service unavailable.</p>}
    <p>Assigned: {assignment?.stand || strip.stand || "—"} · {assignment?.stage || "unassigned"}</p>
    {changed && <p role="status">Stand assignment changed. Close and reopen to make another change.</p>}
    <label>Stand<input aria-label="Airport stand" value={stand} disabled={blocked} onChange={e => setStand(e.target.value.toUpperCase())}/></label>
    {rejection?.callsign === strip.callsign && <p role="alert">{rejection.reason}</p>}
    {rejection?.callsign === strip.callsign && rejection.code === "incompatible_or_occupied" && <label>Override reason<input aria-label="Stand override reason" value={reason} disabled={blocked} onChange={e => setReason(e.target.value)}/></label>}
    <div className="radis-dialog-actions">
      <Button disabled={blocked} onClick={() => automatic(strip.callsign, version)}>AUTO</Button>
      <Button disabled={blocked || !stand.trim()} onClick={() => manual(strip.callsign, stand.trim(), version)}>ASSIGN STAND</Button>
      {rejection?.callsign === strip.callsign && rejection.code === "incompatible_or_occupied" && <Button disabled={blocked || !stand.trim() || !reason.trim()} onClick={() => override(strip.callsign, stand.trim(), version, reason.trim())}>CONFIRM OVERRIDE</Button>}
      {assignment?.pending_acknowledgement && <Button disabled={blocked} onClick={() => acknowledge(strip.callsign, version)}>ACKNOWLEDGE</Button>}
      <Button onClick={close}>DISMISS</Button>
    </div>
  </DialogContent></Dialog>;
}
