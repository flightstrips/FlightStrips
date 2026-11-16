import type {Dispatch, SetStateAction} from "react";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";
import {useWebSocketStore} from "@/store/store-hooks";
import type {RADISEdit} from "./RADISStrip";

export function RADISEditDialog({menu, setMenu, submit}: {
  menu: RADISEdit | null;
  setMenu: Dispatch<SetStateAction<RADISEdit | null>>;
  submit: () => void;
}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const runways = useWebSocketStore(s => s.boardDefinition?.runways);
  const sids = useWebSocketStore(s => s.availableSids);
  const numeric = !!menu && ["assigned_squawk", "altitude", "heading", "eobt", "atd", "endurance_pob"].includes(menu.field);
  const presets = menu?.field === "runway" ? runways : menu?.field === "sid" ? sids.map(s => s.name) : undefined;
  const setValue = (value: string) => setMenu(m => m && {...m, value});
  const append = (key: string) => setMenu(m => m && {...m, value: m.value + key});

  return <Dialog open={!!menu} onOpenChange={open => !open && setMenu(null)}>
    <DialogContent className={`radis-dialog radis-edit-dialog ${numeric ? "radis-numeric-dialog" : ""}`}>
      <DialogTitle className="sr-only">{menu?.title}</DialogTitle>
      <form onSubmit={e => {e.preventDefault(); submit();}}>
        <div className="radis-edit-panel">
          {presets && <div className="radis-menu-presets">{presets.map(value => <Button key={value} variant="trf" type="button" aria-pressed={menu?.value === value} disabled={readOnly} onClick={() => setValue(value)}>{value}</Button>)}</div>}
          <div className={numeric ? "radis-keypad" : "radis-edit-value"}>
            {numeric && ["7", "8", "9", "4", "5", "6", "1", "2", "3", "0"].map(key => <Button variant="trf" type="button" key={key} disabled={readOnly} onClick={() => append(key)}>{key}</Button>)}
            <label className="sr-only" htmlFor="radis-menu-value">{menu?.title}</label>
            <input id="radis-menu-value" autoFocus inputMode={numeric ? "decimal" : "text"} value={menu?.value ?? ""} onChange={e => setValue(e.target.value)} disabled={readOnly}/>
          </div>
          {numeric && <div className="radis-keypad-corrections">
            {menu?.field === "endurance_pob" && <Button variant="trf" type="button" disabled={readOnly} onClick={() => append("/")}>/</Button>}
            <Button variant="trf" type="button" disabled={readOnly} onClick={() => setMenu(m => m && {...m, value: m.value.slice(0, -1)})}>BACK</Button>
            <Button variant="trf" type="button" disabled={readOnly} onClick={() => setValue("")}>ERASE</Button>
          </div>}
          <div className="radis-dialog-actions"><Button variant="darkaction" type="button" onClick={() => setMenu(null)}>ESC</Button><Button variant="darkaction" type="submit" disabled={readOnly}>OK</Button></div>
        </div>
      </form>
    </DialogContent>
  </Dialog>;
}
