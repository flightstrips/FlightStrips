import {RADIS_BAYS, type RADISBay} from "./bays";
import {useState, type CSSProperties, type ReactNode} from "react";
import {DndContext, PointerSensor, KeyboardSensor, useSensor, useSensors, useDroppable, type DragEndEvent} from "@dnd-kit/core";
import {SortableContext, verticalListSortingStrategy, sortableKeyboardCoordinates} from "@dnd-kit/sortable";
import type {FrontendStrip} from "@/api/models";
import { stripRevision, type StripRevision, type StripAnnotations} from "@/api/strip-board";
import {useWebSocketStore} from "@/store/store-hooks";
import {Dialog, DialogContent, DialogTitle} from "@/components/ui/dialog";
import {Button} from "@/components/ui/button";
import {AirportSwitcher} from "@/components/AirportSwitcher";
import Time from "@/components/Time";
import {RADISStrip, type RADISEdit} from "./RADISStrip";
import {RADISFlightPlan} from "./RADISFlightPlan";
import "./radis.css";

function RADISBayPanel({bay, strips, children}: {bay: RADISBay; strips: FrontendStrip[]; children: ReactNode}) {
  const {setNodeRef, isOver} = useDroppable({id: bay});
  return <section ref={setNodeRef} aria-label={bay.replaceAll("_", " ")} className={`radis-bay radis-${bay.toLowerCase()} ${isOver ? "is-over" : ""}`}>
    <SortableContext items={strips.map(s => s.callsign)} strategy={verticalListSortingStrategy}>{children}</SortableContext>
  </section>;
}

