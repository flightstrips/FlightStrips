import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton, CrossingButton, StartButton, LandButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import {
  useFinalStrips,
  useInboundStrips,
  useNonClearedStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, FrontendStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore, useMyPosition, useMessages, useDelOnline, useApronOnline } from "@/store/store-hooks.ts";
import { StripListPopup, type SortMode } from "@/components/StripListPopup.tsx";
import { useState } from "react";
import { CLX_CLEARED_STRIP_WIDTH } from "@/components/strip/ClxClearedStrip.tsx";

const activeHeader = "bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0";
const activeLabel  = "text-[#393939] font-bold text-lg";
const lockedHeader = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const lockedLabel  = "text-white font-bold text-lg";

export default function GEGW() {
  const myPosition = useMyPosition();
  const messages   = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);
  const [arrOpen, setArrOpen] = useState(false);

  const finalStrips  = useFinalStrips();
  const rwyArrStrips = useRwyArrStrips();
  const twyArrStrips = useTaxiArrStrips();
  const pushStrips   = usePushbackStrips();
  const twyDepMerged = useTaxiDepStrips();
  const standStrips  = useStandStrips();

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
    "PUSHBACK":    { strips: pushStrips,   targetBay: Bay.Push },
    "TWY-DEP-UPR": { strips: twyDepMerged, targetBay: Bay.Taxi },
    "TWY-DEP-LWR": { strips: twyDepMerged, targetBay: Bay.Taxi },
    "STAND":       { strips: standStrips,  targetBay: Bay.Stand },
  };

  const transferRules: Record<string, string[]> = {
    "PUSHBACK":    ["TWY-DEP-UPR", "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-UPR": ["PUSHBACK",    "TWY-DEP-LWR", "STAND"],
    "TWY-DEP-LWR": ["PUSHBACK",    "TWY-DEP-UPR", "STAND"],
    "STAND":       ["PUSHBACK",    "TWY-DEP-UPR", "TWY-DEP-LWR"],
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
        if (strip.bay === Bay.Push)  return <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} />;
        if (strip.bay === Bay.Taxi)  return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        if (strip.bay === Bay.Stand) return <Strip strip={strip} status="CLROK" myPosition={myPosition} />;
        return null;
      }}
    >
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* Column 1 (27%) – MESSAGES + FINAL + RWY ARR + TWY ARR */}
      <div className="w-[27%] h-full bg-[#555355] flex flex-col">
        <div className="bg-primary h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">MESSAGES</span>
          <button className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" onClick={() => setComposeOpen(true)}>FREE TEXT</button>
        </div>
        <div className="h-[15%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">FINAL</span>
          <button className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" onClick={() => setArrOpen(true)}>ARR</button>
        </div>
        <DropIndicatorBay bayId="FINAL" className="h-[25%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {finalStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className="h-[20%] w-full bg-[#212121] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {rwyArrStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        {/* TWY ARR is SI-only; no manual drag */}
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-white font-bold text-lg">TWY ARR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <CrossingButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <StartButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <LandButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
          </span>
        </div>
        <DropIndicatorBay bayId="TWY-ARR" className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {twyArrStrips.filter(isFlight).map(s => (
            <Strip key={s.callsign} strip={s} status="FINAL-ARR" myPosition={myPosition} />
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

      {/* Column 2 (28%) – PUSHBACK + TWY DEP UPR + TWY DEP LWR (all draggable) */}
      <div className="w-[28%] h-full bg-[#555355] flex flex-col">
        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">PUSHBACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          standalone={false}
          className="h-[20%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(strip) => (
            <Strip strip={strip} status="HALF" halfStripVariant="APN-PUSH" myPosition={myPosition} selectable={true} />
          )}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0 justify-between">
          <span className="text-[#393939] font-bold text-lg">TWY DEP UPR</span>
          <span className="flex gap-1">
            <MemAidButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <CrossingButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <StartButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
            <LandButton bay={Bay.Taxi} className="bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]" />
          </span>
        </div>
        <SortableBay
          strips={twyDepMerged}
          bayId="TWY-DEP-UPR"
          standalone={false}
          className="h-[35%] w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>

        <div className="bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0">
          <span className="text-[#393939] font-bold text-lg">TWY DEP LWR</span>
        </div>
        <SortableBay
          strips={twyDepMerged}
          bayId="TWY-DEP-LWR"
          standalone={false}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
        >
          {(strip) => (
            <Strip strip={strip} status="CLROK" myPosition={myPosition} width={CLX_CLEARED_STRIP_WIDTH} selectable={true} />
          )}
        </SortableBay>
      </div>

      {/* Column 3 (25%) – CLRDEL: active when CTWR owns clearances (no DEL, no APRON online) */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">
        <div className={clrDelActive ? activeHeader : lockedHeader}>
          <span className={clrDelActive ? activeLabel : lockedLabel}>CLRDEL</span>
        </div>
        <div className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary">
          {clrDelActive && nonClearedStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} />
          ))}
        </div>
      </div>

      {/* Column 4 (20%) – STAND (draggable) */}
      <div className="w-1/5 h-full bg-[#555355] flex flex-col">
        <div className="bg-[#393939] h-10 flex items-center px-2 shrink-0">
          <span className="text-white font-bold text-lg">STAND</span>
        </div>
        <SortableBay
          strips={standStrips}
          bayId="STAND"
          standalone={false}
          className="flex-1 w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary"
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
