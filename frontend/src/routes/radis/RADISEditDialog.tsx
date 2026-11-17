import {useState, type Dispatch, type SetStateAction} from "react";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";
import {useWebSocketStore} from "@/store/store-hooks";
import {formatAltitude} from "@/lib/utils";
import type {RADISEdit} from "./RADISStrip";

const ALTITUDES = [1500, 2500, 3000, 4000, 5000, 7000];
const HEADINGS = [360, 90, 120, 300, 40, 220];

export function RADISEditDialog({menu, setMenu, submit}: {
  menu: RADISEdit | null;
  setMenu: Dispatch<SetStateAction<RADISEdit | null>>;
  submit: (value?: string) => void;
}) {
  const readOnly = useWebSocketStore(s => s.readOnly);
  const runways = useWebSocketStore(s => s.boardDefinition?.runways);
  const sids = useWebSocketStore(s => s.availableSids);
  const transitionAltitude = useWebSocketStore(s => s.transitionAltitude);
  const [customEntry, setCustomEntry] = useState(false);
  const [originalValue] = useState(menu?.value ?? "");
  const field = menu?.field;
  const altitude = field === "altitude";
  const heading = field === "heading" || field === "route_heading";
  const runway = field === "runway";
  const eobt = field === "eobt";
  const clearValue = altitude || field === "heading" || (field === "route_heading" && /^\d{1,3}$/.test(originalValue)) ? "0" : "";
  const keypad = !!menu && ["assigned_squawk", "atd", "endurance_pob"].includes(menu.field);
  const presets = altitude ? ALTITUDES.map(String) : heading ? HEADINGS.map(String) : runway ? runways : field === "sid" ? sids.map(s => s.name) : undefined;
  const selectedPreset = presets?.includes(menu?.value ?? "");
  const mode = altitude ? "altitude" : heading ? "heading" : runway ? "runway" : eobt ? "eobt" : keypad ? "keypad" : "text";
  const setValue = (value: string) => {setCustomEntry(true); setMenu(m => m && {...m, value});};
  const append = (key: string) => setMenu(m => m && {...m, value: m.value + key});
  const choose = (value: string) => {if (!readOnly) submit(value);};
  const close = () => {setCustomEntry(false); setMenu(null);};
  const input = <><label className="sr-only" htmlFor="radis-menu-value">{menu?.title}</label><input id="radis-menu-value" autoFocus={!altitude && !heading} inputMode={altitude || keypad || eobt || field === "heading" ? "numeric" : "text"} value={((altitude || heading) && selectedPreset && !customEntry) || (heading && menu?.value === "0") ? "" : menu?.value ?? ""} onChange={e => setValue(e.target.value)} disabled={readOnly}/></>;

  return <Dialog open={!!menu} onOpenChange={open => !open && close()}>
    <DialogContent className={`radis-dialog radis-edit-dialog radis-edit-${mode}`}>
      <DialogTitle className="sr-only">{menu?.title}</DialogTitle>
      <form onSubmit={e => {e.preventDefault(); submit(menu?.value === "" ? clearValue : undefined);}}>
        <div className="radis-edit-panel">
          {eobt && <div className="radis-edit-caption">EOBT</div>}
          {presets && <div className="radis-menu-presets">{presets.map(value => <Button key={value} variant="trf" type="button" aria-pressed={menu?.value === value} disabled={readOnly} onClick={() => choose(value)}>{altitude ? formatAltitude(Number(value), transitionAltitude) : heading ? value.padStart(3, "0") : value}</Button>)}</div>}
          {!runway && <div className={keypad ? "radis-keypad" : "radis-edit-value"}>
            {keypad && ["7", "8", "9", "4", "5", "6", "1", "2", "3", "0"].map(key => <Button variant="trf" type="button" key={key} disabled={readOnly} onClick={() => append(key)}>{key}</Button>)}
            {input}
          </div>}
          {!eobt && <div className="radis-dialog-actions">
            {(altitude || heading) && <Button variant="darkaction" type="button" disabled={readOnly} onClick={() => choose(clearValue)}>ERASE</Button>}
            <Button variant="darkaction" type="button" onClick={close}>ESC</Button>
            {!runway && !altitude && !heading && <Button variant="darkaction" type="submit" disabled={readOnly}>OK</Button>}
            {(altitude || heading) && customEntry && <Button className="radis-custom-submit" variant="darkaction" type="submit" disabled={readOnly}>OK</Button>}
          </div>}
        </div>
        {eobt && <Button variant="darkaction" className="radis-eobt-submit" type="submit" disabled={readOnly}>OK</Button>}
      </form>
    </DialogContent>
  </Dialog>;
}