export default function RADISBoard() {
  const strips = useWebSocketStore(s => s.strips);
  const readOnly = useWebSocketStore(s => s.readOnly);
  const definition = useWebSocketStore(s => s.boardDefinition);
  const [dragRevision,setDragRevision]=useState<StripRevision|null>(null);
  const move = useWebSocketStore(s => s.moveBoardStrip);
  const update = useWebSocketStore(s => s.updateBoardStrip);
  const annotate = useWebSocketStore(s => s.annotateStrip);
  const availableSids = useWebSocketStore(s => s.availableSids);
  const runway = useWebSocketStore(s => s.runwaySetup);
  const metar = useWebSocketStore(s => s.metar);
  const atis = useWebSocketStore(s => s.depAtisCode);
  const [menu, setMenu] = useState<RADISEdit | null>(null);
  const [fpl, setFpl] = useState<string | null>(null);
  const [runwayHeight, setRunwayHeight] = useState(28);
  const sensors = useSensors(useSensor(PointerSensor, {activationConstraint: {distance: 6}}), useSensor(KeyboardSensor, {coordinateGetter: sortableKeyboardCoordinates}));
  const byBay = (bay: string) => strips.filter(s => s.bay === bay && !s.board_state?.removed_at).sort((a, b) => a.sequence - b.sequence || (a.id ?? 0) - (b.id ?? 0));
  const edit = (strip: FrontendStrip, field: RADISEdit["field"], title: string, value: string) => setMenu({callsign: strip.callsign, field, title, value, revision:stripRevision(strip,definition)??undefined});
  const selected = strips.find(s => s.callsign === fpl);
  const dragEnd = ({active, over}: DragEndEvent) => {
    if (readOnly || !over || active.id === over.id) return;
    const targetStrip = strips.find(s => s.callsign === over.id);
    const bay = targetStrip?.bay ?? String(over.id);
    if (!RADIS_BAYS.includes(bay as RADISBay)) return;
    const ordered = byBay(bay).filter(s => s.callsign !== active.id);
    let index = targetStrip ? ordered.findIndex(s => s.callsign === targetStrip.callsign) : ordered.length;
    const original = byBay(bay);
    if (targetStrip && original.findIndex(s => s.callsign === active.id) >= 0 && original.findIndex(s => s.callsign === active.id) < original.findIndex(s => s.callsign === targetStrip.callsign)) index++;
    move(String(active.id), bay as RADISBay, index > 0 ? ordered[index - 1].callsign : null,dragRevision??undefined);
    setDragRevision(null);
  };
  const submit = () => {
    if (!menu || readOnly) return;
    const {callsign, field, value} = menu;
    if (["notes", "atd", "endurance_pob"].includes(field)) annotate(callsign, field as keyof StripAnnotations, value,false,menu.revision);
    else if (field === "route_heading") update(callsign, /^\d{1,3}$/.test(value) ? {heading: Number(value)} : {route: value.toUpperCase()},menu.revision);
    else update(callsign, {[field]: ["altitude", "heading"].includes(field) ? Number(value) : value},menu.revision);
    setMenu(null);
  };
  const renderBay = (bay: RADISBay) => {const list = byBay(bay); return <RADISBayPanel bay={bay} strips={list}>{list.map(strip => <RADISStrip key={strip.id ?? strip.callsign} strip={strip} open={() => setFpl(strip.callsign)} edit={(field, title, value) => edit(strip, field, title, value)}/>)}</RADISBayPanel>;};
  return <div className="radis-view" style={{"--radis-runway-height": `${runwayHeight}dvh`} as CSSProperties}>
    <DndContext sensors={sensors} onDragStart={({active})=>setDragRevision(stripRevision(strips.find(s=>s.callsign===active.id),definition))} onDragCancel={()=>setDragRevision(null)} onDragEnd={dragEnd}>
      <main className="radis-board" aria-label="RADIS board">
        <div className="radis-left"><h1>ACTIVE</h1>{renderBay("ACTIVE")}<h2 className="radis-runway-title">RUNWAY <input type="range" min="15" max="60" value={runwayHeight} onChange={e => setRunwayHeight(Number(e.target.value))} aria-label="Runway area height"/></h2>{renderBay("RUNWAY")}</div>
        <div className="radis-right"><h2>PASSIVE</h2>{renderBay("PASSIVE_TOP")}{renderBay("PASSIVE_BOTTOM")}</div>
      </main>
    </DndContext>
    <footer className="radis-commandbar"><AirportSwitcher/><span>DEP</span><b>{runway.departure.join("/") || "—"}</b><span>ARR</span><b>{runway.arrival.join("/") || "—"}</b><span>QNH</span><b>{metar?.match(/\bQ(\d{4})\b/)?.[1] ?? "—"}</b><b>{atis || "—"}</b><b>{metar?.match(/\b(?:\d{3}|VRB)\d{2}(?:G\d{2})?KT\b/)?.[0] ?? "—"}</b><span className="radis-toolbar-space"/>{readOnly && <span role="status">READ ONLY</span>}<div className="radis-clock"><Time/></div></footer>
    <Dialog open={!!menu} onOpenChange={open => !open && setMenu(null)}><DialogContent className="radis-dialog"><DialogTitle>{menu?.title}</DialogTitle><form onSubmit={e => {e.preventDefault(); submit();}}><label className="sr-only" htmlFor="radis-menu-value">{menu?.title}</label><input id="radis-menu-value" autoFocus value={menu?.value ?? ""} onChange={e => setMenu(m => m && {...m, value: e.target.value})} disabled={readOnly}/>
      {menu?.field === "runway" && <div className="radis-menu-presets">{definition?.runways?.map(r => <Button key={r} type="button" disabled={readOnly} onClick={() => setMenu({...menu, value: r})}>{r}</Button>)}</div>}
      {menu?.field === "sid" && <div className="radis-menu-presets">{availableSids.map(s => <Button key={s.name} type="button" disabled={readOnly} onClick={() => setMenu({...menu, value: s.name})}>{s.name}</Button>)}</div>}
      {menu && ["assigned_squawk", "altitude", "heading", "route_heading", "atd", "endurance_pob"].includes(menu.field) && <div className="radis-keypad">{["1","2","3","4","5","6","7","8","9","0",".","/"].map(key => <Button type="button" key={key} disabled={readOnly} onClick={() => setMenu(m => m && {...m,value:m.value+key})}>{key}</Button>)}<Button type="button" disabled={readOnly} onClick={() => setMenu(m => m && {...m,value:m.value.slice(0,-1)})}>BACK</Button><Button type="button" disabled={readOnly} onClick={() => setMenu(m => m && {...m,value:""})}>ERASE</Button></div>}
      <div className="radis-dialog-actions"><Button type="button" onClick={() => setMenu(null)}>ESC</Button><Button type="submit" disabled={readOnly}>OK</Button></div></form></DialogContent></Dialog>
    <RADISFlightPlan strip={selected} onClose={() => setFpl(null)} edit={edit}/>
  </div>;
}
