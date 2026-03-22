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
  useTaxiDepStrips,
  useTaxiDepLwrStrips,
  useDeIceStrips,
  useAirborneStrips,
  useDepartStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, FrontendStrip, StripRef } from "@/api/models.ts";
import { Bay, stripDndId } from "@/api/models.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useMessages, useDelOnline, useApronOnline } from "@/store/store-hooks.ts";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { useState } from "react";
import { CLX_CLEARED_STRIP_WIDTH } from "@/components/strip/ClxClearedStrip.tsx";
import { CLS_BTN, CLS_BTN_ORANGE, CLS_BTN_BLUE, CLS_BTN_YELLOW, CLS_SCROLLBAR, CLS_COL } from "@/components/strip/shared";
import { NewIfrDialog } from "@/components/strip/NewIfrDialog";
import { PlannedDialog } from "@/components/strip/PlannedDialog";

// Column widths
const W_COL_ARR      = "w-[27%]";
const W_COL_DEP      = "w-[28%]";
const W_COL_CLRDEL   = "w-1/4";
const W_COL_STAND    = "w-1/5";

const pageWrapper  = "bg-[#A9A9A9] w-screen h-[calc(100vh-60px)] flex justify-center justify-items-center gap-2";
const header       = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const label        = "text-white font-bold text-lg";
const colSep       = "border-t-4 border-[#A9A9A9]";
const scrollArea     = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const darkScrollArea = `w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const [plannedOpen, setPlannedOpen] = useState(false);

  const finalStrips    = useFinalStrips();
  const rwyArrStrips   = useRwyArrStrips();
  const twyArrStrips   = useTaxiArrStrips();
  const pushStrips     = usePushbackStrips();
  const startupStrips  = useClearedStrips();
  const twyDepUpr      = useTaxiDepStrips();
  const twyDepLwr      = useTaxiDepLwrStrips();
  const rwyDepStrips   = useDepartStrips();
  const airborneStrips = useAirborneStrips();
  const deIceStrips    = useDeIceStrips();
  const standStrips    = useStandStrips();

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
    "STARTUP":     { strips: startupStrips,  targetBay: Bay.Cleared },
    "PUSHBACK":    { strips: pushStrips,      targetBay: Bay.Push },
    "TWY-DEP-UPR": { strips: twyDepUpr,      targetBay: Bay.Taxi },
    "TWY-DEP-LWR": { strips: twyDepLwr,      targetBay: Bay.TaxiLwr },
    "RWY-DEP":     { strips: rwyDepStrips,   targetBay: Bay.Depart },
    "AIRBORNE":    { strips: airborneStrips, targetBay: Bay.Airborne },
    "DE-ICE":      { strips: deIceStrips,    targetBay: Bay.DeIce },
    "STAND":       { strips: standStrips,    targetBay: Bay.Stand },
  };

  const transferRules: Record<string, string[]> = {
    "STARTUP":     ["PUSHBACK", "TWY-DEP-UPR", "TWY-DEP-LWR", "DE-ICE", "STAND"],
    "PUSHBACK":    ["STARTUP",  "TWY-DEP-UPR", "TWY-DEP-LWR", "DE-ICE", "STAND"],
    "TWY-DEP-UPR": ["STARTUP",  "PUSHBACK",    "TWY-DEP-LWR", "DE-ICE", "STAND"],
    "TWY-DEP-LWR": ["STARTUP",  "PUSHBACK",    "TWY-DEP-UPR", "DE-ICE", "STAND"],
    "RWY-DEP":     ["TWY-DEP-UPR", "TWY-DEP-LWR", "AIRBORNE"],
    "AIRBORNE":    ["RWY-DEP"],
    "DE-ICE":      ["STARTUP",  "PUSHBACK",    "TWY-DEP-UPR", "TWY-DEP-LWR"],
    "STAND":       ["STARTUP",  "PUSHBACK",    "TWY-DEP-UPR", "TWY-DEP-LWR"],
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
        if (strip.bay === Bay.Taxi)      return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.TaxiLwr)   return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.Depart)    return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.Airborne)  return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.DeIce)     return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand)     return <Strip strip={strip} status="ARR" myPosition={myPosition} />;
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
        <DropIndicatorBay bayId="FINAL" className={`h-[25%] ${scrollArea}`}>
          {finalStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep}`}>
          <span className={label}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className={`h-[20%] ${darkScrollArea}`}>
          {rwyArrStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.TwyArr} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.TwyArr} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.TwyArr} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <DropIndicatorBay bayId="TWY-ARR" className={`flex-1 ${scrollArea}`}>
          {twyArrStrips.map(s => (
            <Strip key={stripDndId(s)} strip={s} status="FINAL-ARR" myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

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
          className={`h-[12%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${header} ${colSep} justify-between`}>
          <span className={label}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={CLS_BTN} onClick={() => setNewOpen(true)}>NEW</button>
            <MemAidButton bay={Bay.Taxi} className={CLS_BTN_BLUE} />
            <LandButton bay={Bay.Taxi} className={CLS_BTN_ORANGE} />
            <StartButton bay={Bay.Taxi} className={CLS_BTN_ORANGE} />
            <CrossingButton bay={Bay.Taxi} className={CLS_BTN_YELLOW} />
          </span>
        </div>
        <SortableBay
          strips={twyDepUpr}
          bayId="TWY-DEP-UPR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[30%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>
        <SortableBay
          strips={twyDepLwr}
          bayId="TWY-DEP-LWR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[15%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
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
          className={`h-[15%] ${darkScrollArea}`}
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
          className={`flex-1 ${darkScrollArea}`}
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
          className={`h-[33%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`bg-primary h-10 flex items-center px-2 shrink-0 justify-between ${colSep}`}>
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
          className={`flex-1 ${scrollArea}`}
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
