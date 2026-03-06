import { Strip } from "@/components/strip/Strip.tsx";
import { MemAidButton } from "@/components/strip/TacticalButtons.tsx";
import { MessageStrip } from "@/components/strip/MessageStrip.tsx";
import { MessageComposeDialog } from "@/components/MessageComposeDialog.tsx";
import { useMyPosition, useMessages } from "@/store/store-hooks.ts";
import {
  useClearedStrips,
  useDeIceStrips,
  useFinalStrips,
  useNorwegianBayStrips,
  useOtherBayStrips,
  usePushbackStrips,
  useRwyArrStrips,
  useSasBayStrips,
  useStandStrips,
  useTaxiArrStrips,
  useTaxiDepStrips,
  isFlight,
} from "@/store/airports/ekch.ts";
import type { AnyStrip, StripRef } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { StripStatus } from "@/components/strip/types.ts";
import { SortableBay, DropIndicatorBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { useState } from "react";
import { APN_TAXI_DEP_STRIP_WIDTH } from "@/components/strip/ApnTaxiDepStrip.tsx";

// Shared header styles
const activeHeader   = "bg-[#b3b3b3] h-10 flex items-center px-2 shrink-0";
const activeLabel    = "text-[#393939] font-bold text-lg";
const lockedHeader   = "bg-[#393939] h-10 flex items-center px-2 shrink-0";
const lockedLabel    = "text-white font-bold text-lg";
const primaryHeader  = "bg-primary h-10 flex items-center px-2 shrink-0";
const primaryLabel   = "text-white font-bold text-lg";
const scrollArea     = "w-full bg-[#555355] p-1 flex flex-col gap-px overflow-y-auto [&::-webkit-scrollbar]:w-2 [&::-webkit-scrollbar-track]:bg-gray-100 [&::-webkit-scrollbar-thumb]:bg-primary";
const btn            = "bg-[#646464] text-white font-bold text-sm px-3 border-2 border-white active:bg-[#424242]";

export default function AAAD() {
  const myPosition  = useMyPosition();
  const messages    = useMessages();
  const [composeOpen, setComposeOpen] = useState(false);

  const finalStrips   = useFinalStrips().filter(isFlight);
  const rwyArrStrips  = useRwyArrStrips().filter(isFlight);
  const standStrips   = useStandStrips().filter(isFlight);
  const twyDepMerged  = useTaxiDepStrips();
  const twyArrStrips  = useTaxiArrStrips();
  const startupStrips = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushStrips    = usePushbackStrips().filter(isFlight);
  const deIceStrips   = useDeIceStrips().filter(isFlight);
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);

  const updateOrder       = useWebSocketStore(state => state.updateOrder);
  const move              = useWebSocketStore(state => state.move);
  const moveTacticalStrip = useWebSocketStore(state => state.moveTacticalStrip);

  const bayStripMap = {
    "TWY-DEP":  { strips: twyDepMerged,  targetBay: Bay.Taxi },
    "TWY-ARR":  { strips: twyArrStrips,  targetBay: Bay.Taxi },
    "STARTUP":  { strips: startupStrips, targetBay: Bay.Cleared },
    "PUSHBACK": { strips: pushStrips,    targetBay: Bay.Push },
    "DE-ICE":   { strips: deIceStrips,   targetBay: Bay.DeIce },
  };

  const transferRules: Record<string, string[]> = {
    "TWY-DEP":  ["TWY-ARR", "STARTUP", "PUSHBACK", "DE-ICE"],
    "TWY-ARR":  ["TWY-DEP", "STARTUP", "PUSHBACK"],
    "STARTUP":  ["TWY-DEP", "TWY-ARR", "PUSHBACK", "DE-ICE"],
    "PUSHBACK": ["TWY-DEP", "TWY-ARR", "STARTUP", "DE-ICE"],
    "DE-ICE":   ["TWY-DEP", "STARTUP", "PUSHBACK"],
  };

  const statusForBay: Record<string, StripStatus> = {
    "TWY-DEP": "TAXI-DEP",
    "TWY-ARR": "ARR",
    "STARTUP": "PUSH",
    "PUSHBACK": "PUSH",
    "DE-ICE": "PUSH",
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
        if (!isFlight(strip)) return <Strip strip={strip} width={APN_TAXI_DEP_STRIP_WIDTH} />;
        const bayEntry = Object.entries(bayStripMap).find(([, c]) =>
          c.strips.some(s => isFlight(s) && s.callsign === strip.callsign)
        );
        if (!bayEntry) return null;
        return <Strip strip={strip} status={statusForBay[bayEntry[0]]} myPosition={myPosition} />;
      }}
    >
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* ── Col 1: MESSAGES / FINAL (locked) / RWY ARR (locked) / STAND ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={primaryHeader + " justify-between"}>
          <span className={primaryLabel}>MESSAGES</span>
          <button className={btn} onClick={() => setComposeOpen(true)}>FREE TEXT</button>
        </div>
        <div className={`h-[15%] ${scrollArea}`}>
          {messages.map(msg => (
            <MessageStrip key={msg.id} msg={msg} />
          ))}
        </div>
        <MessageComposeDialog open={composeOpen} onClose={() => setComposeOpen(false)} />

        <div className={lockedHeader}>
          <span className={lockedLabel}>FINAL</span>
        </div>
        <DropIndicatorBay bayId="FINAL" className={`h-[25%] ${scrollArea}`}>
          {finalStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="HALF" halfStripVariant="LOCKED-ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>RWY ARR</span>
        </div>
        <DropIndicatorBay bayId="RWY-ARR" className={`h-[30%] ${scrollArea}`}>
          {rwyArrStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={activeHeader}>
          <span className={activeLabel}>STAND</span>
        </div>
        <DropIndicatorBay bayId="STAND" className={`flex-1 ${scrollArea}`}>
          {standStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="ARR" myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

      </div>

      {/* ── Col 2: TWY DEP (UPR+LWR) / TWY ARR ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={activeHeader + " justify-between"}>
          <span className={activeLabel}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <MemAidButton bay={Bay.Taxi} className={btn} />
          </span>
        </div>
        {/* TWY DEP-UPR + LWR combined, with TW/TE/GW/GE sub-selector */}
        <SortableBay
          strips={twyDepMerged}
          bayId="TWY-DEP"
          standalone={false}
          className={`h-[60%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="TAXI-DEP" myPosition={myPosition} width={APN_TAXI_DEP_STRIP_WIDTH} />
          )}
        </SortableBay>

        {/* TW / TE / GW / GE bay selector tabs */}
        <div className="flex shrink-0 bg-[#393939]">
          {["TW", "TE", "GW", "GE"].map(tab => (
            <button
              key={tab}
              className="flex-1 h-8 bg-[#555355] text-white font-bold text-sm border border-[#393939] hover:bg-[#6a6a6a]"
            >
              {tab}
            </button>
          ))}
        </div>

        <div className={activeHeader}>
          <span className={activeLabel}>TWY ARR</span>
          <span className="ml-auto">
            <MemAidButton bay={Bay.Taxi} className={btn} />
          </span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          <SortableBay
            strips={twyArrStrips}
            bayId="TWY-ARR"
            standalone={false}
          >
            {(strip) => (
              <Strip strip={strip} status="ARR" myPosition={myPosition} />
            )}
          </SortableBay>
        </div>

      </div>

      {/* ── Col 3: STARTUP / PUSH BACK / DE-ICE ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={lockedHeader}>
          <span className={lockedLabel}>STARTUP</span>
        </div>
        <SortableBay
          strips={startupStrips}
          bayId="STARTUP"
          standalone={false}
          className={`h-[40%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} />
          )}
        </SortableBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>PUSH BACK</span>
        </div>
        <SortableBay
          strips={pushStrips}
          bayId="PUSHBACK"
          standalone={false}
          className={`h-[30%] ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} />
          )}
        </SortableBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>DE-ICE</span>
        </div>
        <SortableBay
          strips={deIceStrips}
          bayId="DE-ICE"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(strip) => (
            <Strip strip={strip} status="PUSH" myPosition={myPosition} />
          )}
        </SortableBay>

      </div>

      {/* ── Col 4: SAS / NORWEGIAN / OTHERS (UNCLEARED) ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={lockedHeader + " justify-between"}>
          <span className={lockedLabel}>SAS</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>PLANNED</button>
          </span>
        </div>
        <DropIndicatorBay bayId="SAS" className={`h-[40%] ${scrollArea}`}>
          {sasStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>NORWEGIAN</span>
        </div>
        <DropIndicatorBay bayId="NORWEGIAN" className={`h-[30%] ${scrollArea}`}>
          {norStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

        <div className={lockedHeader}>
          <span className={lockedLabel}>OTHERS</span>
        </div>
        <DropIndicatorBay bayId="OTHERS" className={`flex-1 ${scrollArea}`}>
          {otherStrips.map(s => (
            <Strip key={s.callsign} strip={s} status="CLR" selectable={false} myPosition={myPosition} />
          ))}
        </DropIndicatorBay>

      </div>

    </div>
    </ViewDndContext>
  );
}
