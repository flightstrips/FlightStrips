import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {
  useClearedStrips,
  useFinalStrips,
  useInboundStrips,
  useNonClearedStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepLwrStrips,
  useDeIceStrips,
  useAirborneStrips,
  useDepartStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, FrontendStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useMessages, useDelOnline, useApronOnline } from "@/store/store-hooks.ts";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { useState } from "react";
import { CLX_CLEARED_STRIP_WIDTH } from "@/components/strip/ClxClearedStrip.tsx";
import { TWY_DEP_STRIP_WIDTH } from "@/components/strip/types";
import { CLS_BTN, CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_SCROLLBAR, CLS_COL, CLS_HEADER_SHADOW } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

// Column widths
const W_COL_ARR      = "w-[27%]";
const W_COL_DEP      = "w-[28%]";
const W_COL_CLRDEL   = "w-1/4";
const W_COL_STAND    = "w-1/5";

const pageWrapper  = "bg-bay-border w-screen h-[95.28vh] flex divide-x-[6px] divide-bay-border border-x-2 border-t-2 border-bay-border";
const header       = `bg-[#393939] h-10 flex items-center px-2 shrink-0 ${CLS_HEADER_SHADOW}`;
const label        = "text-white font-bold text-lg";
const colSep       = "border-t-[6px] border-bay-border";
const scrollArea           = `w-full bg-[#555355] shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const scrollAreaBottom     = `w-full bg-[#555355] shadow-[inset_2px_2px_4px_rgba(0,0,0,0.55),inset_-1px_-1px_2px_rgba(255,255,255,0.07)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const darkScrollAreaBottom = `w-full bg-[#212121] shadow-[inset_3px_3px_7px_rgba(0,0,0,0.85),inset_-1px_-1px_3px_rgba(255,255,255,0.05)] p-0.5 flex flex-col justify-end gap-px overflow-y-auto ${CLS_SCROLLBAR}`;

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const finalStrips    = useFinalStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyArrStrips   = useRwyArrStrips().sort((a, b) => b.sequence - a.sequence);
  const twyArrStrips   = useTaxiArrStrips().sort((a, b) => b.sequence - a.sequence);
  const pushStrips     = usePushbackStrips().sort((a, b) => b.sequence - a.sequence);
  const startupStrips  = useClearedStrips();
  const twyDepDesc     = useTaxiDepLwrStrips().sort((a, b) => b.sequence - a.sequence);
  const rwyDepStrips   = useDepartStrips().sort((a, b) => b.sequence - a.sequence);
  const airborneStrips = useAirborneStrips().sort((a, b) => b.sequence - a.sequence);
  const deIceStrips    = useDeIceStrips().sort((a, b) => b.sequence - a.sequence);
  const standStrips    = useStandStrips().sort((a, b) => b.sequence - a.sequence);

  const inboundStrips = useInboundStrips();

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);
  const assumeStrip       = useWebSocketStore(state => state.assumeStrip);

  const arrSortModes: SortMode<FrontendStrip>[] = [
    { key: "ETA",      label: "ETA",      compareFn: (a, b) => a.eldt.localeCompare(b.eldt) },
    { key: "CALLSIGN", label: "CALLSIGN", compareFn: (a, b) => a.callsign.localeCompare(b.callsign) },
    { key: "ADEP",     label: "ADEP",     compareFn: (a, b) => a.origin.localeCompare(b.origin) },
  ];

  const delOnline   = useDelOnline();
  const apronOnline = useApronOnline();
  // CTWR is responsible for clearances only when neither DEL nor APRON is online.
  const clrDelActive = !delOnline && !apronOnline;

  const nonClearedStrips = useNonClearedStrips();

  const bayStripMap = {
    "STARTUP":  { strips: startupStrips,                    targetBay: Bay.Cleared },
    "PUSHBACK": { strips: pushStrips,                       targetBay: Bay.Push,      descending: true },
    "TWY-DEP":  { strips: twyDepDesc,                       targetBay: Bay.TaxiLwr,   descending: true },
    "RWY-DEP":  { strips: rwyDepStrips,                     targetBay: Bay.Depart,    descending: true },
    "AIRBORNE": { strips: airborneStrips,                   targetBay: Bay.Airborne,  descending: true },
    "DE-ICE":   { strips: deIceStrips,                      targetBay: Bay.DeIce,     descending: true },
    "STAND":    { strips: standStrips,                      targetBay: Bay.Stand,     descending: true },
    "FINAL":    { strips: finalStrips.filter(isFlight),     targetBay: Bay.Final,     descending: true },
    "RWY-ARR":  { strips: rwyArrStrips.filter(isFlight),    targetBay: Bay.RwyArr,    descending: true },
    "TWY-ARR":  { strips: twyArrStrips,                     targetBay: Bay.TwyArr,    descending: true },
  };

  const transferRules: Record<string, string[]> = {
    "STARTUP":  ["PUSHBACK", "TWY-DEP", "DE-ICE", "STAND"],
    "PUSHBACK": ["STARTUP",  "TWY-DEP", "DE-ICE", "STAND"],
    "TWY-DEP":  ["STARTUP",  "PUSHBACK", "DE-ICE", "STAND"],
    "RWY-DEP":  ["TWY-DEP",  "AIRBORNE"],
    "AIRBORNE": ["RWY-DEP"],
    "DE-ICE":   ["STARTUP",  "PUSHBACK", "TWY-DEP"],
    "STAND":    ["STARTUP",  "PUSHBACK", "TWY-DEP"],
  };

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={(activeRef: StripRef, insertAfter: StripRef | null) => {
        if (activeRef.kind === "tactical") moveTacticalStrip(activeRef.id!, insertAfter);
        else updateOrder(activeRef.callsign!, insertAfter);
      }}
      onMove={(callsign, bay) => move(callsign, bay)}
      renderDragOverlay={(strip: AnyStrip) => {
        if (!isFlight(strip)) return <Strip strip={strip} width={CLX_CLEARED_STRIP_WIDTH} />;
        if (strip.bay === Bay.Cleared)   return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Push)      return <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.TaxiLwr)   return <div style={{ width: TWY_DEP_STRIP_WIDTH }}><Strip strip={strip} status="TWY-DEP" myPosition={myPosition} fullWidth /></div>;
        if (strip.bay === Bay.Depart)    return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.Airborne)  return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.DeIce)     return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand)     return <Strip strip={strip} status="ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.Final)     return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.RwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        if (strip.bay === Bay.TwyArr)    return <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />;
        return null;
      }}
    >
    <div className={pageWrapper}>

      {/* Column 1 (27%) – FINAL + RWY ARR + TWY ARR */}
      <div className={`${W_COL_ARR} ${CLS_COL}`}>
        <div className={`${header} justify-between`}>
          <span className={label}>FINAL</span>
          <button className={CLS_BTN} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <SortableBay
          strips={finalStrips.filter(isFlight)}
          bayId="FINAL"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[25%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>RWY ARR</span>
        </div>
        <SortableBay
          strips={rwyArrStrips.filter(isFlight)}
          bayId="RWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[20%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.TwyArr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TwyArr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="FINAL-ARR" myPosition={myPosition} />
          )}
        </SortableBay>

        {arrOpen && (
          <StripListPopup
            title="ARR"
            strips={inboundStrips}
            sortModes={arrSortModes}
            onRowClick={(strip) => {
              move(strip.callsign, Bay.Final);
              assumeStrip(strip.callsign);
              setArrOpen(false);
            }}
            onDismiss={() => setArrOpen(false)}
            myPosition={myPosition}
          />
        )}
      </div>

      {/* Column 2 (28%) – PUSHBACK + TWY DEP + RWY DEP + AIRBORNE */}
      <div className={`${W_COL_DEP} ${CLS_COL}`}>
        <div className={header}>
          <span className={label}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[12%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
            <MemAidButton bay={Bay.TaxiLwr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TaxiLwr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TaxiLwr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyDepDesc}
          bayId="TWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TWY-DEP" myPosition={myPosition} width={TWY_DEP_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>RWY DEP</span>
        </div>
        <SortableBay
          strips={rwyDepStrips}
          bayId="RWY-DEP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[15%] ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>AIRBORNE</span>
        </div>
        <SortableBay
          strips={airborneStrips}
          bayId="AIRBORNE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${darkScrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – STARTUP + DE-ICE A + MESSAGES */}
      <div className={`${W_COL_CLRDEL} ${CLS_COL}`}>
        <div className={`${header} justify-between`}>
          <span className={label}>STARTUP</span>
          <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[33%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>DE-ICE A</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[33%] ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`bg-primary h-10 flex items-center px-2 shrink-0 justify-between ${CLS_HEADER_SHADOW} ${colSep}`}>
          <span className="text-white font-bold text-lg">MESSAGES</span>
          <span className="flex gap-1">
            <button className={CLS_BTN}>INFO</button>
            <button className={CLS_BTN}>MISC.</button>
            <button className={CLS_BTN}>EQUIP</button>
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />
      </div>

      {/* Column 4 (20%) – CLRDEL + STAND */}
      <div className={`${W_COL_STAND} ${CLS_COL}`}>
        <div className={`${header} justify-between`}>
          <span className={label}>CLRDEL</span>
          <span className="flex gap-1">
            <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
            <button className={CLS_BTN} onClick={() => setPlannedOpen(true)}>PLANNED</button>
          </span>
        </div>
        <div className={`h-[75%] ${scrollArea}`}>
          {clrDelActive && nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} fullWidth />
          ))}
        </div>

        <div className={`${header} ${colSep}`}>
          <span className={label}>STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollAreaBottom}`}
        >
          {(strip) => (
            <Strip strip={strip} status="ARR" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

    </div>
    <NewIfrDialog open={newOpen} onOpenChange={setNewOpen} />
    <PlannedDialog open={plannedOpen} onOpenChange={setPlannedOpen} />
    </ViewDndContext>
  );
}
