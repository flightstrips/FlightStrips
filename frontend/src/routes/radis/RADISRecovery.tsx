import {useEffect, useState} from "react";
import {useDroppable} from "@dnd-kit/core";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";

export const RADIS_DELETE = "RADIS_REMOVE";
export function RADISDeleteTarget() {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const selected = useWebSocketStore(s => s.selectedCallsign);
  const remove = useWebSocketStore(s => s.removeStripBoardEntry);
  const {setNodeRef, isOver} = useDroppable({id: RADIS_DELETE, disabled: readOnly});
  return <Button ref={setNodeRef} type="button" aria-label="Remove selected strip" disabled={readOnly} className={isOver ? "radis-delete-over" : ""} onClick={() => selected && remove(selected)}>X</Button>;
}
export function RADISRecovery({close}: {close: () => void}) {
  const strips = useWebSocketStore(s => s.strips);
  const seconds = useWebSocketStore(s => s.boardDefinition?.board_state?.recovery_seconds);
  const readOnly = useWebSocketStore(s => s.readOnly);
  const restore = useWebSocketStore(s => s.restoreStripBoardEntry);
  const [now, setNow] = useState(Date.now);
  useEffect(() => {const timer = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(timer);}, []);
  const removed = strips.filter(s => s.board_state?.removed_at).sort((a, b) => (b.board_state?.removed_at ?? "").localeCompare(a.board_state?.removed_at ?? ""));
  return <Dialog open onOpenChange={open => !open && close()}><DialogContent className="radis-dialog radis-creation"><DialogTitle>Restore airport strips</DialogTitle><div className="radis-find-list">{removed.map(s => {
    const at = Date.parse(s.board_state!.removed_at!);
    const eligible = seconds !== undefined && now >= at && now < at + seconds * 1000;
    return <button key={s.id} type="button" disabled={readOnly || !eligible} aria-label={`Restore ${s.callsign}`} onClick={() => {if (restore(s.callsign)) close();}}><strong>{s.callsign}</strong><span>{s.board_state?.removal_reason}</span><span>{eligible ? "RESTORE" : "EXPIRED"}</span></button>;
  })}</div>{removed.length === 0 && <p>No removed strips.</p>}<Button onClick={close}>DISMISS</Button></DialogContent></Dialog>;
}
