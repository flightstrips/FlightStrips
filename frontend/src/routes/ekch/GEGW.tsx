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
import { CLS_BTN, CLS_SCROLLBAR, CLS_COL } from "@/components/strip/shared";

// Column widths
const W_COL_ARR      = "w-[27%]";
const W_COL_DEP      = "w-[28%]";
const W_COL_CLRDEL   = "w-1/4";
const W_COL_STAND    = "w-1/5";

const pageWrapper  = "bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2";
const activeHeader = "bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0";
const activeLabel  = "text-[#393939] font-bold text-lg";
const lockedHeader = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const lockedLabel  = "text-white font-bold text-lg";
const scrollArea   = `w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;
const darkScrollArea = `w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto ${CLS_SCROLLBAR}`;

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);

  const finalStrips   = useFinalStrips();
  const rwyArrStrips  = useRwyArrStrips();
  const twyArrStrips  = useTaxiArrStrips();
  const pushStrips    = usePushbackStrips();
  const startupStrips = useClearedStrips();
  const twyDepUpr     = useTaxiDepStrips();
  const twyDepLwr     = useTaxiDepLwrStrips();
  const standStrips   = useStandStrips();

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
    "STARTUP":     { strips: startupStrips, targetBay: Bay.Cleared },
    "PUSHBACK":    { strips: pushStrips,    targetBay: Bay.Push },
    "TWY-DEP-UPR": { strips: twyDepUpr,    targetBay: Bay.Taxi },
    "TWY-DEP-LWR": { strips: twyDepLwr,    targetBay: Bay.TaxiLwr },
    "STAND":       { strips: standStrips,   targetBay: Bay.Stand },
  };

  const transferRules: Record<string, string[]> = {
    "STARTUP":     ["PUSHBACK", "TWY-DEP-UPR", "TWY-DEP-LWR", "STAND"],
    "PUSHBACK":    ["STARTUP",  "TWY-DEP-UPR", "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-UPR": ["STARTUP",  "PUSHBACK",    "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-LWR": ["STARTUP",  "PUSHBACK",    "TWY-DEP-UPR", "STAND"],
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
        if (strip.bay === Bay.Cleared)  return <Strip strip={strip} status="PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Push)     return <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Taxi)     return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.TaxiLwr) return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand)   return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        return null;
      }}
    >
    <div className={pageWrapper}>

      {/* Column 1 (27%) – MESSAGES + FINAL + RWY ARR + TWY ARR */}
      <div className={`${W_COL_ARR} ${CLS_COL}`}>
        <div className="bg-primary h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">MESSAGES</span>
          <button className={CLS_BTN} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
        </div>
        <div className={`h-[15%] ${scrollArea}`}>
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />

        <div className={`${lockedHeader} justify-between`}>
          <span className={lockedLabel}>FINAL</span>
          <button className={CLS_BTN} onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <DropIndicatorBay bayId="FINAL" className={`h-[25%] ${scrollArea}`}>
          {finalStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className={`h-[20%] ${darkScrollArea}`}>
          {rwyArrStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        {/* TWY ARR is SI-only; no manual drag */}
        <div className={`${lockedHeader} justify-between`}>
          <span className={lockedLabel}>TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.TwyArr} className={CLS_BTN} />
            <CrossingButton bay={Bay.TwyArr} className={CLS_BTN} />
            <StartButton bay={Bay.TwyArr} className={CLS_BTN} />
            <LandButton bay={Bay.TwyArr} className={CLS_BTN} />
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

      {/* Column 2 (28%) – STARTUP + PUSHBACK + TWY DEP UPR + TWY DEP LWR (all draggable) */}
      <div className={`${W_COL_DEP} ${CLS_COL}`}>
        <div className={activeHeader}>
          <span className={activeLabel}>STARTUP</span>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[15%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={activeHeader}>
          <span className={activeLabel}>PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[15%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className={`${activeHeader} justify-between`}>
          <span className={activeLabel}>TWY DEP UPR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.Taxi} className={CLS_BTN} />
            <CrossingButton bay={Bay.Taxi} className={CLS_BTN} />
            <StartButton bay={Bay.Taxi} className={CLS_BTN} />
            <LandButton bay={Bay.Taxi} className={CLS_BTN} />
          </span>
        </div>
        <SortableBay
          strips={twyDepUpr}
          bayId="TWY-DEP-UPR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`h-[35%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className={activeHeader}>
          <span className={activeLabel}>TWY DEP LWR</span>
        </div>
        <SortableBay
          strips={twyDepLwr}
          bayId="TWY-DEP-LWR"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – CLRDEL: active when CTWR owns clearances (no DEL, no APRON online) */}
      <div className={`${W_COL_CLRDEL} ${CLS_COL}`}>
        <div className={clrDelActive ? activeHeader : lockedHeader}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>CLRDEL</span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {clrDelActive && nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} />
          ))}
        </div>
      </div>

      {/* Column 4 (20%) – STAND (draggable) */}
      <div className={`${W_COL_STAND} ${CLS_COL}`}>
        <div className={lockedHeader}>
          <span className={lockedLabel}>STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          isDragDisabled={(strip) => isFlight(strip) && !!strip.owner && strip.owner !== myPosition}
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>
      </div>

    </div>
    </ViewDndContext>
  );
}
