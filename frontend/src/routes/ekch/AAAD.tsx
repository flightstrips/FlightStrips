import { FlightStrip } from "@/components/strip/FlightStrip.tsx";
import { Message } from "@/components/Message.tsx";
import {
  useActiveMessages,
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
} from "@/store/airports/ekch.ts";
import type { FrontendStrip } from "@/api/models.ts";
import { Bay } from "@/api/models.ts";
import type { HalfStripVariant, StripStatus } from "@/components/strip/types.ts";
import { SortableBay } from "@/components/bays/SortableBay.tsx";
import { ViewDndContext } from "@/components/bays/ViewDndContext.tsx";
import { useWebSocketStore } from "@/store/store-hooks.ts";
import { useRef, useEffect } from "react";

const mapToStrip = (strip: FrontendStrip, status: StripStatus, halfStripVariant?: HalfStripVariant, selectable = true) => (
  <FlightStrip
    key={strip.callsign}
    callsign={strip.callsign}
    status={status}
    halfStripVariant={halfStripVariant}
    pdcStatus={strip.pdc_state}
    destination={strip.destination}
    origin={strip.origin}
    stand={strip.stand}
    eobt={strip.eobt}
    tobt={strip.tobt}
    tsat={strip.tsat}
    ctot={strip.ctot}
    aircraftType={strip.aircraft_type}
    squawk={strip.squawk}
    sid={strip.sid}
    runway={strip.runway}
    clearedAltitude={strip.cleared_altitude}
    requestedAltitude={strip.requested_altitude}
    owner={strip.owner}
    nextControllers={strip.next_controllers}
    previousControllers={strip.previous_controllers}
    selectable={selectable}
  />
);

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
  const messages      = useActiveMessages();
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const finalStrips   = useFinalStrips();
  const rwyArrStrips  = useRwyArrStrips();
  const standStrips   = useStandStrips();
  const twyDepStrips  = useTaxiDepStrips().sort((a, b) => a.sequence - b.sequence);
  const twyArrStrips  = useTaxiArrStrips();
  const startupStrips = useClearedStrips().sort((a, b) => a.sequence - b.sequence);
  const pushStrips    = usePushbackStrips();
  const deIceStrips   = useDeIceStrips();
  const otherStrips   = useOtherBayStrips().sort((a, b) => a.sequence - b.sequence);
  const sasStrips     = useSasBayStrips().sort((a, b) => a.sequence - b.sequence);
  const norStrips     = useNorwegianBayStrips().sort((a, b) => a.sequence - b.sequence);
  const updateOrder   = useWebSocketStore(state => state.updateOrder);
  const move          = useWebSocketStore(state => state.move);

  const bayStripMap = {
    "TWY-DEP":  { strips: twyDepStrips,  targetBay: Bay.Taxi },
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

  return (
    <ViewDndContext
      bayStripMap={bayStripMap}
      transferRules={transferRules}
      onReorder={updateOrder}
      onMove={move}
      renderDragOverlay={(callsign) => {
        const twyDep = twyDepStrips.find(s => s.callsign === callsign);
        if (twyDep) return mapToStrip(twyDep, "CLROK");
        const twyArr = twyArrStrips.find(s => s.callsign === callsign);
        if (twyArr) return mapToStrip(twyArr, "ARR");
        const startup = startupStrips.find(s => s.callsign === callsign);
        if (startup) return mapToStrip(startup, "PUSH");
        const push = pushStrips.find(s => s.callsign === callsign);
        if (push) return mapToStrip(push, "PUSH");
        const deIce = deIceStrips.find(s => s.callsign === callsign);
        if (deIce) return mapToStrip(deIce, "PUSH");
        return null;
      }}
    >
    <div className="bg-[#A9A9A9] w-screen h-[calc(100vh-4rem)] flex justify-center justify-items-center gap-2">

      {/* ── Col 1: MESSAGES / FINAL (locked) / RWY ARR (locked) / STAND ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={primaryHeader}>
          <span className={primaryLabel}>MESSAGES</span>
        </div>
        <div className={`h-[15%] ${scrollArea}`}>
          {messages.map((msg, i) => (
            <Message key={i} from={msg.from}>{msg.message}</Message>
          ))}
          <div ref={messagesEndRef} />
        </div>

        <div className={lockedHeader}>
          <span className={lockedLabel}>FINAL</span>
        </div>
        <div className={`h-[25%] ${scrollArea}`}>
          {finalStrips.map(x => mapToStrip(x, "HALF", "LOCKED-ARR", false))}
        </div>

        <div className={lockedHeader}>
          <span className={lockedLabel}>RWY ARR</span>
        </div>
        <div className={`h-[30%] ${scrollArea}`}>
          {rwyArrStrips.map(x => mapToStrip(x, "ARR", undefined, false))}
        </div>

        <div className={activeHeader}>
          <span className={activeLabel}>STAND</span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {standStrips.map(x => mapToStrip(x, "ARR"))}
        </div>

      </div>

      {/* ── Col 2: TWY DEP (UPR+LWR) / TWY ARR ── */}
      <div className="w-1/4 h-full bg-[#555355] flex flex-col">

        <div className={activeHeader + " justify-between"}>
          <span className={activeLabel}>TWY DEP</span>
          <span className="flex gap-1">
            <button className={btn}>NEW</button>
            <button className={btn}>MEM AID</button>
          </span>
        </div>
        {/* TWY DEP-UPR + LWR combined, with TW/TE/GW/GE sub-selector */}
        <SortableBay
          strips={twyDepStrips}
          bayId="TWY-DEP"
          standalone={false}
          className={`h-[60%] ${scrollArea}`}
        >
          {(callsign) => {
            const strip = twyDepStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "CLROK");
          }}
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
            <button className={btn}>MEM AID</button>
          </span>
        </div>
        <SortableBay
          strips={twyArrStrips}
          bayId="TWY-ARR"
          standalone={false}
          className={`flex-1 ${scrollArea}`}
        >
          {(callsign) => {
            const strip = twyArrStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "ARR");
          }}
        </SortableBay>

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
          {(callsign) => {
            const strip = startupStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "PUSH");
          }}
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
          {(callsign) => {
            const strip = pushStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "PUSH");
          }}
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
          {(callsign) => {
            const strip = deIceStrips.find(s => s.callsign === callsign)!;
            return mapToStrip(strip, "PUSH");
          }}
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
        <div className={`h-[40%] ${scrollArea}`}>
          {sasStrips.map(x => mapToStrip(x, "CLR", undefined, false))}
        </div>

        <div className={lockedHeader}>
          <span className={lockedLabel}>NORWEGIAN</span>
        </div>
        <div className={`h-[30%] ${scrollArea}`}>
          {norStrips.map(x => mapToStrip(x, "CLR", undefined, false))}
        </div>

        <div className={lockedHeader}>
          <span className={lockedLabel}>OTHERS</span>
        </div>
        <div className={`flex-1 ${scrollArea}`}>
          {otherStrips.map(x => mapToStrip(x, "CLR", undefined, false))}
        </div>

      </div>

    </div>
    </ViewDndContext>
  );
}
